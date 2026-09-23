package calib

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// oscLightSet is a one-shot-colour light set on a night.
func oscLightSet(night string, exposureMs int64) inspect.Set {
	return inspect.Set{Key: inspect.SetKey{
		Type: inspect.Light, Object: "NGC7000", Filter: filters.Color,
		ExposureMs: exposureMs, Gain: 100, Offset: 50, Bin: 1, Session: night, Color: true,
	}, Count: 30}
}

// oscCalSet is a calibration set on a night, the way inspect groups them (a flat carries a filter
// and an exposure, a bias neither).
func oscCalSet(night string, ft inspect.FrameType, exposureMs int64) inspect.Set {
	key := inspect.SetKey{Type: ft, Gain: 100, Offset: 50, Bin: 1, Session: night, Color: true}
	switch ft {
	case inspect.Flat:
		key.Filter, key.ExposureMs = filters.Color, exposureMs
	case inspect.Dark:
		key.ExposureMs = exposureMs
	}
	return inspect.Set{Key: key, Count: 20}
}

// oscScan assembles an OSC inventory and stamps the per-light verdicts the way annotateFilterSets
// does — the durable map, keyed by SetKey.ID().
func oscScan(verdicts map[int]filters.FilterSet, sets ...inspect.Set) *inspect.Inventory {
	inv := &inspect.Inventory{Sets: sets, ColorModel: inspect.ColorOSC}
	m := map[string]filters.FilterSet{}
	for i, v := range verdicts {
		if v.Known() {
			m[sets[i].Key.ID()] = v
		}
	}
	if len(m) > 0 {
		inv.FilterSets = m
	}
	return inv
}

// TestStampFlatFilterSet is the bridge the gate stands on: keepSameFilterSet compares the LIGHT's
// filter set against the FLAT MASTER's, and nothing else ever puts one on a master. A flat is an
// evenly-lit panel with no sky to classify, so it inherits its capture night (inspect.NightFilterSet).
func TestStampFlatFilterSet(t *testing.T) {
	const nightA, nightB = "2026-07-29", "2026-08-02"

	tests := []struct {
		name string
		inv  *inspect.Inventory
		set  inspect.Set
		typ  MasterType
		want filters.FilterSet
	}{
		{
			name: "a flat inherits its night's verdict",
			inv: oscScan(map[int]filters.FilterSet{0: filters.FilterSetDualband},
				oscLightSet(nightA, 120_000), oscCalSet(nightA, inspect.Flat, 2000)),
			set:  oscCalSet(nightA, inspect.Flat, 2000),
			typ:  MasterFlat,
			want: filters.FilterSetDualband,
		},
		{
			// Closed-shutter exposures: no light reaches the sensor, so the clip filter in the train
			// cannot have touched them. Stamping one would strand a perfectly good dark.
			name: "a dark never carries a filter set",
			inv: oscScan(map[int]filters.FilterSet{0: filters.FilterSetDualband},
				oscLightSet(nightA, 120_000), oscCalSet(nightA, inspect.Dark, 120_000)),
			set:  oscCalSet(nightA, inspect.Dark, 120_000),
			typ:  MasterDark,
			want: "",
		},
		{
			name: "a bias never carries a filter set",
			inv: oscScan(map[int]filters.FilterSet{0: filters.FilterSetBroadband},
				oscLightSet(nightA, 120_000), oscCalSet(nightA, inspect.Bias, 0)),
			set:  oscCalSet(nightA, inspect.Bias, 0),
			typ:  MasterBias,
			want: "",
		},
		{
			// The stamp must stay ABSENT, not become the string "unknown": Master.FilterSet is
			// omitempty, and writing "unknown" onto every mono flat would churn run.json for every
			// capture that has nothing to do with clip filters.
			name: "an unmeasured night leaves the field empty, not \"unknown\"",
			inv: oscScan(nil,
				oscLightSet(nightA, 120_000), oscCalSet(nightA, inspect.Flat, 2000)),
			set:  oscCalSet(nightA, inspect.Flat, 2000),
			typ:  MasterFlat,
			want: "",
		},
		{
			name: "a flat from a night with no lights stays empty",
			inv: oscScan(map[int]filters.FilterSet{0: filters.FilterSetDualband},
				oscLightSet(nightA, 120_000), oscCalSet(nightB, inspect.Flat, 2000)),
			set:  oscCalSet(nightB, inspect.Flat, 2000),
			typ:  MasterFlat,
			want: "",
		},
		{
			name: "no inventory at all",
			inv:  nil,
			set:  oscCalSet(nightA, inspect.Flat, 2000),
			typ:  MasterFlat,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Master{Type: tt.typ}
			stampFlatFilterSet(&m, tt.inv, tt.set)
			assert.Equal(t, tt.want, m.FilterSet)
		})
	}
}

// TestPreviewCandidates_FlatCarriesNightFilterSet: the Import panel must predict what the run will
// do. The synthetic flat it proposes is the master the run would BUILD, so it has to carry the same
// verdict the built one will — otherwise the preview promises a flat the run then refuses.
func TestPreviewCandidates_FlatCarriesNightFilterSet(t *testing.T) {
	const night = "2026-07-29"
	inv := oscScan(map[int]filters.FilterSet{0: filters.FilterSetDualband},
		oscLightSet(night, 120_000),
		oscCalSet(night, inspect.Flat, 2000),
		oscCalSet(night, inspect.Bias, 0),
	)

	got := PreviewCandidates(inv, nil)

	require.Len(t, got, 2)
	byType := map[MasterType]Master{}
	for _, m := range got {
		byType[m.Type] = m
	}
	assert.Equal(t, filters.FilterSetDualband, byType[MasterFlat].FilterSet)
	assert.Empty(t, string(byType[MasterBias].FilterSet), "a bias is closed-shutter")
}

// TestSuggestForInventory_CrossSetFlatIsRefused is the card's whole point, end to end through the
// Import preview: a dual-band night whose only flat came from a broadband night gets NO flat and is
// told why, instead of silently dividing every frame by the wrong illumination profile.
func TestSuggestForInventory_CrossSetFlatIsRefused(t *testing.T) {
	const dualNight, broadNight = "2026-07-29", "2026-08-02"
	inv := oscScan(map[int]filters.FilterSet{0: filters.FilterSetDualband},
		oscLightSet(dualNight, 120_000),
		oscCalSet(broadNight, inspect.Flat, 2000),
	)
	// The broadband night's own lights are what give its flat a verdict.
	broadLights := oscLightSet(broadNight, 120_000)
	inv.Sets = append(inv.Sets, broadLights)
	inv.FilterSets[broadLights.Key.ID()] = filters.FilterSetBroadband

	pv := SuggestForInventory(inv, PreviewCandidates(inv, nil), false)

	require.NotEmpty(t, pv.Channels)
	for _, ch := range pv.Channels {
		if ch.Session != dualNight {
			continue
		}
		for _, s := range ch.Suggestions {
			assert.NotEqual(t, RoleFlat, s.Role, "the broadband flat must not be offered to a dual-band night")
		}
		assert.Contains(t, joinNotes(ch.Notes), "excluded", "the refusal must be named, not silent")
		return
	}
	t.Fatalf("no channel for night %s", dualNight)
}

func joinNotes(notes []string) string {
	out := ""
	for _, n := range notes {
		out += n + " | "
	}
	return out
}
