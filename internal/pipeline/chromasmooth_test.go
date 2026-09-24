package pipeline

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/fits"
)

// The synthetic fixture models the real failure: a mono-LRGB combine where each channel saw the same
// star through a slightly different PSF (focus/seeing disagreement) — concentrated wing chroma around
// a saturated core — over a noisy sky. The old single box pass smeared that wing chroma into a literal
// SQUARE around every bright star; these tests pin the rewrite's guarantees.
type starChromaCfg struct {
	w, h     int
	pedestal float32 // sky level (all channels)
	lumNoise float32 // mean-SHARED 1px checker (keeps sigmaL > 0, adds zero chroma)
	speckle  float32 // mean-NEUTRAL 1px checker on R/−B — the fine chroma noise (sets sigmaC)
	star     bool    // saturated star at centre with per-channel PSF widths (σ 2.2/2.5/2.9)
	blotch   float32 // mean-neutral 48px sin×sin field on R/−B — the large chroma mottle
	galaxy   bool    // flat ellipse far above every SNR mask with a uniform R−B tint of 2·galaxyTint
	veil     bool    // faint ellipse at ~7σ luminance with a uniform R−B tint of 2·veilTint — the
	// dusty-veil case: real broad colour BELOW the fine SNR mask, with the blotch field on top
}

const (
	fixStarAmp = 0.95
	galaxyLum  = 0.24 // > 30σ whichever branch the checker MAD lands on — above the fine mask
	galaxyTint = 0.01
	// veilLum sits above the bg mask (>6σ) and inside the fine mask (≲10σ) under EITHER branch the
	// checker fixture's two-valued MAD can land on (σL ≈ 0.003 or ≈ 0.006) — the masks' behaviour on
	// the veil is then branch-independent: stage 1 fully applies, stage 2 not at all.
	veilLum                                = 0.04
	veilTint                               = 0.008
	fixSpeckle                             = float32(0.004)
	fixSigmaC                              = 1.4826 * 0.004 // MAD-based sigma of the ±fixSpeckle checker residuals
	fixLumNoise                            = float32(0.002)
	fixPedestal                            = float32(0.05)
	galaxyCX, galaxyCY, galaxyRX, galaxyRY = 32, 32, 24, 16
	veilCX, veilCY, veilRX, veilRY         = 144, 144, 32, 24
)

func makeStarChromaRGB(t *testing.T, dir, name string, cfg starChromaCfg) string {
	t.Helper()
	im := fits.NewImage(cfg.w, cfg.h, 3)
	cx, cy := cfg.w/2, cfg.h/2
	sigmas := [3]float64{2.2, 2.5, 2.9} // per-channel PSF widths → wing chroma around the core
	for y := 0; y < cfg.h; y++ {
		for x := 0; x < cfg.w; x++ {
			i := y*cfg.w + x
			lum := cfg.pedestal
			if (x+y)%2 == 0 {
				lum += cfg.lumNoise
			} else {
				lum -= cfg.lumNoise
			}
			chroma := float32(0) // added to R, subtracted from B (mean-neutral)
			if cfg.speckle > 0 {
				if (x+y)%2 == 0 {
					chroma += cfg.speckle
				} else {
					chroma -= cfg.speckle
				}
			}
			if cfg.blotch > 0 {
				chroma += cfg.blotch * float32(math.Sin(2*math.Pi*float64(x)/48)*math.Sin(2*math.Pi*float64(y)/48))
			}
			if cfg.galaxy && insideEllipse(x, y, galaxyCX, galaxyCY, galaxyRX, galaxyRY) {
				lum += galaxyLum
				chroma += galaxyTint
			}
			if cfg.veil && insideEllipse(x, y, veilCX, veilCY, veilRX, veilRY) {
				lum += veilLum
				chroma += veilTint
			}
			r, g, b := lum+chroma, lum, lum-chroma
			if cfg.star {
				d2 := float64((x-cx)*(x-cx) + (y-cy)*(y-cy))
				r += float32(fixStarAmp * math.Exp(-d2/(2*sigmas[0]*sigmas[0])))
				g += float32(fixStarAmp * math.Exp(-d2/(2*sigmas[1]*sigmas[1])))
				b += float32(fixStarAmp * math.Exp(-d2/(2*sigmas[2]*sigmas[2])))
			}
			im.Pix[0][i], im.Pix[1][i], im.Pix[2][i] = r, g, b
		}
	}
	p := filepath.Join(dir, name+".fits")
	require.NoError(t, im.WriteFITS(p))
	return p
}

