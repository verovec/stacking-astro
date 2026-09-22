package inspect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// monoLight is a filter-wheel exposure: one plane, no Bayer pattern.
func monoLight(filter string) *Frame {
	return &Frame{Type: Light, Filter: filter, ExposureMs: 120000, Gain: 139, Offset: 10, BinX: 1, BinY: 1}
}

// colorLight is a one-shot-color exposure. Channels >= 3 stands in for every colour shape the scan
// recognises (CFA mosaic, developed raw, RGB FITS) — Frame.IsColor is what the resolver keys on.
func colorLight() *Frame {
	return &Frame{Type: Light, Channels: 3, ExposureMs: 120000, Gain: 139, Offset: 10, BinX: 1, BinY: 1}
}

// bayerlessDark is the calibration frame that makes a whole-inventory "exclude every mono frame"
// unsafe: clearSpuriousBayer strips BAYERPAT from calibration frames whenever the scan shows any
// filter-wheel evidence, and plenty of OSC capture software never writes the card on darks at all.
// Such a dark reads as mono by every test we have, yet belongs to the colour rig.
func bayerlessDark() *Frame {
	return &Frame{Type: Dark, ExposureMs: 120000, Gain: 139, Offset: 10, BinX: 1, BinY: 1}
}

// seed builds an inventory the way finalizeInventory does: frames in, verdict computed, sets keyed.
func seed(frames ...*Frame) *Inventory {
	inv := &Inventory{Frames: frames}
	inv.ColorModel = colorModel(inv)
	nameColorChannel(inv)
	inv.Sets = buildSets(inv.Frames)
	return inv
}

func countTypes(inv *Inventory, typ FrameType) int {
	n := 0
	for _, fr := range inv.Frames {
		if fr.Type == typ {
			n++
		}
	}
	return n
}

// TestResolveColorModel pins the request-level colour knob against every scan verdict.
//
// The auto rows are the regression guard: auto must leave the inventory EXACTLY as the scan left it,
// because each mode entry (deepsky, mosaic, comet, per-stage rerun) still owns its own handling of a
// mixed folder and those handlings differ from one another. Anything this function did for auto would
// silently change all four.
func TestResolveColorModel(t *testing.T) {
	tests := []struct {
		name       string
		choice     ColorChoice
		inv        func() *Inventory
		wantErr    string
		wantModel  ColorModel
		wantLights int
		wantDarks  int
		wantWarn   string
	}{
		{
			name:       "auto leaves a mono scan untouched",
			choice:     ChoiceAuto,
			inv:        func() *Inventory { return seed(monoLight("L"), monoLight("R")) },
			wantModel:  ColorMono,
			wantLights: 2,
		},
		{
			name:       "auto leaves a colour scan untouched",
			choice:     ChoiceAuto,
			inv:        func() *Inventory { return seed(colorLight(), colorLight()) },
			wantModel:  ColorOSC,
			wantLights: 2,
		},
		{
			name:       "auto leaves a mixed scan mixed for the mode entry to resolve",
			choice:     ChoiceAuto,
			inv:        func() *Inventory { return seed(monoLight("L"), colorLight()) },
			wantModel:  ColorMixed,
			wantLights: 2,
		},
		{
			name:       "mono on a mono scan changes nothing",
			choice:     ChoiceMono,
			inv:        func() *Inventory { return seed(monoLight("L"), monoLight("R")) },
			wantModel:  ColorMono,
			wantLights: 2,
		},
		{
			name:       "mono on a mixed scan drops the colour lights",
			choice:     ChoiceMono,
			inv:        func() *Inventory { return seed(monoLight("L"), colorLight()) },
			wantModel:  ColorMono,
			wantLights: 1,
			wantWarn:   "monochrome stack",
		},
		{
			name:    "mono on a colour scan is an honest error, not an empty run",
			choice:  ChoiceMono,
			inv:     func() *Inventory { return seed(colorLight(), colorLight()) },
			wantErr: "no monochrome light frames",
		},
		{
			name:       "osc on a colour scan changes nothing",
			choice:     ChoiceOSC,
			inv:        func() *Inventory { return seed(colorLight(), colorLight()) },
			wantModel:  ColorOSC,
			wantLights: 2,
		},
		{
			name:       "osc on a mixed scan drops the mono lights",
			choice:     ChoiceOSC,
			inv:        func() *Inventory { return seed(monoLight("L"), colorLight()) },
			wantModel:  ColorOSC,
			wantLights: 1,
			wantWarn:   "colour stack",
		},
		{
			name:    "osc on a mono scan is an honest error, not an empty run",
			choice:  ChoiceOSC,
			inv:     func() *Inventory { return seed(monoLight("L"), monoLight("R")) },
			wantErr: "no one-shot-color light frames",
		},
		{
			name:       "osc keeps a Bayer-less dark — calibration frames are not lights",
			choice:     ChoiceOSC,
			inv:        func() *Inventory { return seed(colorLight(), monoLight("L"), bayerlessDark()) },
			wantModel:  ColorOSC,
			wantLights: 1,
			wantDarks:  1,
			wantWarn:   "colour stack",
		},
		{
			name:      "an explicit choice on a calibration-only scan is not an error",
			choice:    ChoiceMono,
			inv:       func() *Inventory { return seed(bayerlessDark()) },
			wantModel: ColorMono,
			wantDarks: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := tt.inv()
			err := ResolveColorModel(inv, tt.choice)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, inv.ColorModel, "resolved colour model")
			assert.Equal(t, tt.wantLights, countTypes(inv, Light), "lights kept")
			assert.Equal(t, tt.wantDarks, countTypes(inv, Dark), "darks kept")
			assert.Equal(t, len(inv.Sets), len(buildSets(inv.Frames)), "sets rebuilt after the exclusion")

			if tt.wantWarn == "" {
				assert.Empty(t, inv.Warnings, "no exclusion means no warning")
				return
			}
			require.NotEmpty(t, inv.Warnings, "an exclusion must be reported")
			assert.Contains(t, strings.Join(inv.Warnings, "\n"), tt.wantWarn)
		})
	}
}

