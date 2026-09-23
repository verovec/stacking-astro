// filtersetlanes.go separates a one-shot-colour capture's BROADBAND exposures from its DUAL-BAND
// ones so they stack as two channel lanes instead of one.
//
// The two are not interchangeable data. A dual-band clip passes ~3 nm around Hα and [OIII] and
// nothing else; an unfiltered train passes the whole continuum. Averaging them discards most of what
// each was shot for: the emission lines are diluted by continuum, and the broadband star colour is
// polluted by two narrow windows. The manual procedure has always been two separate stacks and a
// composite (tasks-context/ngc7000-narrowband-plus-broadband.md).
//
// It used to happen silently, because the clip filter is deliberately NOT part of SetKey — it is a
// measured value, and folding it into the key would churn the exclude_sets tokens the UI stores
// (see internal/inspect/filterset.go). So two OSC sets differing only by the filter in front of the
// sensor shared a channel, and the grouped path co-registered and stacked them into one master.
package pipeline

import (
	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// dualbandLaneSuffix names the lane that stacks the DUAL-BAND exposures, when a scan holds both
// kinds. The broadband lane keeps the plain filter name on purpose: it is the reference grid the
// dual-band master is registered onto, and it is what a single-set run is already called — so a
// capture that does not split comes out byte-identical.
const dualbandLaneSuffix = "-dualband"

// splitsFilterSets reports whether this scan holds BOTH a classified broadband and a classified
// dual-band LIGHT set — the only situation in which separating the lanes is justified.
//
// Both must be present and both must be measured. One classified set alone is not evidence about
// any other: a dual-band-only capture (the ordinary L-eXtreme user) has nothing to be separated
// from, and an unclassified set is silence, not "the other kind". In every one of those cases the
// scan keeps the single lane it has today.
func splitsFilterSets(inv *inspect.Inventory) bool {
	if inv == nil {
		return false
	}
	var broadband, dualband bool
	for _, set := range inv.Sets {
		if set.Key.Type != inspect.Light {
			continue
		}
		switch inv.FilterSets[set.Key.ID()] {
		case filters.FilterSetBroadband:
			broadband = true
		case filters.FilterSetDualband:
			dualband = true
		}
	}
	return broadband && dualband
}

// laneFor returns the channel lane a light set stacks in. It renames the STACK, never the light: the
// group keeps its true SetKey, so calibration still matches on the real filter (and on the clip
// filter, via card 0006's gate).
func laneFor(filter string, fs filters.FilterSet, split bool) string {
	if split && fs == filters.FilterSetDualband {
		return filter + dualbandLaneSuffix
	}
	return filter
}
