package job

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/filters"
)

func TestRunRequest_FilterSetOverrides(t *testing.T) {
	const setID = "LIGHT|IC 1848|RGB|e300000|g100o50b1|i0|t-17|s:"

	tests := []struct {
		name    string
		in      map[string]string
		want    map[string]filters.FilterSet
		wantErr string
	}{
		{
			name: "a dual-band assertion reaches the scan",
			in:   map[string]string{setID: "dualband"},
			want: map[string]filters.FilterSet{setID: filters.FilterSetDualband},
		},
		{
			name: "a broadband assertion reaches the scan",
			in:   map[string]string{setID: "broadband"},
			want: map[string]filters.FilterSet{setID: filters.FilterSetBroadband},
		},
		{
			name: "case and padding are accepted, as on the inspect endpoint",
			in:   map[string]string{setID: "  DualBand "},
			want: map[string]filters.FilterSet{setID: filters.FilterSetDualband},
		},
		{
			name: "an explicit unknown asserts nothing and lets detection answer",
			in:   map[string]string{setID: "unknown"},
			want: nil,
		},
		{
			name: "no overrides leaves the request byte-identical downstream",
			in:   nil,
			want: nil,
		},
		{
			name:    "a typo fails the request rather than silently not taking",
			in:      map[string]string{setID: "dualbnad"},
			wantErr: "dualbnad",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RunRequest{FilterSetOverrides: tt.in}.filterSetOverrides()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got, "a rejected request contributes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