// TestResolveColorModel_NamesTheColourChannel covers the trap in promoting a MIXED scan to colour:
// nameColorChannel runs at scan time and only for an already-OSC verdict, so the colour lights of a
// mixed folder reach the pipeline with an empty Filter — which the whole channel machinery reads as
// "no filter" and mis-stacks. Promotion has to name them.
func TestResolveColorModel_NamesTheColourChannel(t *testing.T) {
	inv := seed(monoLight("L"), colorLight())
	require.Equal(t, ColorMixed, inv.ColorModel)
	require.NoError(t, ResolveColorModel(inv, ChoiceOSC))

	require.Len(t, inv.Frames, 1)
	assert.Equal(t, "RGB", inv.Frames[0].Filter, "the surviving colour light must carry the canonical channel name")
}

// TestResolveColorModel_DoesNotMutateSharedFrames pins the sharing invariant the whole
// Inventory-mutation family keeps: frames are handed out read-only by the ScanCache (cache.go warns
// that "a filter override would mutate them in place"), and this resolver runs at RUN time, long
// after the scan that cached them. Renaming through the pointer would rewrite the cached frame and
// leak into a later, unrelated inspection of the same folder.
func TestResolveColorModel_DoesNotMutateSharedFrames(t *testing.T) {
	shared := colorLight()
	inv := seed(monoLight("L"), shared)
	require.Equal(t, ColorMixed, inv.ColorModel)
	require.Equal(t, "", shared.Filter, "precondition: a mixed scan leaves its colour lights unnamed")

	require.NoError(t, ResolveColorModel(inv, ChoiceOSC))

	assert.Equal(t, "", shared.Filter, "the cached frame must be untouched")
	require.Len(t, inv.Frames, 1)
	assert.Equal(t, "RGB", inv.Frames[0].Filter, "the inventory carries a renamed COPY")
	assert.NotSame(t, shared, inv.Frames[0], "the rename must not be done through the shared pointer")
}

// TestParseColorChoice pins the closed enum the API validates against: absent/empty is auto (today's
// behaviour for every stored job), casing is tolerated, and anything else is rejected rather than
// silently falling back to auto.
func TestParseColorChoice(t *testing.T) {
	tests := []struct {
		in      string
		want    ColorChoice
		wantErr bool
	}{
		{in: "", want: ChoiceAuto},
		{in: "auto", want: ChoiceAuto},
		{in: "mono", want: ChoiceMono},
		{in: "osc", want: ChoiceOSC},
		{in: "OSC", want: ChoiceOSC},
		{in: "colour", wantErr: true},
		{in: "rgb", wantErr: true},
		{in: "mixed", wantErr: true}, // a scan verdict, never a user choice
	}
	for _, tt := range tests {
		t.Run("in="+tt.in, func(t *testing.T) {
			got, err := ParseColorChoice(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "color_model")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
