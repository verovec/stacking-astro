package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/config"
	"github.com/verove-jordan/astronomy/internal/fits/fitstest"
	"github.com/verove-jordan/astronomy/internal/inspect"
)

// TestInspect_FilterSetOverrideValidation pins the closed enum at the request boundary. A typo must
// fail loudly: silently dropping it would show the user an override that "did not take", with no
// explanation and a run that then stacks the night as something else.
func TestInspect_FilterSetOverrideValidation(t *testing.T) {
	data := t.TempDir()
	lights := filepath.Join(data, "M31")
	require.NoError(t, os.MkdirAll(lights, 0o755))
	fitstest.Write(t, lights, "l_1.fits", 8, 8, 900, map[string]string{
		"IMAGETYP": "'Light Frame'", "EXPTIME": "120.0", "BAYERPAT": "'RGGB'",
	})
	s := &Server{cfg: &config.Config{DataDir: data}, scanCache: inspect.NewScanCache()}
	h := s.Handler()

	post := func(body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/inspect", bytes.NewReader(b)))
		return rec
	}

	t.Run("rejects an unknown filter set", func(t *testing.T) {
		for _, bad := range []string{"narrowband", "duo", "Ha", "true"} {
			rec := post(map[string]any{
				"path":                 lights,
				"filter_set_overrides": map[string]string{"SOME|SET|ID": bad},
			})
			require.Equal(t, http.StatusBadRequest, rec.Code, "value %q: %s", bad, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "filter_set")
		}
	})

	t.Run("accepts the legal values", func(t *testing.T) {
		for _, ok := range []string{"broadband", "dualband", "unknown", ""} {
			rec := post(map[string]any{
				"path":                 lights,
				"filter_set_overrides": map[string]string{"SOME|SET|ID": ok},
			})
			assert.Equal(t, http.StatusOK, rec.Code, "value %q: %s", ok, rec.Body.String())
		}
	})

	t.Run("an absent field leaves the scan untouched", func(t *testing.T) {
		rec := post(map[string]any{"path": lights})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var inv inspect.Inventory
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
		assert.Empty(t, inv.FilterSets, "no bias in this scan — nothing is classifiable")
	})
}
