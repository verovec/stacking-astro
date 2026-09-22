package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
	"github.com/verove-jordan/astronomy/internal/store"
)

// oscInv builds a one-shot-colour inventory of one light set on one night, carrying the verdict the
// pixel classifier would have reached (Inventory.FilterSets is the durable record; the Set field is
// only its projection, and several code paths drop it).
func oscInv(night string, verdict filters.FilterSet) *inspect.Inventory {
	key := inspect.SetKey{
		Type: inspect.Light, Object: "NGC7000", Filter: filters.Color, ExposureMs: 120_000,
		Gain: 100, Offset: 50, Bin: 1, TempBucket: -10, Session: night, Color: true,
	}
	frames := []*inspect.Frame{
		{Path: "/cur/osc_001.fits", Type: inspect.Light, Filter: filters.Color, Bayer: "RGGB",
			ExposureMs: 120_000, Gain: 100, Offset: 50, BinX: 1, BinY: 1, TempMilliC: -10000,
			HasTemp: true, Object: "NGC7000", Session: night},
		{Path: "/cur/osc_002.fits", Type: inspect.Light, Filter: filters.Color, Bayer: "RGGB",
			ExposureMs: 120_000, Gain: 100, Offset: 50, BinX: 1, BinY: 1, TempMilliC: -10000,
			HasTemp: true, Object: "NGC7000", Session: night},
	}
	inv := &inspect.Inventory{
		Frames:     frames,
		Sets:       []inspect.Set{{Key: key, Frames: frames, Count: 2}},
		ColorModel: inspect.ColorOSC,
	}
	if verdict.Known() {
		inv.FilterSets = map[string]filters.FilterSet{key.ID(): verdict}
		inv.Sets[0].FilterSet = verdict
	}
	return inv
}

// TestBuildReusePlan_CurrentGroupsCarryTheFilterSet is the link between detection and matching. The
// run matches masters per GROUP, not per inspected set, so a verdict that stops at the Inventory
// never reaches calib — the gate would be built, wired and permanently unreachable.
func TestBuildReusePlan_CurrentGroupsCarryTheFilterSet(t *testing.T) {
	tests := []struct {
		name    string
		verdict filters.FilterSet
		want    filters.FilterSet
	}{
		{"a dual-band night reaches the matcher", filters.FilterSetDualband, filters.FilterSetDualband},
		{"a broadband night reaches the matcher", filters.FilterSetBroadband, filters.FilterSetBroadband},
		// The EMPTY value, not the "unknown" literal: calib.RefFor carries one spelling of "no
		// verdict" so no consumer has to know which of the two it will be handed.
		{"an unmeasured night carries no verdict", filters.FilterSetUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := oscInv("2026-07-29", tt.verdict)

			plan, err := buildReusePlan(context.Background(), ReuseConfig{}, inv, 1,
				targetQuery{Object: "NGC7000"})

			require.NoError(t, err)
			groups := plan.byFilter[filters.Color]
			require.Len(t, groups, 1)
			assert.Equal(t, tt.want, groups[0].ref().FilterSet)
			assert.Equal(t, tt.want.Known(), groups[0].ref().FilterSet.Known())
			assert.Equal(t, inv.Sets[0].Key, groups[0].ref().Key, "the key is untouched")
		})
	}
}

// TestBuildReusePlan_PriorGroupsStayUnknown: prior frames come from the catalog, which stores no
// filter set (frames has no such column). Guessing one from the CURRENT night would be worse than
// unknown — it would gate a prior night's flats on a clip filter nobody ever measured on it. Unknown
// reproduces today's ranking exactly, which is the safe direction.
func TestBuildReusePlan_PriorGroupsStayUnknown(t *testing.T) {
	dir := t.TempDir()
	prov := &fakeProvider{lights: []store.FrameRow{
		oscPriorRow(2, priorFile(t, dir, "s2/osc_001.fits")),
		oscPriorRow(2, priorFile(t, dir, "s2/osc_002.fits")),
	}}

	plan, err := buildReusePlan(context.Background(), ReuseConfig{Provider: prov, ConeDeg: 0.5},
		oscInv("2026-07-29", filters.FilterSetDualband), 1, targetQuery{Object: "NGC7000"})

	require.NoError(t, err)
	groups := plan.byFilter[filters.Color]
	require.Len(t, groups, 2, "the current group plus the prior one")
	var current, prior int
	for _, g := range groups {
		if g.Current {
			current++
			assert.Equal(t, filters.FilterSetDualband, g.ref().FilterSet)
			continue
		}
		prior++
		assert.False(t, g.ref().FilterSet.Known(),
			"the catalog never recorded this night's clip filter")
	}
	assert.Equal(t, 1, current)
	assert.Equal(t, 1, prior)
}

func oscPriorRow(session int64, path string) store.FrameRow {
	return store.FrameRow{
		SessionID: session, Path: path, FrameType: "LIGHT", Filter: filters.Color,
		ExposureMs: 120_000, Gain: 100, Offset: 50, Bin: 1, TempMilliC: -10000, HasTemp: true,
	}
}