func insideEllipse(x, y, cx, cy, rx, ry int) bool {
	dx, dy := float64(x-cx)/float64(rx), float64(y-cy)/float64(ry)
	return dx*dx+dy*dy <= 1
}

func readRGB(t *testing.T, path string) *fits.Image {
	t.Helper()
	im, err := fits.ReadImage(path)
	require.NoError(t, err)
	return im
}

// TestChromaSmoothRGB_LuminanceByteIdentical: whatever the pass combination, the per-pixel RGB mean
// (the luminance) is preserved — the contract that makes the whole operation detail-lossless.
func TestChromaSmoothRGB_LuminanceByteIdentical(t *testing.T) {
	cfg := starChromaCfg{w: 128, h: 128, pedestal: fixPedestal, lumNoise: fixLumNoise,
		speckle: fixSpeckle, star: true, blotch: 0.01, galaxy: true}
	cases := []struct {
		name string
		o    chromaSmoothOpts
	}{
		{"fine only", chromaSmoothOpts{FinePx: 6}},
		{"background only", chromaSmoothOpts{BgPx: 24}},
		{"both", chromaSmoothOpts{FinePx: 6, BgPx: 24}},
		{"sky desat only", chromaSmoothOpts{SkyDesat: 1}},
		{"all three", chromaSmoothOpts{FinePx: 6, BgPx: 24, SkyDesat: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := makeStarChromaRGB(t, dir, "rgb", cfg)
			in := readRGB(t, path)
			_, err := chromaSmoothRGB(path, tc.o)
			require.NoError(t, err)
			out := readRGB(t, path)
			var maxErr float64
			for i := range out.Pix[0] {
				mIn := float64(in.Pix[0][i]+in.Pix[1][i]+in.Pix[2][i]) / 3
				mOut := float64(out.Pix[0][i]+out.Pix[1][i]+out.Pix[2][i]) / 3
				if d := math.Abs(mOut - mIn); d > maxErr {
					maxErr = d
				}
			}
			assert.Less(t, maxErr, 1e-4, "per-pixel RGB mean must survive the smoothing")
		})
	}
}

// TestChromaSmoothRGB_GaussianImpulseNoSquare: a point chroma impulse must spread as a round gaussian,
// not the old box plateau — the corner of the old kernel carried the SAME energy as its edge (ratio 1);
// a gaussian's corner at (r,r) is exp(−r²/2σ²) of its edge at (r,0) ≈ 0.22 for r=6, σ=6/√3.
func TestChromaSmoothRGB_GaussianImpulseNoSquare(t *testing.T) {
	const w, h = 96, 96
	dir := t.TempDir()
	path := makeStarChromaRGB(t, dir, "rgb", starChromaCfg{w: w, h: h,
		pedestal: fixPedestal, lumNoise: fixLumNoise, speckle: fixSpeckle})
	im := readRGB(t, path)
	cx, cy := w/2, h/2
	imp := float32(5 * fixSigmaC) // strong but below the ±6σ winsorization: tests the blur shape, not the clip
	im.Pix[0][cy*w+cx] += imp
	im.Pix[2][cy*w+cx] -= imp
	require.NoError(t, im.OverwriteData(path))

	_, err := chromaSmoothRGB(path, chromaSmoothOpts{FinePx: 6})
	require.NoError(t, err)
	out := readRGB(t, path)
	chromaAt := func(x, y int) float64 {
		i := y*w + x
		return float64(out.Pix[0][i] - out.Pix[2][i])
	}
	edge := math.Abs(chromaAt(cx+6, cy))
	corner := math.Abs(chromaAt(cx+6, cy+6))
	require.Greater(t, edge, 1e-6, "the spread impulse must be measurable at the kernel edge")
	assert.Less(t, corner, 0.5*edge,
		"corner ≈ edge is the square-box signature; a gaussian corner must carry well under half the edge")
}

