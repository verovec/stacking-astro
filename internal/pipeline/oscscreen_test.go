package pipeline

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/fits"
	"github.com/verove-jordan/astronomy/internal/mode"
)

// writeOSCTestFITS writes a 3-plane colour master, the shape a one-shot-colour stack has.
func writeOSCTestFITS(t *testing.T, dir, name string, w, h int, fill func(plane, x, y int) float32) string {
	t.Helper()
	im := fits.NewImage(w, h, 3)
	for c := 0; c < 3; c++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				im.Pix[c][y*w+x] = fill(c, x, y)
			}
		}
	}
	p := filepath.Join(dir, name)
	require.NoError(t, im.WriteFITS(p))
	return p
}

// A duo-band capture stacked beside a broadband one yields synthesized Ha/OIII channels, but the
// broadband reference they must be continuum-subtracted against is a COLOUR master named "RGB" —
// not the "R"/"G"/"B"/"L" a filter wheel produces. Without this the subtraction declines, the raw
// (continuum-dominated) line is screened, and the whole galaxy is washed pink instead of its
// emission being isolated.
func TestLineContinuumSubtract_OSCReference(t *testing.T) {
	const w, h = 512, 256
	// Red plane carries the Ha continuum, green plane the [OIII] continuum, and they differ so a
	// wrong-plane pick cannot pass by luck.
	redRamp := func(x int) float32 { return 0.1 + 0.5*float32(x)/float32(w) }
	greenRamp := func(x int) float32 { return 0.2 + 0.8*float32(x)/float32(w) }
	inBlob := func(x, y int) bool { return x < 64 && y < 64 }

	t.Run("Ha is continuum-subtracted against the master's RED plane", func(t *testing.T) {
		dir := t.TempDir()
		writeOSCTestFITS(t, dir, "combine_RGB.fits", w, h, func(c, x, y int) float32 {
			if c == 0 {
				return redRamp(x)
			}
			return greenRamp(x)
		})
		writeHaTestFITS(t, dir, "syn_Ha.fits", w, h, func(x, y int) float32 {
			v := 0.5 * redRamp(x)
			if inBlob(x, y) {
				v += 0.3
			}
			return v
		})

		hc, why := haContinuumSubtract(dir, map[string]string{"Ha": "syn_Ha", "RGB": "combine_RGB"}, dir)
		require.NotNil(t, hc, "subtraction declined: %s", why)
		assert.Equal(t, "RGB", hc.Ref)
		assert.InDelta(t, 0.5, hc.K, 0.01, "k must be fitted against the red plane")

		ex, err := fits.ReadImage(hc.ExcessPath)
		require.NoError(t, err)
		var contMax, blobMin float32 = 0, math.MaxFloat32
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := ex.Pix[0][y*w+x]
				if inBlob(x, y) {
					if v < blobMin {
						blobMin = v
					}
				} else if v > contMax {
					contMax = v
				}
			}
		}
		assert.Less(t, float64(contMax), 0.02, "continuum must cancel outside the emission blob")
		assert.Greater(t, float64(blobMin), 0.25, "the emission blob must survive")
	})

	t.Run("OIII is continuum-subtracted against the master's GREEN plane", func(t *testing.T) {
		dir := t.TempDir()
		writeOSCTestFITS(t, dir, "combine_RGB.fits", w, h, func(c, x, y int) float32 {
			if c == 0 {
				return redRamp(x)
			}
			return greenRamp(x)
		})
		writeHaTestFITS(t, dir, "syn_OIII.fits", w, h, func(x, y int) float32 {
			return 0.25 * greenRamp(x)
		})

		hc, why := oiiiContinuumSubtract(dir, map[string]string{"OIII": "syn_OIII", "RGB": "combine_RGB"}, dir)
		require.NotNil(t, hc, "subtraction declined: %s", why)
		assert.Equal(t, "RGB", hc.Ref)
		assert.InDelta(t, 0.25, hc.K, 0.01, "k must be fitted against the green plane, not the red one")
	})

	t.Run("a mono run still prefers its real R channel", func(t *testing.T) {
		dir := t.TempDir()
		writeHaTestFITS(t, dir, "combine_R.fits", w, h, func(x, y int) float32 { return redRamp(x) })
		writeOSCTestFITS(t, dir, "combine_RGB.fits", w, h, func(c, x, y int) float32 { return greenRamp(x) })
		writeHaTestFITS(t, dir, "combine_Ha.fits", w, h, func(x, y int) float32 { return 0.5 * redRamp(x) })

		hc, why := haContinuumSubtract(dir,
			map[string]string{"Ha": "combine_Ha", "R": "combine_R", "RGB": "combine_RGB"}, dir)
		require.NotNil(t, hc, "subtraction declined: %s", why)
		assert.Equal(t, "R", hc.Ref, "a real broadband filter outranks the colour master")
	})
}

// The colour path short-circuits resolvePalette to a pass-through, which left HaScreen/OIIIScreen
// false — so a mixed broadband + duo-band run stacked both lanes, synthesized Ha/OIII, and then
// composited the broadband base alone: the narrowband contributed nothing to the image.
func TestResolvePalette_ColourRunScreensItsEmissionChannels(t *testing.T) {
	osc := func() *mode.Preset { p := mode.For(mode.Deepsky); p.Color = mode.OSC; return &p }

	t.Run("plain colour run is unchanged", func(t *testing.T) {
		pal, _ := resolvePalette(osc(), map[string]string{"RGB": "master_RGB"})
		assert.True(t, pal.Color)
		assert.Equal(t, "rgb", pal.Name)
		assert.False(t, pal.HaScreen, "no Ha channel — nothing to screen")
		assert.False(t, pal.OIIIScreen)
		assert.False(t, pal.Narrowband)
	})

	t.Run("synthesized emission channels are screened over the colour base", func(t *testing.T) {
		pal, _ := resolvePalette(osc(), map[string]string{
			"RGB": "master_RGB", "Ha": duobandHaBase, "OIII": duobandOIIIBase})
		assert.True(t, pal.Color, "the broadband stack stays the colour base")
		assert.Equal(t, "rgb", pal.Name)
		assert.True(t, pal.HaScreen, "Ha must screen over the broadband base")
		assert.True(t, pal.OIIIScreen)
		assert.False(t, pal.Narrowband, "screening is not a mapped-narrowband render")
	})

	t.Run("a narrowband palette still maps the lines instead of screening them", func(t *testing.T) {
		p := osc()
		p.Palette = "hoo"
		pal, _ := resolvePalette(p, map[string]string{
			"RGB": "master_RGB", "Ha": duobandHaBase, "OIII": duobandOIIIBase})
		assert.Equal(t, "hoo", pal.Name)
		assert.True(t, pal.Narrowband)
		assert.False(t, pal.HaScreen, "under hoo, Ha IS the red base — screening it would double-count")
	})
}
