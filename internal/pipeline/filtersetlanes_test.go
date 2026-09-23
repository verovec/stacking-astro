package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// oscLightSetOn is a one-shot-colour light set: one night, one exposure, two frames.
func oscLightSetOn(night string, exposureMs int64) inspect.Set {
	key := inspect.SetKey{
		Type: inspect.Light, Object: "NGC7000", Filter: filters.Color, ExposureMs: exposureMs,
		Gain: 100, Offset: 50, Bin: 1, TempBucket: -10, Session: night, Color: true,
	}
	frames := []*inspect.Frame{
		{Path: "/cur/" + night + "_" + string(rune('a'+exposureMs%26)) + "_1.fits", Type: inspect.Light,
			Filter: filters.Color, Bayer: "RGGB", ExposureMs: exposureMs, Gain: 100, Offset: 50,
			BinX: 1, BinY: 1, TempMilliC: -10000, HasTemp: true, Object: "NGC7000", Session: night},
		{Path: "/cur/" + night + "_" + string(rune('a'+exposureMs%26)) + "_2.fits", Type: inspect.Light,
			Filter: filters.Color, Bayer: "RGGB", ExposureMs: exposureMs, Gain: 100, Offset: 50,
			BinX: 1, BinY: 1, TempMilliC: -10000, HasTemp: true, Object: "NGC7000", Session: night},
	}
	return inspect.Set{Key: key, Frames: frames, Count: len(frames)}
}

// oscScanOf assembles an OSC inventory from light sets and their verdicts (index-aligned).
func oscScanOf(sets []inspect.Set, verdicts []filters.FilterSet) *inspect.Inventory {
	inv := &inspect.Inventory{ColorModel: inspect.ColorOSC}
	m := map[string]filters.FilterSet{}
	for i, s := range sets {
		if i < len(verdicts) && verdicts[i].Known() {
			s.FilterSet = verdicts[i]
			m[s.Key.ID()] = verdicts[i]
		}
		inv.Sets = append(inv.Sets, s)
		inv.Frames = append(inv.Frames, s.Frames...)
	}
	if len(m) > 0 {
		inv.FilterSets = m
	}
	return inv
}

// TestPartitionGroups_FilterSet is the card's core: a folder holding BOTH a broadband and a
// dual-band exposure of one object must stack them as TWO lanes, not one.
//
// Today they land in the same lane. The clip filter is deliberately NOT part of SetKey (it is a
// measured value, and folding it into the key would churn the exclude_sets tokens the UI stores), so
// two OSC sets differing only by the filter in front of the sensor share a channel — and
// processChannelGroups co-registers and stacks them into ONE master. That is the silent soup this
// card exists to end: a full-continuum exposure averaged with two narrow emission windows.
//
// The partition fires ONLY when both sets are actually present and classified. A broadband-only
// capture, a dual-band-only capture (the ordinary L-eXtreme user) and anything unclassified must
// produce exactly the lane they produce today.
func TestPartitionGroups_FilterSet(t *testing.T) {
	const nightA, nightB = "2026-07-29", "2026-08-02"

	tests := []struct {
		name      string
		sets      []inspect.Set
		verdicts  []filters.FilterSet
		wantLanes []string
	}{
		{
			name:      "broadband + dual-band split into two lanes",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000), oscLightSetOn(nightB, 300_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetBroadband, filters.FilterSetDualband},
			wantLanes: []string{filters.Color, filters.Color + dualbandLaneSuffix},
		},
		{
			// Order must not matter: whichever set the scanner happens to list first, the broadband
			// lane keeps the plain name (it is the reference the dual-band master registers onto).
			name:      "the dual-band set listed first still takes the suffixed lane",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000), oscLightSetOn(nightB, 300_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetDualband, filters.FilterSetBroadband},
			wantLanes: []string{filters.Color, filters.Color + dualbandLaneSuffix},
		},
		{
			// The ordinary dual-band capture. There is nothing to separate it FROM, so it keeps the
			// single lane it has always had and the run is byte-identical.
			name:      "dual-band only — one lane, unchanged",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetDualband},
			wantLanes: []string{filters.Color},
		},
		{
			name:      "broadband only — one lane, unchanged",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetBroadband},
			wantLanes: []string{filters.Color},
		},
		{
			// Two broadband nights of one object are a multi-night merge, which is the WHOLE point of
			// the grouped path. They must stay in one lane and keep merging.
			name:      "two broadband nights stay one lane — this is the multi-night merge",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000), oscLightSetOn(nightB, 300_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetBroadband, filters.FilterSetBroadband},
			wantLanes: []string{filters.Color},
		},
		{
			// Nothing measured → nothing partitioned. Never guess which of two unclassified sets was
			// shot through what; merging is what this scan already does today.
			name:      "unclassified sets never partition",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000), oscLightSetOn(nightB, 300_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetUnknown, filters.FilterSetUnknown},
			wantLanes: []string{filters.Color},
		},
		{
			// Half-classified is still not evidence that the OTHER set is the opposite kind.
			name:      "one classified set and one unknown never partition",
			sets:      []inspect.Set{oscLightSetOn(nightA, 120_000), oscLightSetOn(nightB, 300_000)},
			verdicts:  []filters.FilterSet{filters.FilterSetDualband, filters.FilterSetUnknown},
			wantLanes: []string{filters.Color},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := oscScanOf(tt.sets, tt.verdicts)

			plan, err := buildReusePlan(context.Background(), ReuseConfig{}, inv, 1,
				targetQuery{Object: "NGC7000"})

			require.NoError(t, err)
			lanes := make([]string, 0, len(plan.byFilter))
			for lane := range plan.byFilter {
				lanes = append(lanes, lane)
			}
			assert.ElementsMatch(t, tt.wantLanes, lanes)
			// Whatever the lane is called, every group still carries its TRUE filter for calibration
			// matching — the lane names the stack, it must never rename the light.
			for lane, groups := range plan.byFilter {
				for _, g := range groups {
					assert.Equal(t, filters.Color, g.Key.Filter, "lane %s renamed the light's filter", lane)
				}
			}
		})
	}
}

