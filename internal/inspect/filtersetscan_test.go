package inspect

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/fits"
)

const testBiasADU = 500.0 // a typical ZWO electronic pedestal — twenty times the broadband sky

// cfaImage builds an undebayered RGGB mosaic whose primaries sit at the given levels (already
// including the pedestal), so a test can state a sky in the same ADU the runbooks use.
func cfaImage(r, g, b float64) *fits.Image {
	const w, h = 8, 8
	pix := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v float64
			switch {
			case y%2 == 0 && x%2 == 0:
				v = r
			case y%2 == 1 && x%2 == 1:
				v = b
			default:
				v = g
			}
			pix[y*w+x] = float32(v)
		}
	}
	return &fits.Image{W: w, H: h, C: 1, Pix: [][]float32{pix}}
}

// oscFrame is a one-shot-color frame of the given type at the given exposure.
func oscFrame(path string, typ FrameType, exposureMs int64) *Frame {
	return &Frame{
		Path: path, Type: typ, Bayer: "RGGB", Filter: filters.Color,
		ExposureMs: exposureMs, Gain: 100, Offset: 50, BinX: 1, BinY: 1,
		// EGAIN 1 e-/ADU keeps these fixtures stating their skies in the unit the thresholds use,
		// so every expectation here is unchanged by the electron conversion.
		EGain: 1.0,
	}
}

// oscInventory assembles a scan and runs the same finalize steps the scanner does.
func oscInventory(frames ...*Frame) *Inventory {
	inv := &Inventory{Frames: frames}
	inv.ColorModel = colorModel(inv)
	inv.Sets = buildSets(inv.Frames)
	return inv
}

func loaderFor(images map[string]*fits.Image) imageLoader {
	return func(path string) (*fits.Image, error) {
		im, ok := images[path]
		if !ok {
			return nil, errors.New("no such frame")
		}
		return im, nil
	}
}

func setIDFor(t *testing.T, inv *Inventory, typ FrameType, exposureMs int64) string {
	t.Helper()
	for _, s := range inv.Sets {
		if s.Key.Type == typ && s.Key.ExposureMs == exposureMs {
			return s.Key.ID()
		}
	}
	t.Fatalf("no %s set at %dms", typ, exposureMs)
	return ""
}

// TestInventory_FilterSetAnnotation covers the whole measured path: two colour nights at different
// exposures, one dual-band and one broadband, separated purely by their pixels.
func TestInventory_FilterSetAnnotation(t *testing.T) {
	dual := oscFrame("dual.fits", Light, 120_000)
	broad := oscFrame("broad.fits", Light, 300_000)
	bias := oscFrame("bias.fits", Bias, 0)
	inv := oscInventory(dual, broad, bias)
	require.Equal(t, ColorOSC, inv.ColorModel)

	load := loaderFor(map[string]*fits.Image{
		// Dual-band at 120 s: 6/3/2 ADU of sky above the pedestal, red-dominant.
		"dual.fits": cfaImage(testBiasADU+6, testBiasADU+3, testBiasADU+2),
		// Broadband at 300 s: 70/95/105 → 28/38/42 per 120 s, blue-dominant.
		"broad.fits": cfaImage(testBiasADU+70, testBiasADU+95, testBiasADU+105),
		"bias.fits":  cfaImage(testBiasADU, testBiasADU, testBiasADU),
	})

	annotateFilterSets(inv, nil, load)

	dualID := setIDFor(t, inv, Light, 120_000)
	broadID := setIDFor(t, inv, Light, 300_000)
	assert.Equal(t, filters.FilterSetDualband, inv.FilterSets[dualID])
	assert.Equal(t, filters.FilterSetBroadband, inv.FilterSets[broadID])

	// The projection onto the Set values must match the durable map.
	for _, s := range inv.Sets {
		assert.Equal(t, inv.FilterSets[s.Key.ID()], s.FilterSet, "set %s", s.Key.ID())
	}
	// Calibration sets are never classified — a bias has no sky to measure.
	assert.NotContains(t, inv.FilterSets, setIDFor(t, inv, Bias, 0))
}