// TestChromaSmoothRGB_StarCannotPaintBackground: whatever the radius, a saturated star's wing chroma
// is winsorized + masked before it can spread — sky pixels far from the star keep their chroma to
// within a couple of noise sigmas. This is the "squares around bright stars" regression pin.
func TestChromaSmoothRGB_StarCannotPaintBackground(t *testing.T) {
	for _, fine := range []int{6, 24} {
		t.Run(map[int]string{6: "default radius", 24: "extreme radius"}[fine], func(t *testing.T) {
			const w, h = 160, 160
			dir := t.TempDir()
			path := makeStarChromaRGB(t, dir, "rgb", starChromaCfg{w: w, h: h,
				pedestal: fixPedestal, lumNoise: fixLumNoise, speckle: fixSpeckle, star: true})
			in := readRGB(t, path)
			_, err := chromaSmoothRGB(path, chromaSmoothOpts{FinePx: fine})
			require.NoError(t, err)
			out := readRGB(t, path)

			cx, cy := w/2, h/2
			var maxDelta float64
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if (x-cx)*(x-cx)+(y-cy)*(y-cy) < 40*40 {
						continue // judge genuine sky, clear of the star and its mask transition
					}
					i := y*w + x
					dIn := float64(in.Pix[0][i] - in.Pix[2][i])
					dOut := float64(out.Pix[0][i] - out.Pix[2][i])
					if d := math.Abs(dOut - dIn); d > maxDelta {
						maxDelta = d
					}
				}
			}
			assert.Less(t, maxDelta, 2*fixSigmaC,
				"sky chroma may only move by noise-scale amounts — never by star-wing leakage")
		})
	}
}

// TestChromaSmoothRGB_StarWingChromaPreserved: the dilated near-saturation core mask exempts the star
// core and its wings entirely — their (authentic) per-channel PSF chroma is byte-identical.
func TestChromaSmoothRGB_StarWingChromaPreserved(t *testing.T) {
	const w, h = 96, 96
	dir := t.TempDir()
	path := makeStarChromaRGB(t, dir, "rgb", starChromaCfg{w: w, h: h,
		pedestal: fixPedestal, lumNoise: fixLumNoise, speckle: fixSpeckle, star: true})
	in := readRGB(t, path)
	_, err := chromaSmoothRGB(path, chromaSmoothOpts{FinePx: 6, BgPx: 24})
	require.NoError(t, err)
	out := readRGB(t, path)

	cx, cy := w/2, h/2
	for _, off := range [][2]int{{2, 0}, {3, 0}, {0, 3}, {-3, 0}, {2, 2}} {
		i := (cy+off[1])*w + (cx + off[0])
		assert.InDelta(t, in.Pix[0][i]-in.Pix[2][i], out.Pix[0][i]-out.Pix[2][i], 1e-6,
			"wing pixel (%+d,%+d) sits inside the dilated core mask and must keep its chroma exactly", off[0], off[1])
	}
}

// TestChromaSmoothRGB_BackgroundMottleReduced: the coarse pass flattens large-scale sky colour mottle
// the fine pass cannot reach; without it the mottle survives nearly intact.
func TestChromaSmoothRGB_BackgroundMottleReduced(t *testing.T) {
	const w, h = 192, 192
	rms := func(im *fits.Image) float64 {
		var sum float64
		for i := range im.Pix[0] {
			d := float64(im.Pix[0][i] - im.Pix[2][i])
			sum += d * d
		}
		return math.Sqrt(sum / float64(len(im.Pix[0])))
	}
	cfg := starChromaCfg{w: w, h: h, pedestal: fixPedestal, lumNoise: fixLumNoise, blotch: 0.01}

	dir := t.TempDir()
	both := makeStarChromaRGB(t, dir, "both", cfg)
	fineOnly := makeStarChromaRGB(t, dir, "fine", cfg)
	before := rms(readRGB(t, both))

	_, err := chromaSmoothRGB(both, chromaSmoothOpts{FinePx: 6, BgPx: 24})
	require.NoError(t, err)
	_, err = chromaSmoothRGB(fineOnly, chromaSmoothOpts{FinePx: 6})
	require.NoError(t, err)

	withBg, withoutBg := rms(readRGB(t, both)), rms(readRGB(t, fineOnly))
	assert.Less(t, withBg, 0.4*before, "coarse pass must flatten the 48px mottle by ≥60%%")
	assert.Greater(t, withoutBg, 0.6*before, "the fine pass alone barely touches 48px mottle — the coarse knob does the work")
}

