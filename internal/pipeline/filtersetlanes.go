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
	"path/filepath"
	"strings"

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

// dualbandLaneOf returns the channel map's dual-band lane key, or "" when the run did not split.
func dualbandLaneOf(channels map[string]string) string {
	for lane := range channels {
		if strings.HasSuffix(lane, dualbandLaneSuffix) {
			return lane
		}
	}
	return ""
}

// dualSetChannels turns the dual-band LANE into the two emission channels it always was, once its
// master has been registered onto the broadband grid.
//
// It splits the REGISTERED master on purpose. Pseudo-Hα and pseudo-[OIII] are composited
// pixel-for-pixel against the broadband base, so splitting before registration would bake the two
// lanes' pointing difference into the emission layers — precisely the misalignment that registering
// the masters exists to remove.
//
// The lane is then dropped from the map whatever happens. It is not a colour channel: leaving it in
// would hand resolvePalette a second RGB it has no meaning for, and the finish would have to guess
// which of the two was the real one. A master that cannot be split costs only its own emission
// layers — the broadband base is a complete image on its own, and the run says so and finishes.
//
// Unlike duobandChannels this is NOT gated on a narrowband palette. The whole point of a mixed
// capture is the runbook's composite: a natural broadband base with the emission lines screened over
// it, which needs Ha/OIII present while the palette stays "natural".
func dualSetChannels(channels map[string]string, outDir string) (map[string]string, string) {
	lane := dualbandLaneOf(channels)
	if lane == "" {
		return channels, ""
	}
	out := make(map[string]string, len(channels)+1)
	for k, v := range channels {
		if k != lane {
			out[k] = v
		}
	}
	ha, oiii, err := splitDuoband(filepath.Join(outDir, channels[lane]+".fits"), outDir)
	if err != nil {
		return out, "dual-band lane split skipped, finishing on the broadband base alone: " + err.Error()
	}
	out["Ha"], out["OIII"] = ha, oiii
	return out, "mixed filter sets: the dual-band master was registered onto the broadband grid, then " +
		"separated into pseudo-Hα (red pixels) and pseudo-[OIII] (max of green/blue) — the broadband " +
		"stack is the colour base and the two emission lines screen over it"
}

// registerSynthesizedChannels records the emission channels a run SEPARATED from a colour master, so
// everything reading the Result — the refine panel, a re-mix, run.json — can see the run has them.
//
// Without it the pseudo-Hα/[OIII] files were written, handed to the finish and then forgotten: a
// mixed capture could be finished once and never re-tuned, because reconstructChannelsFromDisk walks
// res.Channels and there was nothing there to walk.
//
// They are flagged Synthesized and carry no frame count or exposure. A pseudo-Hα separated from a
// colour master is not an Hα master stacked from frames shot through a 3 nm filter, and anything
// reporting integration has to be able to tell them apart.
func registerSynthesizedChannels(res *Result, channels map[string]string) {
	if res == nil {
		return
	}
	have := make(map[string]bool, len(res.Channels))
	object := ""
	for _, ch := range res.Channels {
		have[ch.Filter] = true
		if object == "" {
			object = ch.Object
		}
	}
	for _, f := range []string{"Ha", "OIII", "SII"} {
		if _, split := channels[f]; !split || have[f] {
			continue
		}
		res.Channels = append(res.Channels, ChannelResult{Object: object, Filter: f, Synthesized: true})
	}
}
