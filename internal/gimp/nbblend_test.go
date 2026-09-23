package gimp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emissionInputs is a composite carrying both emission screens — the mixed-capture shape.
func emissionInputs() Inputs {
	return Inputs{
		Base: "/w/base.tif", Color: true,
		Ha:   "/w/ha.tif",
		OIII: "/w/oiii.tif", OIIIScreen: 0.50,
	}
}

// nbCompose renders the composite script for these inputs at the default Ha screen opacity.
func nbCompose(in Inputs) string {
	return composeScript(in, nil, 0.42, 0,
		&Result{Xcf: "/w/o.xcf", Tif: "/w/o.tif", Png: "/w/o.png"})
}

// TestComposeScript_NBDefaultsAreByteIdentical is the degradation contract for both new knobs. A run
// that never asks for them must emit the exact script it emitted before they existed — the zero
// value of NBBlend must mean "unset", not "blend at zero", or every existing composite would
// silently lose its emission layers.
func TestComposeScript_NBDefaultsAreByteIdentical(t *testing.T) {
	before := nbCompose(emissionInputs())

	tests := []struct {
		name string
		in   Inputs
	}{
		{"both knobs absent (zero values)", emissionInputs()},
		{"explicit defaults", func() Inputs { i := emissionInputs(); i.NBBlend, i.OIIIBoost = 1, 1; return i }()},
		{"boost at its off value only", func() Inputs { i := emissionInputs(); i.OIIIBoost = 1; return i }()},
		{"blend at its full value only", func() Inputs { i := emissionInputs(); i.NBBlend = 1; return i }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, before, nbCompose(tt.in))
		})
	}
}

// TestComposeScript_NBBlendScalesBothScreens: the PO's one-slider ask. Mixing dual-band and
// broadband at equal strength drags the dual-band's noise into the final, so the whole narrowband
// contribution needs a single weight — scaling BOTH emission opacities together, because turning
// them down one at a time changes the Ha/[OIII] colour balance as a side effect.
func TestComposeScript_NBBlendScalesBothScreens(t *testing.T) {
	in := emissionInputs()
	in.NBBlend = 0.5

	got := nbCompose(in)

	assert.Contains(t, got, "(gimp-layer-set-opacity ha 21)", "0.42 * 0.5")
	assert.Contains(t, got, "(gimp-layer-set-opacity oiii 25)", "0.50 * 0.5")
}

// At zero the narrowband contribution is gone entirely: the layers are not loaded at all, leaving
// the pure broadband base. That is the "show me the base alone" end of the slider, and it must not
// leave a zero-opacity layer behind in the script.
func TestComposeScript_NBBlendZeroDropsTheLayers(t *testing.T) {
	in := emissionInputs()
	in.NBBlend = 0.0001 // a real zero is the unset sentinel; this is the smallest meaningful ask

	got := nbCompose(in)

	assert.NotContains(t, got, "ha.tif", "a vanishing screen is not loaded")
	assert.NotContains(t, got, "oiii.tif")
	assert.Contains(t, got, "base.tif", "the broadband base always remains")
}

// TestComposeScript_OIIIBoostEmitsTheCurve: the boost reaches GIMP as an explicit curve on the OIII
// layer, applied BEFORE the teal tint and the screen so the roll-off acts on the layer's own values.
func TestComposeScript_OIIIBoostEmitsTheCurve(t *testing.T) {
	in := emissionInputs()
	in.OIIIBoost = 1.35

	got := nbCompose(in)

	require.Contains(t, got, "gimp-drawable-curves-explicit oiii")
	curve := strings.Index(got, "gimp-drawable-curves-explicit oiii")
	tint := strings.Index(got, "gimp-drawable-levels oiii HISTOGRAM-RED")
	screen := strings.Index(got, "gimp-layer-set-mode oiii")
	assert.Less(t, curve, tint, "the boost must precede the teal tint")
	assert.Less(t, curve, screen, "the boost must precede the screen")
	assert.NotContains(t, got, "gimp-drawable-curves-explicit ha", "the boost is [OIII]-only")
}
