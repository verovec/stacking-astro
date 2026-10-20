package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFitsDataSHA256_IgnoresHeaderChanges(t *testing.T) {
	dir := t.TempDir()
	grid := make([]float64, synthW*synthH)
	for i := range grid {
		grid[i] = float64(i % 4000)
	}
	require.NoError(t, writeFrame(dir, "a", grid, map[string]string{"DATE": "'2026-01-01T00:00:00'"}))
	require.NoError(t, writeFrame(dir, "b", grid, map[string]string{"DATE": "'2026-10-20T12:00:00'", "PROGRAM": "'Siril'"}))
	grid[0]++
	require.NoError(t, writeFrame(dir, "c", grid, nil))

	a, err := fitsDataSHA256(filepath.Join(dir, "a.fits"))
	require.NoError(t, err)
	b, err := fitsDataSHA256(filepath.Join(dir, "b.fits"))
	require.NoError(t, err)
	c, err := fitsDataSHA256(filepath.Join(dir, "c.fits"))
	require.NoError(t, err)
	assert.Equal(t, a, b, "header cards must not change the data hash")
	assert.NotEqual(t, a, c, "a pixel change must change the data hash")
}

func TestFitsDataOffset_Rejects(t *testing.T) {
	for name, body := range map[string][]byte{
		"empty":     nil,
		"no END":    bytes.Repeat([]byte(" "), fitsBlock),
		"truncated": append(bytes.Repeat([]byte(" "), 80), []byte("END"+string(bytes.Repeat([]byte(" "), 77)))...),
	} {
		_, err := fitsDataOffset(body)
		assert.Error(t, err, name)
	}
}

func TestSyntheticCaptures_AreByteIdenticalWhenRegenerated(t *testing.T) {
	for name, build := range map[string]func(string) error{
		"single-night": buildGoldenCapture,
		"two-night":    func(root string) error { return buildTwoNightCapture(root, 35) },
	} {
		a, b := t.TempDir(), t.TempDir()
		require.NoError(t, build(a), name)
		require.NoError(t, build(b), name)
		err := filepath.Walk(a, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(a, path)
			left, _ := os.ReadFile(path)
			right, rerr := os.ReadFile(filepath.Join(b, rel))
			require.NoError(t, rerr, rel)
			assert.True(t, bytes.Equal(left, right), "%s %s differs", name, rel)
			return nil
		})
		require.NoError(t, err)
	}
}

func TestSyntheticCaptures_FrameCounts(t *testing.T) {
	cases := []struct {
		name  string
		build func(string) error
		want  int
	}{
		{"single-night", buildGoldenCapture, 12 + 9},
		{"two-night", func(root string) error { return buildTwoNightCapture(root, 35) }, 12 + 9 + 4 + 6},
	}
	for _, c := range cases {
		root := t.TempDir()
		require.NoError(t, c.build(root))
		found, err := filepath.Glob(filepath.Join(root, "*", "*.fits"))
		require.NoError(t, err)
		assert.Len(t, found, c.want, c.name)
	}
}

func TestNormalizer_ReplacesInOrderAndDropsVolatileKeys(t *testing.T) {
	n := newNormalizer("20261020_120000", "/fx/oracle/m31-db", "/home/captures", true)
	assert.Equal(t, "<CAPTURES>/M31-DB/lights/a.fit", n.str("/home/captures/M31-DB/lights/a.fit"))
	assert.Equal(t, "<ROOT>/work/run_<RUN>/x", n.str("/fx/oracle/m31-db/work/run_20261020_120000/x"))

	got := n.value(map[string]any{
		"timings": []any{1}, "engine": "dev", "path": "/fx/oracle/m31-db/out",
		"nested": []any{map[string]any{"created_at": 1.0, "k": "20261020_120000"}},
	})
	assert.Equal(t, map[string]any{
		"path":   "<ROOT>/out",
		"nested": []any{map[string]any{"k": "<RUN>"}},
	}, got)
}

func TestCaptureScript_NumbersInEmissionOrderAcrossDirs(t *testing.T) {
	shim, work := t.TempDir(), t.TempDir()
	script := filepath.Join(work, "_astrostack.ssf")
	for _, step := range []struct{ dir, body string }{
		{"masters", "first\n"}, {"light_L", "second\n"}, {"masters", "third\n"},
	} {
		require.NoError(t, os.WriteFile(script, []byte(step.body), 0o644))
		require.NoError(t, captureScript(shim, filepath.Join(work, step.dir), script))
	}
	names, err := shimScriptNames(shim)
	require.NoError(t, err)
	assert.Equal(t, []string{"0001_masters.ssf", "0002_light_L.ssf", "0003_masters.ssf"}, names)
	third, _ := os.ReadFile(filepath.Join(shim, shimScripts, names[2]))
	assert.Equal(t, "third\n", string(third))
}