// TestChromaSmoothRGB_ObjectColourPreserved: a real object (far above sky) keeps its colour — the
// coarse pass is background-only, and the fine pass is an identity on a uniform tint.
func TestChromaSmoothRGB_ObjectColourPreserved(t *testing.T) {
	const w, h = 128, 128
	dir := t.TempDir()
	path := makeStarChromaRGB(t, dir, "rgb", starChromaCfg{w: w, h: h,
		pedestal: fixPedestal, lumNoise: fixLumNoise, speckle: fixSpeckle, galaxy: true})
	in := readRGB(t, path)
	_, err := chromaSmoothRGB(path, chromaSmoothOpts{FinePx: 6, BgPx: 24})
	require.NoError(t, err)
	out := readRGB(t, path)

	// Judge the ellipse interior (eroded to half radii, clear of the boundary transition).
	tintMean := func(im *fits.Image) float64 {
		var sum float64
		var n int
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if !insideEllipse(x, y, galaxyCX, galaxyCY, galaxyRX/2, galaxyRY/2) {
					continue
				}
				sum += float64(im.Pix[0][y*w+x] - im.Pix[2][y*w+x])
				n++
			}
		}
		return sum / float64(n)
	}
	tin, tout := tintMean(in), tintMean(out)
	require.InDelta(t, 2*galaxyTint, tin, 1e-3, "fixture sanity: interior tint is 2·galaxyTint")
	assert.InDelta(t, tin, tout, 0.02*tin, "object interior tint must survive within 2%%")
}

// deepStackCfg is the sky-desat fixture at REALISTIC deep-stack scales: after ~80 stacked subs the
// pixel noise is ~3e-5 of full scale while everything the stretch makes visible (dust veil, blue
// nebulosity, colour patches) lives at +0.05%..+2% of the sky level — i.e. at enormous noise-SNR.
// The card 0026 bug (σ-scaled masks classifying the whole veil as bright signal) is invisible to a
// fixture with exaggerated noise, so this one pins the brightness-relative semantics instead.
const (
	dsPedestal         = float32(0.05)
	dsLumNoise         = float32(2e-5) // ±checker → σL ≈ 3e-5: the deep-stack pixel noise scale
	dsSpeckle          = float32(4e-6) // fine chroma noise → σc, the winsorization scale
	dsBlotch           = float32(6e-6) // 48px chroma alternation — the patches, everywhere
	dsVeilLift         = float32(5e-4) // veil at +1% of sky: ≫ noise, below skyDesatNebLo
	dsVeilTint         = float32(4e-6)
	dsBlobLift         = float32(7.5e-3) // bright extended signal at +15%: above skyDesatNebHi
	dsBlobTint         = float32(8e-6)
	dsStarAmp          = float32(3e-3) // point star: a LOCAL peak ≫ skyDesatStarHi after σ2 blur
	dsW, dsH           = 288, 224
	dsVeilCX, dsVeilCY = 200, 150
	dsVeilRX, dsVeilRY = 80, 60
	dsBlobCX, dsBlobCY = 70, 60
	dsBlobRX, dsBlobRY = 40, 30
	dsStarCX, dsStarCY = 60, 170
)

