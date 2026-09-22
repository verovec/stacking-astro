package inspect

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/fits"
	"github.com/verove-jordan/astronomy/internal/fits/fitstest"
)

// mosaicPixels builds a genuine CFA frame: the two greens agree, R and B sit apart from them and
// from each other — the structure a colour sensor produces and a monochrome one cannot.
func mosaicPixels(w, h int) []uint16 {
	pix := make([]uint16, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v uint16
			switch {
			case y%2 == 0 && x%2 == 0:
				v = 5300 // R
			case y%2 == 1 && x%2 == 1:
				v = 5020 // B
			default:
				v = 5040 // G1 / G2
			}
			pix[y*w+x] = v
		}
	}
	return pix
}

// flatPixels is one uniform response — a monochrome sensor, whatever its header claims.
func flatPixels(w, h int) []uint16 {
	pix := make([]uint16, w*h)
	for i := range pix {
		pix[i] = 5000
	}
	return pix
}

func imageOf(pix []uint16, w, h int) *fits.Image {
	out := make([]float32, len(pix))
	for i, v := range pix {
		out[i] = float32(v)
	}
	return &fits.Image{W: w, H: h, C: 1, Pix: [][]float32{out}}
}

// TestClearSpuriousBayer_DualbandOSC is the card's core case, and the data-loss bug it closes.
//
// An ASI2600MC behind an L-eXtreme dual-band clip writes FILTER='L-eXtreme'. That name is not a
// wheel slot, but IsMono says otherwise, so the old veto stripped BAYERPAT from those lights AND
// dragged the scan's shared calibration frames down with them — destroying a colour session and its
// calibration in one pass, silently. The pixels say plainly that this is a mosaic.
func TestClearSpuriousBayer_DualbandOSC(t *testing.T) {
	const w, h = 16, 16
	mosaic := imageOf(mosaicPixels(w, h), w, h)

	broadband := &Frame{Path: "bb.fits", Type: Light, Bayer: "RGGB", Instrument: "ASI2600MC"}
	dualband := &Frame{
		Path: "db.fits", Type: Light, Bayer: "RGGB",
		Filter: "L-eXtreme", Instrument: "ASI2600MC",
	}
	flat := &Frame{Path: "flat.fits", Type: Flat, Bayer: "RGGB", Instrument: "ASI2600MC"}
	dark := &Frame{Path: "dark.fits", Type: Dark, Bayer: "RGGB", Instrument: "ASI2600MC"}

	inv := &Inventory{Frames: []*Frame{broadband, dualband, flat, dark}}
	clearSpuriousBayerWith(inv, func(string) (*fits.Image, error) { return mosaic, nil })

	for _, fr := range inv.Frames {
		assert.Equal(t, "RGGB", fr.Bayer, "%s must keep its Bayer pattern", fr.Path)
	}
	assert.Empty(t, inv.Warnings, "nothing was spurious, so there is nothing to warn about")

	inv.ColorModel = colorModel(inv)
	assert.Equal(t, ColorOSC, inv.ColorModel, "a dual-band colour session is still a colour session")
}

// TestClearSpuriousBayer_MonoRigUnchanged is the regression guard. The ASI1600MM case is the reason
// the veto exists, and it must behave exactly as before: the spurious card is cleared on the
// filtered lights and on the session's calibration frames, with the same warning.
func TestClearSpuriousBayer_MonoRigUnchanged(t *testing.T) {
	const w, h = 16, 16
	flatResponse := imageOf(flatPixels(w, h), w, h)

	lightL := &Frame{Path: "l.fits", Type: Light, Bayer: "RGGB", Filter: "L", Instrument: "ASI1600MM"}
	lightHa := &Frame{Path: "ha.fits", Type: Light, Bayer: "RGGB", Filter: "Ha", Instrument: "ASI1600MM"}
	bias := &Frame{Path: "bias.fits", Type: Bias, Bayer: "RGGB", Instrument: "ASI1600MM"}

	inv := &Inventory{Frames: []*Frame{lightL, lightHa, bias}}
	clearSpuriousBayerWith(inv, func(string) (*fits.Image, error) { return flatResponse, nil })

	for _, fr := range inv.Frames {
		assert.Empty(t, fr.Bayer, "%s must lose its spurious BAYERPAT", fr.Path)
	}
	require.Len(t, inv.Warnings, 1)
	assert.Contains(t, inv.Warnings[0], "filter wheel (mono rig)")
	assert.Contains(t, inv.Warnings[0], "3 frame(s)")

	inv.ColorModel = colorModel(inv)
	assert.Equal(t, ColorMono, inv.ColorModel)
}

