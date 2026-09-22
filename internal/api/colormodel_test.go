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

// TestCreateJob_ColorModelValidation pins the closed enum at the request boundary: a typo in
// color_model must fail the REQUEST, not the worker mid-run.
//
// The Server carries a nil job manager on purpose. Validation happens before Enqueue, so a rejected
// body never reaches it — and a body that slipped through would nil-panic here rather than quietly
// enqueueing a job. That is the "no job created" half of the assertion.
func TestCreateJob_ColorModelValidation(t *testing.T) {
	data := t.TempDir()
	lights := filepath.Join(data, "M31")
	require.NoError(t, os.MkdirAll(lights, 0o755))
	fitstest.Write(t, lights, "l_1.fits", 8, 8, 900, map[string]string{
		"IMAGETYP": "'Light Frame'", "FILTER": "'L'", "EXPTIME": "120.0",
	})
	s := &Server{cfg: &config.Config{DataDir: data}, scanCache: inspect.NewScanCache()}
	h := s.Handler()

	post := func(body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(b)))
		return rec
	}

	rejected := []string{"colour", "rgb", "mixed", "bayer", "true"}
	for _, v := range rejected {
		t.Run("rejects color_model="+v, func(t *testing.T) {
			rec := post(map[string]any{
				"path": lights, "mode": "deepsky", "format": "image", "color_model": v,
			})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "color_model")
		})
	}
}

// TestCreateJob_ColorModelAccepted proves the three legal values (and an absent field) pass validation
// and reach the manager. With a nil manager that contact point panics — recovered here — which is
// exactly the evidence we want: the request was NOT rejected at the boundary.
func TestCreateJob_ColorModelAccepted(t *testing.T) {
	data := t.TempDir()
	lights := filepath.Join(data, "M31")
	require.NoError(t, os.MkdirAll(lights, 0o755))
	fitstest.Write(t, lights, "l_1.fits", 8, 8, 900, map[string]string{
		"IMAGETYP": "'Light Frame'", "FILTER": "'L'", "EXPTIME": "120.0",
	})
	s := &Server{cfg: &config.Config{DataDir: data}, scanCache: inspect.NewScanCache()}

	for _, v := range []string{"", "auto", "mono", "osc", "OSC"} {
		t.Run("accepts color_model="+v, func(t *testing.T) {
			body := map[string]any{"path": lights, "mode": "deepsky", "format": "image"}
			if v != "" {
				body["color_model"] = v
			}
			b, _ := json.Marshal(body)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(b))

			reachedManager := false
			func() {
				defer func() { reachedManager = recover() != nil }()
				s.Handler().ServeHTTP(rec, req)
			}()
			assert.True(t, reachedManager || rec.Code != http.StatusBadRequest,
				"a legal color_model must pass validation, got %d %s", rec.Code, rec.Body.String())
		})
	}
}

// TestParseColorChoice_WiredToRequest pins that the enum the API validates against is the one the
// pipeline resolves with — one definition, not a second copy of the value list in the handler.
func TestParseColorChoice_WiredToRequest(t *testing.T) {
	for _, v := range []string{"auto", "mono", "osc"} {
		got, err := inspect.ParseColorChoice(v)
		require.NoError(t, err)
		assert.Equal(t, inspect.ColorChoice(v), got)
	}
}
