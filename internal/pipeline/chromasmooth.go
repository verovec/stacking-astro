// Star-protected, mean-preserving chroma noise reduction on the combined linear RGB.
//
// The original implementation blurred the chroma residual with ONE box pass — a flat square kernel
// whose impulse response painted a literal SQUARE of smeared colour around every very bright star
// (the star's concentrated wing chroma, spread uniformly over the (2r+1)² support). This rewrite
// keeps the same luminance identity (m=(R+G+B)/3 is byte-for-byte unchanged) and adds:
//
//   - Gaussian-shaped blurs (imgops.GaussianBlur, three box passes) — no flat kernel, no square
//     corners; σ = px/√3 matches the old single box's variance, so knob semantics survive.
//   - Winsorized residuals: the blur input is clipped to ±chromaClipSigmas·σc (the chroma noise
//     scale) and re-zeroed, so a bright star's wing chroma is bounded BEFORE it can spread — it can
//     never paint a plateau at any radius.
//   - SNR protection: pixels well above the sky keep their own chroma (smoothstep weights shared by
//     the three channels), and a dilated near-saturation core mask exempts star cores + wings.
//   - A second, much coarser background-only pass (Preset.ChromaBgSmoothPx) that flattens the large
//     green/brown chroma mottle at scales the fine pass cannot reach — genuine sky only.
package pipeline

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/verove-jordan/astronomy/internal/fits"
	"github.com/verove-jordan/astronomy/internal/imgops"
)

// chromaSmoothOpts carries the two chroma-NR radii in pixels (0 = that pass off) and the sky-desat
// strength (0 = off).
type chromaSmoothOpts struct {
	FinePx   int     // Preset.ChromaSmoothPx — fine pass (residual colour patches)
	BgPx     int     // Preset.ChromaBgSmoothPx — coarse background-only pass (large chroma mottle)
	SkyDesat float64 // Preset.SkyDesat — neutralize the floor's colour + mid-scale patches on faint veils
	// Preset.SkyDesatKeepCool — hue-selective stage 1: flatten only the WARM mid-scale deviations
	// (brown/olive dust mottle, airglow) and keep the cool ones (blue reflection nebulosity). The
	// scales of both are identical on a reflection field, so hue is the only separator left.
	KeepCool bool
}

// Tuning constants for the chroma NR masks, centralised so retuning is one edit.
const (
	chromaMaskSigma    = 2.0     // σ of the luminance pre-blur that stabilises the SNR mask
	chromaClipSigmas   = 6.0     // residuals are winsorized to ±this many chroma sigmas before blurring
	chromaFineSNRLo    = 10.0    // fine pass: full smoothing below this SNR above sky…
	chromaFineSNRHi    = 30.0    // …none above it (bright star wings / galaxy cores keep their chroma)
	chromaBgSNRLo      = 2.0     // coarse pass: full smoothing below this SNR (genuine sky)…
	chromaBgSNRHi      = 6.0     // …none above it (any real signal keeps its colour)
	chromaCoreLevel    = 0.85    // near-saturation level marking bright star cores in the linear combine
	chromaStatsSamples = 200_000 // subsample size for the sky/noise statistics
	skyDesatBgPx       = 48      // coarse-field radius for the sky-desat pass when ChromaBgSmoothPx is off

	// The sky-desat masks are BRIGHTNESS-RELATIVE (fractions of the sky level), NOT stack-noise SNR:
	// in a deep stack the noise sigma is so small that even the faintest visible dust sits at 40-400σ,
	// so a σ-scaled mask classifies the whole veil as "bright signal" and the pass never touches the
	// patches the stretch makes visible (the card 0025 field bug, root-caused on M45 24/09: the whole
	// veil — brown patches AND blue nebulosity — lives between +0.05% and +0.2% of sky in linear).
	skyDesatStarLo = 0.005 // local-peak contrast (lumS−lumC)/bg where star protection starts…
	skyDesatStarHi = 0.02  // …and is total: a stacked faint star peaks ≥ +2% locally, veil texture ±0.2%
	skyDesatNebLo  = 0.02  // coarse lift (lumC−bg)/bg where the patch-flattening stage starts fading…
	skyDesatNebHi  = 0.10  // …and is off: bright extended signal keeps its own colour STRUCTURE
	skyDesatRelLo  = 1e-4  // coarse lift below which the floor is fully neutralized…
	skyDesatRelHi  = 4e-4  // …fading to none BELOW the faint veil (M45 veil ≥ +0.05%): the veil keeps its colour
)

