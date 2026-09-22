package filters

import (
	"fmt"
	"strings"
)

// FilterSet is which clip filter an ONE-SHOT-COLOR session was shot through. It is filter vocabulary,
// so it lives here with the rest of it — and is mirrored in frontend/src/constants/filters.ts, pinned
// by filters.spec.ts, under the same rule as Canonical.
//
// It exists because a colour camera has no filter wheel and therefore no FILTER card: the same body
// shoots broadband one night and through a dual-band Ha/OIII clip the next, and nothing in the
// headers or the filenames says which. The two are NOT interchangeable — a dual-band frame carries
// emission lines on a near-black sky, a broadband frame carries a full continuum — so stacking or
// flat-matching them together is wrong. The only witness is the pixels.
//
// This is deliberately NOT a member of Canonical: those are wheel slots, with a display rank and an
// IsNarrowband answer. A filter set is a property of a whole colour session.
type FilterSet string

const (
	// FilterSetUnknown is the honest default: not measured, or the signals disagreed. Every consumer
	// must treat it as "no information" and fall back to today's behaviour — never as broadband.
	FilterSetUnknown FilterSet = "unknown"
	// FilterSetBroadband is an unfiltered (or UV/IR-cut) colour session: full continuum, bright sky.
	FilterSetBroadband FilterSet = "broadband"
	// FilterSetDualband is a dual-narrowband clip (Ha + OIII): two narrow windows, near-black sky.
	FilterSetDualband FilterSet = "dualband"
)

// filterSetOrder is the display order for the UI badge/override control.
var filterSetOrder = []FilterSet{FilterSetUnknown, FilterSetBroadband, FilterSetDualband}

// FilterSets returns every filter set in display order (a copy — callers may sort or append).
func FilterSets() []FilterSet {
	return append([]FilterSet(nil), filterSetOrder...)
}

// Valid reports whether f is a known filter set.
func (f FilterSet) Valid() bool {
	for _, known := range filterSetOrder {
		if f == known {
			return true
		}
	}
	return false
}

// Known reports whether f carries actual information — i.e. is neither empty nor unknown. Consumers
// branch on this rather than comparing against broadband, so an unmeasured set can never be mistaken
// for a measured one.
func (f FilterSet) Known() bool {
	return f == FilterSetBroadband || f == FilterSetDualband
}

// ParseFilterSet validates a wire value. The empty string is unknown, so a client that never sends
// the field — and every inventory recorded before filter sets existed — keeps today's behaviour.
func ParseFilterSet(s string) (FilterSet, error) {
	switch FilterSet(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return FilterSetUnknown, nil
	case FilterSetUnknown:
		return FilterSetUnknown, nil
	case FilterSetBroadband:
		return FilterSetBroadband, nil
	case FilterSetDualband:
		return FilterSetDualband, nil
	default:
		return "", fmt.Errorf("unknown filter_set %q (want: unknown, broadband, dualband)", s)
	}
}