// TestFilterSetAnnotation_NoBiasIsUnknown pins the honest-refusal case. An uncalibrated frame sits at
// hundreds of ADU of pure electronic offset, which dwarfs even a broadband sky — so without a
// pedestal to subtract, classifying would report "broadband" for everything, confidently and wrongly.
func TestFilterSetAnnotation_NoBiasIsUnknown(t *testing.T) {
	dual := oscFrame("dual.fits", Light, 120_000)
	inv := oscInventory(dual)
	load := loaderFor(map[string]*fits.Image{
		"dual.fits": cfaImage(testBiasADU+6, testBiasADU+3, testBiasADU+2),
	})

	annotateFilterSets(inv, nil, load)

	assert.Empty(t, inv.FilterSets, "no pedestal to subtract → no verdict")
	for _, s := range inv.Sets {
		assert.Empty(t, string(s.FilterSet))
	}
}

// TestFilterSetAnnotation_MonoScanUntouched: the whole feature is one-shot-color only, and a mono
// inventory must come out byte-identical to one produced before filter sets existed.
func TestFilterSetAnnotation_MonoScanUntouched(t *testing.T) {
	inv := oscInventory(
		&Frame{Path: "l.fits", Type: Light, Filter: "L", ExposureMs: 120_000, Gain: 139, BinX: 1, BinY: 1},
		&Frame{Path: "b.fits", Type: Bias, ExposureMs: 0, Gain: 139, BinX: 1, BinY: 1},
	)
	require.Equal(t, ColorMono, inv.ColorModel)

	annotateFilterSets(inv, nil, func(string) (*fits.Image, error) {
		t.Fatal("a mono scan must not read a single pixel for filter sets")
		return nil, nil
	})

	assert.Nil(t, inv.FilterSets)
}

// TestFilterSetAnnotation_OverrideWins pins the precedence: the user looked at the stack, the
// classifier only looked at three frames. An override also applies where detection found nothing —
// that is the entire point of having one.
func TestFilterSetAnnotation_OverrideWins(t *testing.T) {
	dual := oscFrame("dual.fits", Light, 120_000)
	bias := oscFrame("bias.fits", Bias, 0)
	inv := oscInventory(dual, bias)
	load := loaderFor(map[string]*fits.Image{
		"dual.fits": cfaImage(testBiasADU+6, testBiasADU+3, testBiasADU+2),
		"bias.fits": cfaImage(testBiasADU, testBiasADU, testBiasADU),
	})
	dualID := setIDFor(t, inv, Light, 120_000)

	t.Run("beats a measured verdict", func(t *testing.T) {
		annotateFilterSets(inv, map[string]filters.FilterSet{dualID: filters.FilterSetBroadband}, load)
		assert.Equal(t, filters.FilterSetBroadband, inv.FilterSets[dualID])
	})

	t.Run("applies where nothing could be measured", func(t *testing.T) {
		blind := oscInventory(oscFrame("dual.fits", Light, 120_000)) // no bias → no detection
		id := setIDFor(t, blind, Light, 120_000)
		annotateFilterSets(blind, map[string]filters.FilterSet{id: filters.FilterSetDualband},
			func(string) (*fits.Image, error) { return nil, errors.New("unreadable") })
		assert.Equal(t, filters.FilterSetDualband, blind.FilterSets[id])
	})

	t.Run("an unknown override is not a verdict", func(t *testing.T) {
		annotateFilterSets(inv, map[string]filters.FilterSet{dualID: filters.FilterSetUnknown}, load)
		assert.Equal(t, filters.FilterSetDualband, inv.FilterSets[dualID], "falls back to detection")
	})
}