// chromaSmoothRGB smooths ONLY the colour of a combined RGB linear FITS in place, preserving the
// per-pixel mean EXACTLY: m=(R+G+B)/3, c'=m+blend(c−m). The blend weights are shared by the three
// channels and every blur input sums to zero across channels, so (R'+G'+B')/3 stays byte-for-byte m —
// every bit of luminance/detail survives (in LRGB the L layer supplies detail anyway); only the mutual
// channel disagreements (colour noise/patches) are flattened. Bright-star wings and saturated cores
// keep their authentic chroma (see the file comment). Both radii ≤ 0 or a non-colour image → no-op.
// Soft-fail: returns a note and any error.
func chromaSmoothRGB(path string, o chromaSmoothOpts) (string, error) {
	if o.FinePx <= 0 && o.BgPx <= 0 && o.SkyDesat <= 0 {
		return "", nil
	}
	im, err := fits.ReadImage(path)
	if err != nil {
		return "", fmt.Errorf("chroma smooth: read: %w", err)
	}
	if im.C < 3 {
		return "", nil // colour smoothing only makes sense on an RGB image
	}
	f, ok := buildChromaField(im, o)
	if !ok {
		return "chroma smooth skipped: degenerate image statistics", nil
	}
	// Reuse ONE residual buffer across the three channels (a 16MP master is ~64 MB/plane; the blurs
	// allocate their own outputs, so this scratch is the only per-channel plane we control).
	scratch := make([]float32, len(im.Pix[0]))
	for ch, pix := range im.Pix[:3] {
		chromaSmoothChannel(ch, pix, scratch, f, o)
	}
	if err := im.OverwriteData(path); err != nil {
		return "", fmt.Errorf("chroma smooth: write: %w", err)
	}
	return chromaSmoothNote(o), nil
}

// chromaField is the shared per-pixel context of one chroma-smooth run: the preserved luminance and
// its statistics (for the SNR protection weights), the winsorization zero-sum plane, and the core mask.
type chromaField struct {
	w, h       int
	mean       []float32    // (R+G+B)/3 — the preserved luminance
	lumS       []float32    // gaussian-stabilised luminance driving the SNR mask
	lumC       []float32    // coarse luminance field (sky-desat only): local-peak + relative-lift masks
	starW      []float32    // sky-desat star weight (local-peak smoothstep), shared by the three channels
	smD        [3][]float32 // sky-desat coarse chroma fields (star-masked residuals), one per channel
	warm       []float32    // KeepCool hue gate: 1 = warm mid-scale deviation (flatten), 0 = cool (keep)
	zero       []float32    // per-pixel mean of the clipped residuals — subtracted so Σ(blur input) ≡ 0
	core       []bool       // dilated near-saturation cores: fully exempt from replacement
	bg, sigmaL float64      // sky level + robust luminance sigma
	clip       float32      // winsorization bound (chromaClipSigmas·σc)
}

func buildChromaField(im *fits.Image, o chromaSmoothOpts) (*chromaField, bool) {
	f := &chromaField{w: im.W, h: im.H}
	r, g, b := im.Pix[0], im.Pix[1], im.Pix[2]
	f.mean = make([]float32, len(r))
	for i := range f.mean {
		f.mean[i] = (r[i] + g[i] + b[i]) / 3
	}
	var sigmaC float64
	f.bg, f.sigmaL, sigmaC = chromaStats(im, f.mean)
	if !(f.sigmaL > 0) || !(sigmaC > 0) {
		return nil, false // flat/degenerate statistics: the masks would divide by zero — skip, never guess
	}
	f.clip = float32(chromaClipSigmas * sigmaC)
	f.lumS = imgops.GaussianBlur(f.mean, im.W, im.H, chromaMaskSigma)
	f.core = chromaCoreMask(f.mean, im.W, im.H, max(o.FinePx, 4))
	f.zero = chromaZeroSum(im, f.mean, f.clip)
	if o.SkyDesat > 0 {
		if !(f.bg > 0) {
			return nil, false // the relative-lift masks divide by the sky level
		}
		f.lumC = imgops.GaussianBlur(f.mean, im.W, im.H, chromaSigmaPx(bgRadiusPx(o)))
		f.starW = make([]float32, len(f.mean))
		for i := range f.starW {
			f.starW[i] = float32(smoothstep(skyDesatStarLo, skyDesatStarHi,
				(float64(f.lumS[i])-float64(f.lumC[i]))/f.bg))
		}
		// The desat target fields are built from STAR-MASKED, UNCLIPPED residuals. Star-masked: a
		// bright star's wing chroma must not leak into the coarse field stage 1 copies back, or
		// every star grows a soft colour disc. Unclipped: the winsorization (a star-wing guard) also
		// caps genuine broad nebula colour (a reflection veil's blue runs 5-20σc), which would turn
		// the reference field grey and stage 1 would desaturate the veil with the patches — the star
		// mask already does the guarding here. Raw residuals sum to zero across channels and the
		// mask weight is channel-shared, so the mean stays exact.
		d := make([]float32, len(f.mean))
		for ch := 0; ch < 3; ch++ {
			pix := im.Pix[ch]
			for i := range d {
				d[i] = (pix[i] - f.mean[i]) * (1 - f.starW[i])
			}
			f.smD[ch] = imgops.GaussianBlur(d, im.W, im.H, chromaSigmaPx(bgRadiusPx(o)))
		}
		if o.KeepCool {
			// Hue gate on the MID-SCALE deviation (c − smD): warm (R−B rising above its coarse
			// field) → flatten; cool → keep. Zero-centred over ±σc so neutral noise is half-treated;
			// the gate is channel-shared, so the mean stays exact.
			sigmaC := float64(f.clip) / chromaClipSigmas
			f.warm = make([]float32, len(f.mean))
			r, b := im.Pix[0], im.Pix[2]
			for i := range f.warm {
				dev := float64(r[i]-b[i]) - float64(f.smD[0][i]-f.smD[2][i])
				f.warm[i] = float32(smoothstep(-sigmaC, sigmaC, dev))
			}
		}
	}
	return f, true
}

