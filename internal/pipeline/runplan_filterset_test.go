package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/calib"
	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// oscInvWithBias is oscInv plus the bias frames the scan needs to have anything to calibrate with.
func oscInvWithBias(night string, verdict filters.FilterSet) *inspect.Inventory {
	inv := oscInv(night, verdict)
	biasKey := inspect.SetKey{Type: inspect.Bias, Gain: 100, Offset: 50, Bin: 1, Color: true}
	inv.Sets = append(inv.Sets, inspect.Set{Key: biasKey, Count: 20,
		Frames: []*inspect.Frame{{Type: inspect.Bias, Gain: 100, Offset: 50, BinX: 1, Bayer: "RGGB"}}})
	return inv
}

// libFlat is a library flat master for the OSC config. Library masters always read filter-set
// unknown — master_frames has no column for it.
func libFlat(fs filters.FilterSet) calib.Master {
	return calib.Master{
		Type: calib.MasterFlat, Filter: filters.Color, ExposureMs: 2000, Gain: 100, Offset: 50,
		Bin: 1, FrameCount: 30, Path: "/library/master_FLAT_RGB.fits", FilterSet: fs,
	}
}

// TestRunPlanPreview_MastersPerSet pins what the pre-run plan tells the user about EACH light set's
// calibration — the card's visibility half. Before this, a group's row could say "flat: library" and
// nothing else: not which night it came from, not which clip filter it was shot through, and above
// all not that the run was about to refuse it.
func TestRunPlanPreview_MastersPerSet(t *testing.T) {
	const night = "2026-07-29"

	tests := []struct {
		name             string
		verdict          filters.FilterSet
		lib              []calib.Master
		force            bool
		wantFlat         bool
		wantFilterSt     filters.FilterSet
		wantFlatFallback bool
		wantNote         string
	}{
		{
			// The clean case: the flat agrees with the lights. No chip, and the plan still names the
			// set so the user can see WHY it was accepted.
			name:             "a same-set flat is planned without a warning",
			verdict:          filters.FilterSetDualband,
			lib:              []calib.Master{libFlat(filters.FilterSetDualband)},
			wantFlat:         true,
			wantFilterSt:     filters.FilterSetDualband,
			wantFlatFallback: false,
		},
		{
			// The card's bug, surfaced before the run starts rather than discovered in the output.
			name:             "a cross-set flat is refused and the plan says so",
			verdict:          filters.FilterSetDualband,
			lib:              []calib.Master{libFlat(filters.FilterSetBroadband)},
			wantFlat:         false,
			wantFilterSt:     filters.FilterSetDualband,
			wantFlatFallback: true,
			wantNote:         "excluded",
		},
		{
			// force_calibration_frames means "apply my masters anyway". The plan must then show the
			// flat AND keep the chip: the user gets their master, and the warning they earned.
			name:             "forcing applies the cross-set flat but keeps the warning",
			verdict:          filters.FilterSetDualband,
			lib:              []calib.Master{libFlat(filters.FilterSetBroadband)},
			force:            true,
			wantFlat:         true,
			wantFilterSt:     filters.FilterSetDualband,
			wantFlatFallback: true,
			wantNote:         "forced flat",
		},
		{
			// Nothing measured → nothing gated. This is every monochrome capture and every colour
			// night the pixels could not settle, and it must plan exactly as it did before the card.
			name:             "an unmeasured night plans exactly as before",
			verdict:          filters.FilterSetUnknown,
			lib:              []calib.Master{libFlat(filters.FilterSetUnknown)},
			wantFlat:         true,
			wantFilterSt:     "",
			wantFlatFallback: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PreviewRunPlan(context.Background(), nil,
				planScanner{oscInvWithBias(night, tt.verdict)}, &fakeMasterLib{masters: tt.lib},
				nil, "", 0.5, tt.force, nil)
			require.NoError(t, err)

			require.Len(t, plan.Channels, 1)
			require.Len(t, plan.Channels[0].Groups, 1)
			g := plan.Channels[0].Groups[0]

			assert.Equal(t, tt.wantFilterSt, g.FilterSet, "the plan names the lights' own clip filter")
			assert.Equal(t, tt.wantFlatFallback, g.FlatFallback, "the warning chip")
			if tt.wantFlat {
				require.NotNil(t, g.Flat, "a flat was expected")
				require.NotNil(t, g.Flat.Master)
				assert.Equal(t, 30, g.Flat.Master.FrameCount, "the plan names the flat's depth")
			} else {
				assert.Nil(t, g.Flat, "the refused flat must not be presented as planned")
			}
			// The bias is capture-built here, and its depth is part of the per-set detail.
			require.NotNil(t, g.Bias)
			require.NotNil(t, g.Bias.Master)
			assert.Equal(t, 20, g.Bias.Master.FrameCount)
			if tt.wantNote != "" {
				assert.Contains(t, joinPlanNotes(g.Notes), tt.wantNote)
			}
		})
	}
}

// TestRunPlanPreview_FallbackFlagsABorrowedNight: the chip is not only about clip filters. A flat
// borrowed from another night is the older, quieter fallback (dust moves between nights), and it has
// always been buried in a note. The plan flags it structurally so the UI can raise it.
func TestRunPlanPreview_FallbackFlagsABorrowedNight(t *testing.T) {
	inv := oscInvWithBias("2026-07-29", filters.FilterSetUnknown)
	otherNight := libFlat(filters.FilterSetUnknown)
	otherNight.Session = "2026-08-02"
	otherNight.Path = "" // a capture-built per-night flat, as a multi-night scan produces

	plan, err := PreviewRunPlan(context.Background(), nil, planScanner{inv},
		&fakeMasterLib{masters: []calib.Master{otherNight}}, nil, "", 0.5, false, nil)
	require.NoError(t, err)

	g := plan.Channels[0].Groups[0]
	require.NotNil(t, g.Flat, "the borrowed flat is still applied — it is better than none")
	assert.True(t, g.FlatFallback, "but the user is told it came from another night")
}

// TestRunPlanPreview_NoFlatAtAllIsAFallback: a run with no usable flat is the loudest fallback of
// all, and it used to be indistinguishable from a clean plan in the payload.
func TestRunPlanPreview_NoFlatAtAllIsAFallback(t *testing.T) {
	plan, err := PreviewRunPlan(context.Background(), nil,
		planScanner{oscInvWithBias("2026-07-29", filters.FilterSetUnknown)}, &fakeMasterLib{},
		nil, "", 0.5, false, nil)
	require.NoError(t, err)

	g := plan.Channels[0].Groups[0]
	require.Nil(t, g.Flat)
	assert.True(t, g.FlatFallback)
}

func joinPlanNotes(notes []string) string {
	out := ""
	for _, n := range notes {
		out += n + " | "
	}
	return out
}