// TestClearSpuriousBayer_PerInstrument: one rig's evidence must never reach another rig's frames.
// Scan-wide scoping is what let a mono session strip the BAYERPAT off a colour camera's calibration
// frames sitting in the same folder — the ordinary mixed intake this epic is built for.
func TestClearSpuriousBayer_PerInstrument(t *testing.T) {
	const w, h = 16, 16
	mosaic := imageOf(mosaicPixels(w, h), w, h)
	flatResponse := imageOf(flatPixels(w, h), w, h)

	monoLightL := &Frame{Path: "mono_l.fits", Type: Light, Bayer: "RGGB", Filter: "L", Instrument: "ASI1600MM"}
	monoBias := &Frame{Path: "mono_bias.fits", Type: Bias, Bayer: "RGGB", Instrument: "ASI1600MM"}
	oscLight := &Frame{Path: "osc.fits", Type: Light, Bayer: "RGGB", Instrument: "ASI2600MC"}
	oscFlat := &Frame{Path: "osc_flat.fits", Type: Flat, Bayer: "RGGB", Instrument: "ASI2600MC"}

	inv := &Inventory{Frames: []*Frame{monoLightL, monoBias, oscLight, oscFlat}}
	clearSpuriousBayerWith(inv, func(path string) (*fits.Image, error) {
		if path == "mono_l.fits" || path == "mono_bias.fits" {
			return flatResponse, nil
		}
		return mosaic, nil
	})

	assert.Empty(t, monoLightL.Bayer, "the mono rig's spurious card is still cleared")
	assert.Empty(t, monoBias.Bayer, "including its own calibration")
	assert.Equal(t, "RGGB", oscLight.Bayer, "the colour rig is untouched by the mono rig's evidence")
	assert.Equal(t, "RGGB", oscFlat.Bayer, "and so is its calibration")
}

// TestClearSpuriousBayer_DeclinedKeepsOldBehaviour: when the pixels cannot settle it, the veto still
// fires — every existing mono session depends on that — but it says so instead of acting in silence.
func TestClearSpuriousBayer_DeclinedKeepsOldBehaviour(t *testing.T) {
	light := &Frame{Path: "l.fits", Type: Light, Bayer: "RGGB", Filter: "L", Instrument: "ASI1600MM"}
	inv := &Inventory{Frames: []*Frame{light}}

	clearSpuriousBayerWith(inv, func(string) (*fits.Image, error) { return nil, assert.AnError })

	assert.Empty(t, light.Bayer, "unreadable pixels keep the pre-existing behaviour")
	require.Len(t, inv.Warnings, 2)
	assert.Contains(t, inv.Warnings[1], "do not settle")
}

// TestIsOSCDir_DualbandDir walks the real scanner over a dual-band colour folder on disk. It used to
// answer false — the veto had already stripped the evidence by the time the question was asked.
func TestIsOSCDir_DualbandDir(t *testing.T) {
	const w, h = 16, 16

	t.Run("a dual-band colour folder is one-shot-color", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"l_1.fits", "l_2.fits", "l_3.fits"} {
			fitstest.WritePixels(t, dir, name, w, h, mosaicPixels(w, h), map[string]string{
				"IMAGETYP": "'Light Frame'", "EXPTIME": "120.0", "GAIN": "100",
				"BAYERPAT": "'RGGB'", "FILTER": "'L-eXtreme'", "INSTRUME": "'ZWO ASI2600MC Pro'",
			})
		}
		assert.True(t, IsOSCDir(dir))
	})

	t.Run("a mono filter-wheel folder is still not", func(t *testing.T) {
		dir := t.TempDir()
		for i, filter := range []string{"L", "R", "G"} {
			fitstest.WritePixels(t, dir, filepath.Base(string(rune('a'+i))+".fits"), w, h,
				flatPixels(w, h), map[string]string{
					"IMAGETYP": "'Light Frame'", "EXPTIME": "120.0", "GAIN": "139",
					"BAYERPAT": "'RGGB'", "FILTER": "'" + filter + "'", "INSTRUME": "'ZWO ASI1600MM Pro'",
				})
		}
		assert.False(t, IsOSCDir(dir))
	})
}