// TestPartitionGroups_MonoIsUntouched: a filter wheel has no clip filter at all. Every mono scan must
// come out of the partition exactly as it went in — this is the degradation contract the whole card
// rests on, and mono is the overwhelming majority of the captures this engine stacks.
func TestPartitionGroups_MonoIsUntouched(t *testing.T) {
	plan, err := buildReusePlan(context.Background(), ReuseConfig{}, currentInv(), 1,
		targetQuery{Object: "M51"})

	require.NoError(t, err)
	require.Len(t, plan.byFilter, 1)
	groups, ok := plan.byFilter["L"]
	require.True(t, ok, "the L lane keeps its plain name")
	require.Len(t, groups, 1)
	assert.Equal(t, "L", groups[0].Key.Filter)
}

// TestDualbandLast pins the registration order. seqapplyreg lands every master on the REFERENCE's
// pixel grid, and symlinkOrdered numbers the sequence from this slice — so index 0 is the reference
// and it has to be the broadband master.
//
// It is not a matter of taste. The broadband stack is the colour base and, on a mixed capture, the
// wider and deeper one; letting the narrowband pointing define the final canvas would crop the base
// to the emission frame. `-framing=min` then keeps only the field they share.
func TestDualbandLast(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "the dual-band lane moves behind the broadband one",
			in:   []string{filters.Color + dualbandLaneSuffix, filters.Color},
			want: []string{filters.Color, filters.Color + dualbandLaneSuffix},
		},
		{
			name: "already in order — untouched",
			in:   []string{filters.Color, filters.Color + dualbandLaneSuffix},
			want: []string{filters.Color, filters.Color + dualbandLaneSuffix},
		},
		{
			// Every ordinary run: no lane was split, so the canonical order stands exactly as it is.
			name: "a mono channel order is never reordered",
			in:   []string{"L", "R", "G", "B"},
			want: []string{"L", "R", "G", "B"},
		},
		{
			name: "single channel",
			in:   []string{filters.Color},
			want: []string{filters.Color},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dualbandLast(tt.in))
		})
	}
}

// TestOrderedFilters_NonCanonicalIsDeterministic: orderedFilters emitted the filters it does not
// know in Go MAP ORDER, which is randomised per run. Two lanes named RGB and RGB-dualband are both
// unknown to the canonical list, so the reference master — and therefore the final canvas — could
// change between two runs over identical input. Sorting makes a run reproducible.
func TestOrderedFilters_NonCanonicalIsDeterministic(t *testing.T) {
	masters := map[string]string{
		"L": "a", filters.Color + dualbandLaneSuffix: "b", filters.Color: "c", "Zz": "d",
	}
	first := orderedFilters(masters)
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, orderedFilters(masters), "ordering must not depend on map iteration")
	}
	assert.Equal(t, "L", first[0], "canonical filters still lead")
	assert.Equal(t, []string{filters.Color, filters.Color + dualbandLaneSuffix, "Zz"}, first[1:],
		"the rest are sorted, not random")
}