func makeDeepStackRGB(t *testing.T, dir string) string {
	t.Helper()
	im := fits.NewImage(dsW, dsH, 3)
	sigmas := [3]float64{2.2, 2.5, 2.9} // per-channel PSF widths → authentic star wing chroma
	for y := 0; y < dsH; y++ {
		for x := 0; x < dsW; x++ {
			i := y*dsW + x
			lum := dsPedestal
			if (x+y)%2 == 0 {
				lum += dsLumNoise
			} else {
				lum -= dsLumNoise
			}
			chroma := float32(0)
			if (x+y)%2 == 0 {
				chroma += dsSpeckle
			} else {
				chroma -= dsSpeckle
			}
			chroma += dsBlotch * float32(math.Sin(2*math.Pi*float64(x)/48)*math.Sin(2*math.Pi*float64(y)/48))
			if insideEllipse(x, y, dsVeilCX, dsVeilCY, dsVeilRX, dsVeilRY) {
				lum += dsVeilLift
				chroma += dsVeilTint
			}
			if insideEllipse(x, y, dsBlobCX, dsBlobCY, dsBlobRX, dsBlobRY) {
				lum += dsBlobLift
				chroma += dsBlobTint
			}
			r, g, b := lum+chroma, lum, lum-chroma
			d2 := float64((x-dsStarCX)*(x-dsStarCX) + (y-dsStarCY)*(y-dsStarCY))
			r += dsStarAmp * float32(math.Exp(-d2/(2*sigmas[0]*sigmas[0])))
			g += dsStarAmp * float32(math.Exp(-d2/(2*sigmas[1]*sigmas[1])))
			b += dsStarAmp * float32(math.Exp(-d2/(2*sigmas[2]*sigmas[2])))
			im.Pix[0][i], im.Pix[1][i], im.Pix[2][i] = r, g, b
		}
	}
	p := filepath.Join(dir, "deep.fits")
	require.NoError(t, im.WriteFITS(p))
	return p
}

// TestChromaSmoothRGB_SkyDesat_DeepStackSemantics pins the brightness-relative behaviour on the
// realistic fixture: (a) the floor's chroma collapses, (b) the patches riding ON the faint veil
// flatten while the veil's broad tint survives, (c) a bright extended object keeps its colour AND
// its colour structure, (d) a star keeps its core chroma (contrast protection, not level).
func TestChromaSmoothRGB_SkyDesat_DeepStackSemantics(t *testing.T) {
	dir := t.TempDir()
	path := makeDeepStackRGB(t, dir)
	in := readRGB(t, path)
	_, err := chromaSmoothRGB(path, chromaSmoothOpts{SkyDesat: 1})
	require.NoError(t, err)
	out := readRGB(t, path)

	rb := func(im *fits.Image, i int) float64 { return float64(im.Pix[0][i] - im.Pix[2][i]) }
	stats := func(im *fits.Image, keep func(x, y int) bool) (mean, std float64) {
		var vals []float64
		for y := 0; y < dsH; y++ {
			for x := 0; x < dsW; x++ {
				if keep(x, y) {
					vals = append(vals, rb(im, y*dsW+x))
				}
			}
		}
		for _, v := range vals {
			mean += v
		}
		mean /= float64(len(vals))
		for _, v := range vals {
			std += (v - mean) * (v - mean)
		}
		return mean, math.Sqrt(std / float64(len(vals)))
	}

	// (a) floor (clear of veil, blob and star): chroma rms collapses.
	sky := func(x, y int) bool {
		return x >= 5 && x < 40 && y >= 100 && y < 215 && (x-dsStarCX)*(x-dsStarCX)+(y-dsStarCY)*(y-dsStarCY) > 30*30
	}
	_, sIn := stats(in, sky)
	_, sOut := stats(out, sky)
	require.Greater(t, sIn, float64(dsBlotch), "fixture sanity: the floor carries the chroma mottle")
	assert.Less(t, sOut, 0.15*sIn, "sky-floor chroma must be ≥85%% neutralized")

	// (b) veil interior (eroded): broad tint survives, patches flatten.
	veil := func(x, y int) bool { return insideEllipse(x, y, dsVeilCX, dsVeilCY, dsVeilRX/2, dsVeilRY/2) }
	mIn, pIn := stats(in, veil)
	mOut, pOut := stats(out, veil)
	require.InDelta(t, 2*float64(dsVeilTint), mIn, 2e-6, "fixture sanity: veil interior tint")
	assert.Greater(t, mOut, 0.6*mIn, "the veil's broad colour must survive (≥60%%)")
	assert.Less(t, pOut, 0.4*pIn, "the colour patches ON the veil must flatten by ≥60%%")

	// (c) bright extended object: tint AND colour structure preserved (the neb mask fades stage 1 out).
	blob := func(x, y int) bool { return insideEllipse(x, y, dsBlobCX, dsBlobCY, dsBlobRX/2, dsBlobRY/2) }
	bmIn, bsIn := stats(in, blob)
	bmOut, bsOut := stats(out, blob)
	assert.Greater(t, bmOut, 0.85*bmIn, "bright extended signal keeps its tint")
	assert.Greater(t, bsOut, 0.75*bsIn, "bright extended signal keeps its colour STRUCTURE")

	// (d) star core: chroma survives via the local-peak protection.
	coreIn := rb(in, (dsStarCY)*dsW+dsStarCX+3) // wing pixel where per-channel PSFs disagree most
	coreOut := rb(out, (dsStarCY)*dsW+dsStarCX+3)
	require.Greater(t, math.Abs(coreIn), 1e-5, "fixture sanity: the star wing carries chroma")
	assert.Greater(t, math.Abs(coreOut), 0.5*math.Abs(coreIn), "a star keeps ≥50%% of its wing chroma")
}

