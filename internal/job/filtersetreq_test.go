package job

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
)

// The filter-set classifier reads the pixels, and on a real capture it can decline (the two signals
// disagree) or be wrong (its sky thresholds are ADU, so a night shot at a different analogue gain
// lands in the wrong band). /api/inspect already accepts a per-set override for exactly that; a RUN
// did not, so the classification could be corrected for LOOKING at a scan but never for STACKING it
// — and without both sets classified, splitsFilterSets declines and the broadband and dual-band
// lights merge into one master, which is the outcome the lane split exists to prevent.
func TestRunRequest_FilterSetOverrides(t *testing.T) {
	const setID = "LIGHT|M 31|RGB|e300000|g100o50b1|i0|t-17|s:2026-09-22"

	t.Run("wire tag decodes per-set overrides", func(t *testing.T) {
		var r RunRequest
		require.NoError(t, json.Unmarshal([]byte(
			`{"path":"input/M31","mode":"deepsky","filter_set_overrides":{"`+setID+`":"dualband"}}`), &r))
		assert.Equal(t, map[string]string{setID: "dualband"}, r.FilterSetOverrides)
	})

	t.Run("omitted leaves the map nil so today's behaviour is unchanged", func(t *testing.T) {
		var r RunRequest
		require.NoError(t, json.Unmarshal([]byte(`{"path":"input/M31","mode":"deepsky"}`), &r))
		assert.Nil(t, r.FilterSetOverrides)
	})
}

// parseFilterSetOverrides is what Enqueue validates with. A typo must fail the REQUEST — exactly as
// the inspect endpoint rejects it — rather than being dropped silently, which would leave the user
// watching their override "not take" with no explanation.
func TestParseFilterSetOverrides(t *testing.T) {
	const a, b = "SET|A", "SET|B"

	tests := []struct {
		name    string
		in      map[string]string
		want    map[string]filters.FilterSet
		wantErr string
	}{
		{name: "nil stays nil", in: nil, want: nil},
		{name: "empty stays nil", in: map[string]string{}, want: nil},
		{
			name: "both kinds parse",
			in:   map[string]string{a: "dualband", b: "broadband"},
			want: map[string]filters.FilterSet{a: filters.FilterSetDualband, b: filters.FilterSetBroadband},
		},
		{
			name: "case and padding are tolerated, as on the inspect endpoint",
			in:   map[string]string{a: "  DualBand "},
			want: map[string]filters.FilterSet{a: filters.FilterSetDualband},
		},
		{
			name: "an explicit unknown is a legitimate way to clear an override",
			in:   map[string]string{a: "unknown"},
			want: map[string]filters.FilterSet{a: filters.FilterSetUnknown},
		},
		{name: "a typo fails the request", in: map[string]string{a: "dual-band"}, wantErr: "unknown filter_set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFilterSetOverrides(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
