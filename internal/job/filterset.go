package job

import "github.com/verove-jordan/astronomy/internal/filters"

// parseFilterSetOverrides validates a RunRequest's per-set clip-filter assertions and converts them
// to the typed map the scan takes. It is the job-side twin of the parsing /api/inspect already does,
// so the same wire value means the same thing whether the user is looking at a scan or stacking it.
//
// A nil/empty map stays nil: a run that asserts nothing must reach the scan exactly as before, with
// detection alone deciding.
func parseFilterSetOverrides(raw map[string]string) (map[string]filters.FilterSet, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]filters.FilterSet, len(raw))
	for setID, v := range raw {
		fs, err := filters.ParseFilterSet(v)
		if err != nil {
			return nil, err
		}
		out[setID] = fs
	}
	return out, nil
}

// mustParseFilterSetOverrides is the worker-side call, for a request Enqueue already validated. A
// value that fails here cannot come from the API or the CLI — it would mean a hand-edited row in the
// jobs table — and dropping it is the safe outcome: the scan falls back to detection alone rather
// than the run dying at its very first step.
func mustParseFilterSetOverrides(raw map[string]string) map[string]filters.FilterSet {
	out, err := parseFilterSetOverrides(raw)
	if err != nil {
		return nil
	}
	return out
}