// TestFilterSetAnnotation_NormalizedPixels: Siril writes 32-bit float FITS normalized to [0,1], so
// the same night measured after a Siril pass must classify the same as the camera's 16-bit original.
func TestFilterSetAnnotation_NormalizedPixels(t *testing.T) {
	norm := func(r, g, b float64) *fits.Image {
		return cfaImage(r/adu16Scale, g/adu16Scale, b/adu16Scale)
	}
	dual := oscFrame("dual.fits", Light, 120_000)
	bias := oscFrame("bias.fits", Bias, 0)
	inv := oscInventory(dual, bias)
	load := loaderFor(map[string]*fits.Image{
		"dual.fits": norm(testBiasADU+6, testBiasADU+3, testBiasADU+2),
		"bias.fits": norm(testBiasADU, testBiasADU, testBiasADU),
	})

	annotateFilterSets(inv, nil, load)

	assert.Equal(t, filters.FilterSetDualband, inv.FilterSets[setIDFor(t, inv, Light, 120_000)])
}

// TestCFAChannelSky_Patterns pins the mosaic geometry: read the primaries at the wrong offsets and
// every verdict inverts, silently.
func TestCFAChannelSky_Patterns(t *testing.T) {
	// One 2×2 cell repeated: positions (0,0),(1,0),(0,1),(1,1) hold 10,20,30,40.
	pix := []float32{10, 20, 10, 20, 30, 40, 30, 40}
	const w, h = 4, 2

	tests := []struct {
		pattern string
		want    ChannelSky
	}{
		{"RGGB", ChannelSky{R: 10, G: 25, B: 40}}, // R=(0,0)=10, G=(1,0)+(0,1)=20,30, B=(1,1)=40
		{"BGGR", ChannelSky{R: 40, G: 25, B: 10}},
		{"GRBG", ChannelSky{R: 20, G: 25, B: 30}}, // R=(1,0)=20, G=(0,0)+(1,1)=10,40, B=(0,1)=30
		{"GBRG", ChannelSky{R: 30, G: 25, B: 20}},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, ok := cfaChannelSky(pix, w, h, tt.pattern)
			require.True(t, ok)
			assert.InDelta(t, tt.want.R, got.R, 1e-9)
			assert.InDelta(t, tt.want.G, got.G, 1e-9)
			assert.InDelta(t, tt.want.B, got.B, 1e-9)
		})
	}

	t.Run("an unrecognised pattern yields nothing rather than a guess", func(t *testing.T) {
		_, ok := cfaChannelSky(pix, w, h, "XYZW")
		assert.False(t, ok)
	})
}

// TestApplyFilterSets_SurvivesSetRebuild is why the map is the durable record: several code paths
// rebuild Sets from Frames, and buildSets returns fresh structs with no FilterSet on them.
func TestApplyFilterSets_SurvivesSetRebuild(t *testing.T) {
	dual := oscFrame("dual.fits", Light, 120_000)
	bias := oscFrame("bias.fits", Bias, 0)
	inv := oscInventory(dual, bias)
	load := loaderFor(map[string]*fits.Image{
		"dual.fits": cfaImage(testBiasADU+6, testBiasADU+3, testBiasADU+2),
		"bias.fits": cfaImage(testBiasADU, testBiasADU, testBiasADU),
	})
	annotateFilterSets(inv, nil, load)
	id := setIDFor(t, inv, Light, 120_000)
	require.Equal(t, filters.FilterSetDualband, inv.Sets[0].FilterSet)

	inv.Sets = buildSets(inv.Frames) // what excludeFrames and friends do
	for _, s := range inv.Sets {
		require.Empty(t, string(s.FilterSet), "a rebuild drops the projection")
	}
	applyFilterSets(inv)

	assert.Equal(t, filters.FilterSetDualband, inv.FilterSets[id])
	for _, s := range inv.Sets {
		assert.Equal(t, inv.FilterSets[s.Key.ID()], s.FilterSet)
	}
}

// lightSet is a one-shot-colour light set on a night, carrying a measured verdict.
func lightSet(night string, exposureMs int64, fs filters.FilterSet) Set {
	return Set{Key: SetKey{
		Type: Light, Object: "NGC7000", Filter: filters.Color,
		ExposureMs: exposureMs, Gain: 100, Offset: 50, Bin: 1, Session: night, Color: true,
	}, FilterSet: fs}
}