// TestChromaSmoothRGB_SkyDesatZero_NoopByteIdentical: sky_desat 0 with no radii is a TRUE no-op —
// the un-configured contract (default runs stay byte-identical).
func TestChromaSmoothRGB_SkyDesatZero_NoopByteIdentical(t *testing.T) {
	dir := t.TempDir()
	path := makeStarChromaRGB(t, dir, "rgb", starChromaCfg{w: 64, h: 64,
		pedestal: fixPedestal, lumNoise: fixLumNoise, speckle: fixSpeckle, blotch: 0.01})
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	note, err := chromaSmoothRGB(path, chromaSmoothOpts{SkyDesat: 0})
	require.NoError(t, err)
	assert.Empty(t, note)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(before, after), "sky_desat 0 and no radii → the file must not be rewritten")
}

// TestChromaSmoothRGB_NoopBothZero: both radii at 0 is a TRUE no-op — the file bytes are untouched.
func TestChromaSmoothRGB_NoopBothZero(t *testing.T) {
	dir := t.TempDir()
	path := makeStarChromaRGB(t, dir, "rgb", starChromaCfg{w: 64, h: 64,
		pedestal: fixPedestal, lumNoise: fixLumNoise, speckle: fixSpeckle})
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	note, err := chromaSmoothRGB(path, chromaSmoothOpts{})
	require.NoError(t, err)
	assert.Empty(t, note)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(before, after), "no pass enabled → the file must not be rewritten")
}

// TestChromaSmoothRGB_DegenerateStatsSkips: a flat image has no measurable noise scale — the masks
// would divide by zero, so the step skips with a note instead of guessing.
func TestChromaSmoothRGB_DegenerateStatsSkips(t *testing.T) {
	dir := t.TempDir()
	im := fits.NewImage(32, 32, 3)
	for c := 0; c < 3; c++ {
		for i := range im.Pix[c] {
			im.Pix[c][i] = 0.25
		}
	}
	path := filepath.Join(dir, "flat.fits")
	require.NoError(t, im.WriteFITS(path))
	in := readRGB(t, path)

	note, err := chromaSmoothRGB(path, chromaSmoothOpts{FinePx: 6, BgPx: 24})
	require.NoError(t, err)
	assert.Contains(t, note, "skipped")
	out := readRGB(t, path)
	assert.Equal(t, in.Pix[0], out.Pix[0], "degenerate statistics must leave pixels untouched")
}
