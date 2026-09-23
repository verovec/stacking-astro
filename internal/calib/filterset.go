// filterset.go holds the clip-filter half of matching: which optical train a light and a flat were
// shot through, and the rule that they must be the same one. Nothing here touches darks or bias —
// those are closed-shutter exposures, and no light reaches the sensor through the filter at all.
package calib

import (
	"github.com/verove-jordan/astronomy/internal/filters"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// LightRef identifies the light set being calibrated: its key, plus the clip filter its night was
// shot through. The filter set is carried BESIDE the key rather than inside it because SetKey.ID() is
// the token the UI sends back in exclude_sets — folding a measured value into it would churn stored
// tokens the moment a detection threshold moved (see internal/inspect/filterset.go).
//
// The zero value is a light with no known filter set, which is every monochrome light and every
// colour night the pixels could not settle — and which reproduces the pre-filter-set matching
// exactly.
type LightRef struct {
	Key       inspect.SetKey
	FilterSet filters.FilterSet
}

// RefFor builds the LightRef for a light set of this scan: the set's own measured verdict, falling
// back to its night's. It is the single place the run and the preview agree on what a light's clip
// filter is, so the two can never disagree about which flats are eligible.
//
// The night fallback matters for the paths that rebuild Sets from Frames: buildSets returns fresh
// structs with no projected FilterSet, and a light group assembled that way would otherwise read
// unknown and quietly re-open the gate it is supposed to close.
func RefFor(inv *inspect.Inventory, set inspect.Set) LightRef {
	if set.FilterSet.Known() {
		return LightRef{Key: set.Key, FilterSet: set.FilterSet}
	}
	if night := inv.NightFilterSet(set.Key.Session); night.Known() {
		return LightRef{Key: set.Key, FilterSet: night}
	}
	// Left EMPTY rather than the "unknown" literal: both read !Known(), and carrying one spelling
	// keeps every consumer — and every test — from having to know which one it will get.
	return LightRef{Key: set.Key}
}

// stampFlatFilterSet gives a FLAT master the clip filter of the night it was shot on. It is the only
// thing that ever puts a filter set on a master, and therefore the only thing that makes
// keepSameFilterSet able to fire.
//
// A flat is an evenly-illuminated panel, not a sky, so the pixel classifier has nothing to measure on
// it (internal/inspect/filterset.go classifies LIGHT sets only) — it inherits the night instead, via
// inspect.NightFilterSet. Darks, dark-flats and bias are skipped: closed shutter, no light path.
//
// An unknown verdict leaves the field EMPTY rather than writing "unknown". Master.FilterSet is
// omitempty, and stamping the literal would add a key to every flat in every run.json — including the
// monochrome captures that have nothing to do with clip filters.
func stampFlatFilterSet(m *Master, inv *inspect.Inventory, set inspect.Set) {
	if m == nil || m.Type != MasterFlat {
		return
	}
	if fs := inv.NightFilterSet(set.Key.Session); fs.Known() {
		m.FilterSet = fs
	}
}

// keepSameFilterSet removes the flats shot through a DIFFERENT clip filter than the light, returning
// the surviving pool and how many were dropped.
//
// It pre-filters the candidate pool rather than ranking inside pickFlat, for the reason dims.go
// documents at length: striking a master after the match has already chosen it loses the fallback,
// while filtering first lets the remaining passes find the next-best candidate. It also keeps
// pickFlat and flatBeats exactly as they were, so every existing ranking guarantee still holds.
//
// Both sides must be KNOWN for the gate to fire — either one unknown leaves the pool untouched and
// reproduces the pre-filter-set behaviour byte for byte. force_calibration_frames drops it like
// every other gate: the user asked for their masters to be applied.
func keepSameFilterSet(ref LightRef, masters []Master, force bool) ([]Master, int) {
	if force || !ref.FilterSet.Known() {
		return masters, 0
	}
	kept := make([]Master, 0, len(masters))
	dropped := 0
	for _, m := range masters {
		if m.Type == MasterFlat && m.FilterSet.Known() && m.FilterSet != ref.FilterSet {
			dropped++
			continue
		}
		kept = append(kept, m)
	}
	if dropped == 0 {
		return masters, 0
	}
	return kept, dropped
}

// flatExclusionCost reports whether refusing the cross-set flats actually cost this light set
// something: it ended up with no flat, or with one borrowed from a different capture night. When its
// own night supplied a same-set flat, the exclusion changed nothing and there is nothing to report.
func flatExclusionCost(light inspect.SetKey, flat *Master) bool {
	return flat == nil || flat.Session != light.Session
}

// otherSet names the opposite filter set, for the exclusion note.
func otherSet(f filters.FilterSet) filters.FilterSet {
	if f == filters.FilterSetDualband {
		return filters.FilterSetBroadband
	}
	return filters.FilterSetDualband
}
