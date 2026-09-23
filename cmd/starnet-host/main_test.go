package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/starnet"
)

// fakeStarnet writes a stand-in StarNet: it prints two progress lines and copies in→out, so the wire
// contract can be tested on any machine (CI included) without the real, non-redistributable binary.
// exitCode > 0 makes it fail after printing, standing in for a StarNet that rejects its input.
func fakeStarnet(t *testing.T, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stand-in is POSIX-only")
	}
	path := filepath.Join(t.TempDir(), "starnet-fake")
	script := "#!/bin/sh\n" +
		"echo '50%'\n" +
		"echo '100%'\n"
	if exitCode > 0 {
		script += "echo 'tiff has 32 bits per sample'\nexit 1\n"
	} else {
		script += "cp \"$1\" \"$2\"\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// serve mounts the REAL routes over a runner, so these tests exercise the service the engine talks to
// rather than a reimplementation of it.
func serve(t *testing.T, runner *starnet.Runner) string {
	t.Helper()
	srv := httptest.NewServer(routes(runner))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestService_RemoveStarsEndToEnd is the whole point of the bridge: an engine runner with NO local
// binary (the containerized case) drives the host's StarNet over HTTP and gets the starless file
// written to the shared path, with progress streamed as if it had run locally.
func TestService_RemoveStarsEndToEnd(t *testing.T) {
	host := starnet.NewVariant(fakeStarnet(t, 0), starnet.VariantPositional, "")
	url := serve(t, host)

	dir := t.TempDir()
	in, out := filepath.Join(dir, "final.tif"), filepath.Join(dir, "starless.tif")
	require.NoError(t, os.WriteFile(in, []byte("tiff-bytes"), 0o644))

	var lines []string
	engine := starnet.NewVariant("", starnet.VariantAuto, url) // no local binary → offload
	require.NoError(t, engine.RemoveStars(context.Background(), in, out, starnet.Options{}, func(p starnet.Progress) {
		if p.Line != "" {
			lines = append(lines, p.Line)
		}
	}))

	got, err := os.ReadFile(out)
	require.NoError(t, err, "the starless file lands on the shared path, not in the HTTP response")
	assert.Equal(t, "tiff-bytes", string(got))
	assert.Equal(t, []string{"50%", "100%"}, lines, "host progress reaches the job log unchanged")
}

// TestService_RemoveStarsFailureSurfaces: a StarNet that fails on the host must fail the engine call,
// so the pipeline soft-falls to full stars instead of trusting a starless file that was never written.
func TestService_RemoveStarsFailureSurfaces(t *testing.T) {
	url := serve(t, starnet.NewVariant(fakeStarnet(t, 1), starnet.VariantPositional, ""))

	err := starnet.NewVariant("", starnet.VariantAuto, url).
		RemoveStars(context.Background(), "/in.tif", "/out.tif", starnet.Options{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tiff has 32 bits per sample", "the host's own diagnosis travels back")
}

// TestService_HealthReflectsHostBinary: /health is what Available() answers in offload mode, so it must
// track the host binary — not merely that the process is up.
func TestService_HealthReflectsHostBinary(t *testing.T) {
	tests := []struct {
		name string
		bin  string
		want bool
	}{
		{"binary present", fakeStarnet(t, 0), true},
		{"binary missing", "/nonexistent/starnet2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := serve(t, starnet.NewVariant(tt.bin, starnet.VariantPositional, ""))
			err := starnet.NewVariant("", starnet.VariantAuto, url).Available(context.Background())
			if tt.want {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestService_RejectsIncompletePaths: the engine and host share absolute paths, so a request missing
// one is a wiring bug — answer 400 rather than exec'ing StarNet with an empty argument.
func TestService_RejectsIncompletePaths(t *testing.T) {
	url := serve(t, starnet.NewVariant(fakeStarnet(t, 0), starnet.VariantPositional, ""))

	resp, err := http.Post(url+"/run", "application/json", strings.NewReader(`{"in":"/a"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPort_DefaultAndOverride(t *testing.T) {
	t.Setenv("ASTRO_STARNET_PORT", "")
	assert.Equal(t, "8085", port())
	t.Setenv("ASTRO_STARNET_PORT", "9999")
	assert.Equal(t, "9999", port())
}