// calSet is a calibration set on a night. It never carries a verdict of its own — that is the whole
// reason NightFilterSet exists.
func calSet(night string, typ FrameType, exposureMs int64) Set {
	return Set{Key: SetKey{
		Type: typ, ExposureMs: exposureMs, Gain: 100, Offset: 50, Bin: 1, Session: night, Color: true,
	}}
}

// invWith assembles an inventory from sets and derives the durable verdict map from them, the way
// annotateFilterSets does.
func invWith(sets ...Set) *Inventory {
	inv := &Inventory{Sets: sets, ColorModel: ColorOSC}
	verdicts := map[string]filters.FilterSet{}
	for _, s := range sets {
		if s.FilterSet.Known() {
			verdicts[s.Key.ID()] = s.FilterSet
		}
	}
	if len(verdicts) > 0 {
		inv.FilterSets = verdicts
	}
	return inv
}

// TestInventory_NightFilterSet is what lets a FLAT be gated on the clip filter at all.
//
// A flat has no sky: ClassifyFilterSet measures the night sky above the bias pedestal, and an
// evenly-illuminated panel is not that, so 0005 deliberately classifies LIGHT sets only. A flat must
// therefore inherit the verdict of the night it was shot on — and only when that night speaks with
// one voice. Two different verdicts on one night mean the clip filter was swapped mid-session, and
// nothing in the night key says which flat belongs to which half: the honest answer is unknown,
// which leaves today's ranking untouched.
func TestInventory_NightFilterSet(t *testing.T) {
	const nightA, nightB = "2026-07-29", "2026-08-02"

	tests := []struct {
		name    string
		inv     *Inventory
		session string
		want    filters.FilterSet
	}{
		{
			name:    "the night's single light set decides it",
			inv:     invWith(lightSet(nightA, 120_000, filters.FilterSetDualband)),
			session: nightA,
			want:    filters.FilterSetDualband,
		},
		{
			name: "several light sets that agree still decide it",
			inv: invWith(
				lightSet(nightA, 120_000, filters.FilterSetDualband),
				lightSet(nightA, 300_000, filters.FilterSetDualband),
			),
			session: nightA,
			want:    filters.FilterSetDualband,
		},
		{
			name: "a known verdict carries the night's unclassified sets with it",
			inv: invWith(
				lightSet(nightA, 120_000, filters.FilterSetBroadband),
				lightSet(nightA, 300_000, filters.FilterSetUnknown),
			),
			session: nightA,
			want:    filters.FilterSetBroadband,
		},
		{
			name: "the clip filter was swapped mid-night — no honest verdict",
			inv: invWith(
				lightSet(nightA, 120_000, filters.FilterSetDualband),
				lightSet(nightA, 300_000, filters.FilterSetBroadband),
			),
			session: nightA,
			want:    filters.FilterSetUnknown,
		},
		{
			name: "only the asked-for night counts",
			inv: invWith(
				lightSet(nightA, 120_000, filters.FilterSetDualband),
				lightSet(nightB, 120_000, filters.FilterSetBroadband),
			),
			session: nightB,
			want:    filters.FilterSetBroadband,
		},
		{
			// The single-night scan, which is most captures: Session is "" on every set, so the
			// undated bucket must resolve exactly like a named night.
			name:    "a single-night scan resolves the empty night key",
			inv:     invWith(lightSet("", 120_000, filters.FilterSetDualband), calSet("", Flat, 2000)),
			session: "",
			want:    filters.FilterSetDualband,
		},
		{
			name:    "a night with no light set at all",
			inv:     invWith(lightSet(nightA, 120_000, filters.FilterSetDualband), calSet(nightB, Flat, 2000)),
			session: nightB,
			want:    filters.FilterSetUnknown,
		},
		{
			name:    "nothing was measured",
			inv:     invWith(lightSet(nightA, 120_000, filters.FilterSetUnknown)),
			session: nightA,
			want:    filters.FilterSetUnknown,
		},
		{
			name:    "no inventory",
			inv:     nil,
			session: nightA,
			want:    filters.FilterSetUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.inv.NightFilterSet(tt.session))
		})
	}
}

