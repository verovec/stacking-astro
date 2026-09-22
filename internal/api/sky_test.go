package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/config"
)

func skyTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "messier.csv"),
		[]byte("name,ra,dec,diameter,mag,alias\nM81,148.888,69.065,26.9,6.9,Bode's Galaxy/NGC3031\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ngc.csv"),
		[]byte("name,ra,dec,diameter,mag,alias\nNGC104,6.0238,-72.081,50,4.0,47 Tucanae\n"), 0o644))
	return &Server{cfg: &config.Config{SirilCatalogDir: dir}}
}

// GET /api/sky/search is the Mosaic planner's target search — the one sky endpoint that survives the
// planner removal (E01 card 0015).
func TestSkySearch(t *testing.T) {
	h := skyTestServer(t).Handler()

	search := func(t *testing.T, query string) []skySearchResult {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sky/search?q="+query, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp struct {
			Results []skySearchResult `json:"results"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		return resp.Results
	}

	tests := []struct {
		name, query, wantFirst string
	}{
		{"catalogue id", "M81", "M81"},
		{"case insensitive", "m81", "M81"},
		{"common name", "Bode", "M81"},
		{"other catalogue", "NGC104", "NGC104"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := search(t, tt.query)
			require.NotEmpty(t, results)
			assert.Equal(t, tt.wantFirst, results[0].Name)
			assert.NotZero(t, results[0].RADeg)
		})
	}

	t.Run("missing query is a 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sky/search", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
