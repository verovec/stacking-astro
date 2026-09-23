package job

import (
	"fmt"

	"github.com/verove-jordan/astronomy/internal/filters"
)

// filterSetOverrides parses the request's per-set clip-filter assertions into the typed form the
// scan takes.
//
// A one-shot-colour sensor has no FILTER card, so whether a night was shot through a dual-band clip
// can only be measured from the pixels — and on a capture whose sky straddles the thresholds (a
// moonlit or variable night) the measurement honestly declines. Until now the only way to say so
// was POST /api/inspect, which the Import UI uses; a run launched straight at POST /api/jobs had no
// way to assert it and could only inherit "unknown". That is what let an agent finish an emission
// capture as plain broadband colour with nothing able to contradict it.
//
// Returns nil for an empty map so an untouched request stays byte-identical downstream.
func (r RunRequest) filterSetOverrides() (map[string]filters.FilterSet, error) {
	if len(r.FilterSetOverrides) == 0 {
		return nil, nil
	}
	out := make(map[string]filters.FilterSet, len(r.FilterSetOverrides))
	for setID, raw := range r.FilterSetOverrides {
		fs, err := filters.ParseFilterSet(raw)
		if err != nil {
			// A typo must fail the REQUEST, not leave the set silently unclassified — the user would
			// see their assertion "not take" with no explanation. Same precedence as /api/inspect.
			return nil, fmt.Errorf("filter_set_overrides[%q]: %w", setID, err)
		}
		if !fs.Known() {
			continue // "unknown" asserts nothing; let detection answer
		}
		out[setID] = fs
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// filterSetOverridesOrNil is filterSetOverrides for the assembly path, where Enqueue has already
// rejected anything malformed: a parse error at this point cannot be new, and asserting nothing is
// the same safe default an absent override has.
func filterSetOverridesOrNil(r RunRequest) map[string]filters.FilterSet {
	out, err := r.filterSetOverrides()
	if err != nil {
		return nil
	}
	return out
}