// TestInventory_NightFilterSet_ReadsTheDurableMap: Sets are rebuilt from Frames by several code
// paths and lose their projected FilterSet (see TestApplyFilterSets_SurvivesSetRebuild). The night
// verdict must survive that, so it reads Inventory.FilterSets and not the Set field.
func TestInventory_NightFilterSet_ReadsTheDurableMap(t *testing.T) {
	inv := invWith(lightSet("2026-07-29", 120_000, filters.FilterSetDualband))
	require.Equal(t, filters.FilterSetDualband, inv.NightFilterSet("2026-07-29"))

	for i := range inv.Sets { // what a rebuild leaves behind
		inv.Sets[i].FilterSet = ""
	}

	assert.Equal(t, filters.FilterSetDualband, inv.NightFilterSet("2026-07-29"))
}

// cfaStarfield builds an RGGB mosaic whose sky sits at the given levels and where starFrac of the
// cells carry an extra starADU — the one-sided bright contamination every real light frame has.
func cfaStarfield(r, g, b, starFrac, starADU float64) *fits.Image {
	const w, h = 64, 64
	pix := make([]float32, w*h)
	cellsPerRow := w / 2
	starEvery := int(1 / starFrac)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v float64
			switch {
			case y%2 == 0 && x%2 == 0:
				v = r
			case y%2 == 1 && x%2 == 1:
				v = b
			default:
				v = g
			}
			if cell := (y/2)*cellsPerRow + x/2; starEvery > 0 && cell%starEvery == 0 {
				v += starADU
			}
			pix[y*w+x] = float32(v)
		}
	}
	return &fits.Image{W: w, H: h, C: 1, Pix: [][]float32{pix}}
}

// A light frame's bright pixels are stars, not sky. Reading the plain MEAN of every photosite
// reports the stars too, and on a real dual-band night that inflation was enough to push the
// amplitude out of the dual-band band and into the "no verdict" gap (measured on IC 1848: 9.1 ADU
// per 120 s read robustly, 10.9 read as a mean, against a 10.0 threshold).
func TestCFAChannelSky_ReadsSkyNotStars(t *testing.T) {
	const sky, stars, starADU = 512.0, 0.05, 40.0
	im := cfaStarfield(sky, sky, sky, stars, starADU)

	got, ok := cfaChannelSky(im.Pix[0], im.W, im.H, "RGGB")
	require.True(t, ok)

	assert.InDelta(t, sky, got.R, 0.5, "the sky level, not the sky plus a share of every star")
	assert.InDelta(t, sky, got.G, 0.5)
	assert.InDelta(t, sky, got.B, 0.5)
	assert.Less(t, got.R, sky+stars*starADU,
		"a plain mean would have landed here — %.1f ADU above the sky", stars*starADU)
}

// The regression that sent a real dual-band capture down the broadband road: the sky IS dual-band
// faint, but a dense star field drags the mean across the threshold and the night ends up
// unclassified — which every consumer reads as "no evidence" and finishes as plain colour.
func TestFilterSetAnnotation_DenseStarFieldStillClassifies(t *testing.T) {
	// Sky above the pedestal: R=12, G=9.6, B=5.6 -> amplitude 9.07, inside the dual-band band and
	// red-dominant. The 5% star fraction adds 2.0 ADU to a mean, which would read 11.07: no verdict.
	light := oscFrame("light.fits", Light, 120_000)
	bias := oscFrame("bias.fits", Bias, 0)
	inv := oscInventory(light, bias)
	load := loaderFor(map[string]*fits.Image{
		"light.fits": cfaStarfield(testBiasADU+12, testBiasADU+9.6, testBiasADU+5.6, 0.05, 40),
		"bias.fits":  cfaImage(testBiasADU, testBiasADU, testBiasADU),
	})

	annotateFilterSets(inv, nil, load)

	assert.Equal(t, filters.FilterSetDualband, inv.FilterSets[setIDFor(t, inv, Light, 120_000)],
		"the stars must not decide the filter set")
}