// bgRadiusPx is the coarse-pass radius: the knob when set, the sky-desat default otherwise.
func bgRadiusPx(o chromaSmoothOpts) int {
	if o.BgPx > 0 {
		return o.BgPx
	}
	return skyDesatBgPx
}

// chromaStats measures the sky level + robust sigma of the luminance plane and the pooled chroma
// noise sigma. All MAD-based, so real signal and star wings don't inflate them.
func chromaStats(im *fits.Image, mean []float32) (bg, sigmaL, sigmaC float64) {
	bg, sigmaL = medianMAD(imgops.Subsample(mean, chromaStatsSamples))
	step := 1
	if want := chromaStatsSamples / 3; len(mean) > want {
		step = len(mean) / want
	}
	res := make([]float32, 0, 3*(len(mean)/step+1))
	for _, pix := range im.Pix[:3] {
		for i := 0; i < len(pix); i += step {
			res = append(res, pix[i]-mean[i])
		}
	}
	_, sigmaC = medianMAD(res)
	return bg, sigmaL, sigmaC
}

// medianMAD returns the median and the MAD-based robust sigma (1.4826·MAD) of vals.
func medianMAD(vals []float32) (median, sigma float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	buf := make([]float64, len(vals))
	for i, v := range vals {
		buf[i] = float64(v)
	}
	sort.Float64s(buf)
	median = buf[len(buf)/2]
	for i, v := range buf {
		buf[i] = math.Abs(v - median)
	}
	sort.Float64s(buf)
	return median, 1.4826 * buf[len(buf)/2]
}

// chromaCoreMask marks near-saturation pixels (bright star cores in the linear combine), dilated a
// few px, so the entire replacement support around a core keeps its authentic chroma.
func chromaCoreMask(mean []float32, w, h, dilatePx int) []bool {
	mask := make([]bool, len(mean))
	found := false
	for i, v := range mean {
		if v > chromaCoreLevel {
			mask[i], found = true, true
		}
	}
	if !found {
		return mask
	}
	return imgops.BinaryDilation(mask, w, h, dilatePx)
}

// chromaZeroSum is the per-pixel mean of the three winsorized residuals. Clipping is nonlinear and
// would break the exact mean preservation, so the blur input is re-zeroed by subtracting this plane —
// the three blur inputs then sum to exactly 0 at every pixel, and by linearity so do their blurs.
func chromaZeroSum(im *fits.Image, mean []float32, clip float32) []float32 {
	z := make([]float32, len(mean))
	for _, pix := range im.Pix[:3] {
		for i := range z {
			z[i] += clipF32(pix[i]-mean[i], clip) / 3
		}
	}
	return z
}

