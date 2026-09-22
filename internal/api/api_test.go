package api

import (
	"bytes"
	"encoding/binary"
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

func TestWithin(t *testing.T) {
	s := &Server{cfg: &config.Config{DataDir: "/tmp/data"}}
	cases := []struct {
		path string
		ok   bool
	}{
		{"/tmp/data", true},
		{"/tmp/data/M31", true},
		{"/tmp/data/../data/M31", true},
		{"/tmp/dataother", false},
		{"/etc/passwd", false},
		{"", false},
	}
	for _, tc := range cases {
		_, ok := s.within(tc.path, "/tmp/data")
		assert.Equal(t, tc.ok, ok, tc.path)
	}
}

func TestServeFile_TraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("hello"), 0o644))
	s := &Server{cfg: &config.Config{OutputDir: dir}}
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/file?path="+filepath.Join(dir, "ok.txt"), nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/file?path=/etc/passwd", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInspect_MultiPathMergeAndConfinement(t *testing.T) {
	data := t.TempDir()
	lights := filepath.Join(data, "lights")
	calib := filepath.Join(data, "calib")
	require.NoError(t, os.MkdirAll(lights, 0o755))
	require.NoError(t, os.MkdirAll(calib, 0o755))
	fitstest.Write(t, lights, "l_1.fits", 8, 8, 900, map[string]string{
		"IMAGETYP": "'Light Frame'", "FILTER": "'L'", "EXPTIME": "120.0", "GAIN": "139", "OBJECT": "'M31'",
	})
	fitstest.Write(t, calib, "dark_1.fits", 8, 8, 600, map[string]string{
		"IMAGETYP": "'Dark Frame'", "EXPTIME": "120.0", "GAIN": "139",
	})

	s := &Server{cfg: &config.Config{DataDir: data}, scanCache: inspect.NewScanCache()}
	h := s.Handler()

	t.Run("merges multiple folders into one inventory", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"paths": []string{lights, calib}})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/inspect", bytes.NewReader(body)))
		require.Equal(t, http.StatusOK, rec.Code)
		var inv inspect.Inventory
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
		assert.Len(t, inv.Frames, 2, "one light + one dark, merged across the two folders")
	})

	t.Run("rejects a path outside the data dir", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"paths": []string{lights, "/etc"}})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/inspect", bytes.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestPreview_FITSBufferAndConfinement(t *testing.T) {
	data := t.TempDir()
	fitstest.Write(t, data, "frame.fits", 8, 8, 1000, map[string]string{"IMAGETYP": "'Light Frame'"})
	s := &Server{cfg: &config.Config{DataDir: data, PreviewMaxEdge: 1500}}
	h := s.Handler()

	t.Run("returns a binary preview buffer whose header matches the body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/preview?path="+filepath.Join(data, "frame.fits"), nil))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
		body := rec.Body.Bytes()
		require.GreaterOrEqual(t, len(body), 16)
		w := binary.LittleEndian.Uint32(body[0:])
		hh := binary.LittleEndian.Uint32(body[4:])
		c := binary.LittleEndian.Uint32(body[8:])
		assert.Equal(t, uint32(8), w)
		assert.Equal(t, uint32(8), hh)
		assert.Equal(t, uint32(1), c)
		assert.Equal(t, 16+int(w*hh*c)*2, len(body))
	})

	t.Run("rejects a path outside the data dir", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/preview?path=/etc/hosts", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("rejects an unsupported file type", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(data, "clip.ser"), []byte("x"), 0o644))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/preview?path="+filepath.Join(data, "clip.ser"), nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestRerunRoute_Registered(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	// A bad id is rejected in the handler before it touches the manager, so this needs no store/DB. A
	// 400 (rather than a 404) proves POST /api/jobs/{id}/rerun is registered and the handler runs.
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/jobs/notanumber/rerun", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCORSPreflight(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/jobs", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}