// chromaSmoothChannel replaces one channel's chroma with its star-protected smoothed version, in
// place. scratch must be len(pix) and is overwritten (shared across the three channel calls); ch is
// the channel index (selects the precomputed sky-desat coarse field).
func chromaSmoothChannel(ch int, pix, scratch []float32, f *chromaField, o chromaSmoothOpts) {
	for i := range scratch {
		scratch[i] = clipF32(pix[i]-f.mean[i], f.clip) - f.zero[i]
	}
	var smF, smB []float32
	if o.FinePx > 0 {
		smF = imgops.GaussianBlur(scratch, f.w, f.h, chromaSigmaPx(o.FinePx))
	}
	if o.BgPx > 0 {
		smB = coarseChroma(scratch, smF, f, o)
	}
	smD := f.smD[ch]
	for i := range pix {
		if f.core[i] {
			continue // saturated core + dilated wings: authentic chroma, untouched
		}
		snr := (float64(f.lumS[i]) - f.bg) / f.sigmaL
		c := float64(pix[i] - f.mean[i]) // the UNclipped original chroma
		if smF != nil {
			c += (1 - smoothstep(chromaFineSNRLo, chromaFineSNRHi, snr)) * (float64(smF[i]) - c)
		}
		if o.BgPx > 0 {
			c += (1 - smoothstep(chromaBgSNRLo, chromaBgSNRHi, snr)) * (float64(smB[i]) - c)
		}
		if o.SkyDesat > 0 {
			// Brightness-relative masks (NOT stack-σ SNR — see the constants' comment): a star is a
			// LOCAL PEAK over the coarse field, bright extended signal is a large COARSE LIFT, the
			// floor is a near-zero lift. All weights are channel-shared and both stages are linear
			// in c (smB zero-sums across channels), so the per-pixel mean is preserved exactly.
			star := float64(f.starW[i])
			rel := (float64(f.lumC[i]) - f.bg) / f.bg
			// Stage 1 — frequency separation over the whole sky + faint veil: pull the chroma toward
			// its star-masked coarse field, erasing the 10-100px colour ALTERNATION (the patches)
			// while broad colour (the coarse field itself) survives. Fades out on stars (their own
			// colour is a point the coarse field can't represent) and on bright extended signal
			// (real colour structure).
			w1 := o.SkyDesat * (1 - star) * (1 - smoothstep(skyDesatNebLo, skyDesatNebHi, rel))
			if f.warm != nil {
				w1 *= float64(f.warm[i]) // KeepCool: only warm deviations flatten, blue survives
			}
			c += w1 * (float64(smD[i]) - c)
			// Stage 2 — neutralize the TRUE floor: below skyDesatRelLo of lift the remaining broad
			// colour goes to grey, fading to none by skyDesatRelHi so faint nebulosity keeps the
			// broad tint stage 1 left it.
			c *= 1 - o.SkyDesat*(1-star)*(1-smoothstep(skyDesatRelLo, skyDesatRelHi, rel))
		}
		pix[i] = f.mean[i] + float32(c)
	}
}

// coarseChroma is the background pass's smoothing field: a further blur of the fine field when both
// passes run (gaussian composition, σB² = σF² + σrest²), else a direct blur of the residuals. The
// sky-desat pass reuses this field; when the bg radius knob is off it falls back to skyDesatBgPx.
func coarseChroma(scratch, smF []float32, f *chromaField, o chromaSmoothOpts) []float32 {
	sigmaB := chromaSigmaPx(bgRadiusPx(o))
	if smF == nil {
		return imgops.GaussianBlur(scratch, f.w, f.h, sigmaB)
	}
	sigmaF := chromaSigmaPx(o.FinePx)
	rest := math.Sqrt(math.Max(0, sigmaB*sigmaB-sigmaF*sigmaF))
	return imgops.GaussianBlur(smF, f.w, f.h, rest)
}

// chromaSigmaPx converts a knob radius (px of the legacy single box pass) to a variance-matched
// gaussian sigma: a box of radius r has σ ≈ r/√3, so the knob keeps its old smoothing power.
func chromaSigmaPx(px int) float64 { return float64(px) / math.Sqrt(3) }

// chromaSmoothNote renders the run note for the enabled passes.
func chromaSmoothNote(o chromaSmoothOpts) string {
	var parts []string
	if o.FinePx > 0 {
		parts = append(parts, fmt.Sprintf("gaussian %dpx", o.FinePx))
	}
	if o.BgPx > 0 {
		parts = append(parts, fmt.Sprintf("background %dpx", o.BgPx))
	}
	if o.SkyDesat > 0 {
		s := fmt.Sprintf("sky desat %.2g", o.SkyDesat)
		if o.KeepCool {
			s += " (cool-preserving)"
		}
		parts = append(parts, s)
	}
	return "chroma smoothed (mean-preserving, star-protected: " + strings.Join(parts, " + ") + ")"
}

// clipF32 clamps v to [−bound, bound].
func clipF32(v, bound float32) float32 {
	if v > bound {
		return bound
	}
	if v < -bound {
		return -bound
	}
	return v
}

// smoothstep is the Hermite S-curve: 0 for x ≤ a, 1 for x ≥ b, smooth in between (same shape as the
// unexported internal/noise helper; re-declared here rather than exporting it for one caller).
func smoothstep(a, b, x float64) float64 {
	if x <= a {
		return 0
	}
	if x >= b {
		return 1
	}
	t := (x - a) / (b - a)
	return t * t * (3 - 2*t)
}
