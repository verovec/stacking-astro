package starnet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunner_Offload is the transport decision table. A usable LOCAL binary always wins: the offload
// exists because some platforms have no runnable StarNet build at all, not to reach a faster device.
func TestRunner_Offload(t *testing.T) {
	tests := []struct {
		name string
		bin  string
		url  string
		want bool
	}{
		{"local binary and no url", "/bin/echo", "", false},
		{"local binary wins over url", "/bin/echo", "http://host:8085", false},
		{"no binary, url set", "", "http://host:8085", true},
		{"unresolvable binary, url set", "/nonexistent/starnet2", "http://host:8085", true},
		{"nothing configured", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NewVariant(tt.bin, VariantAuto, tt.url).offload())
		})
	}
}

// TestAvailable_OffloadPingsService: with no local binary, availability is the host service answering
// /health — so a reachable service makes StarNet available to a container that cannot exec it.
func TestAvailable_OffloadPingsService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	require.NoError(t, NewVariant("", VariantAuto, srv.URL).Available(context.Background()))
}

// TestAvailable_OffloadServiceUnhealthy: a service that answers non-200 is unavailable, and the error
// names the fix rather than a missing binary the container could never have.
func TestAvailable_OffloadServiceUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "starnet binary path is empty", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := NewVariant("", VariantAuto, srv.URL).Available(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starnet binary path is empty")
}

// TestAvailable_OffloadServiceDown: an unreachable service names the recipe that starts it, because
// "StarNet missing" in a container is almost always "the host service isn't running".
func TestAvailable_OffloadServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listening now

	err := NewVariant("", VariantAuto, url).Available(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "just run-starnet-service")
}

// TestRemoveStars_RemoteStreamsProgressThenResult: in offload mode the runner POSTs the paths, forwards
// the host's StarNet output through onProgress, and treats the trailing "ok" sentinel as success.
func TestRemoveStars_RemoteStreamsProgressThenResult(t *testing.T) {
	var got RemoteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/run", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		f := w.(http.Flusher)
		fmt.Fprintln(w, "100%")
		f.Flush()
		fmt.Fprintln(w, ResultPrefix+"ok")
		f.Flush()
	}))
	defer srv.Close()

	var lines []string
	var percents []int
	err := NewVariant("", VariantAuto, srv.URL).RemoveStars(context.Background(),
		"/shared/in.tif", "/shared/out.tif", Options{Stride: 128}, func(p Progress) {
			if p.Line != "" {
				lines = append(lines, p.Line)
				percents = append(percents, p.Percent)
			}
		})
	require.NoError(t, err)
	assert.Equal(t, []string{"100%"}, lines, "host StarNet lines forwarded verbatim")
	assert.Equal(t, []int{100}, percents, "percentages parsed as they are in local mode")
	assert.Equal(t, RemoteRequest{In: "/shared/in.tif", Out: "/shared/out.tif", Stride: 128}, got,
		"only paths travel — the engine and host share them through the bind mounts")
}

// TestRemoveStars_RemoteErrorSentinel: the host service's "error:<msg>" sentinel surfaces as a failed
// call, so the pipeline soft-falls to full stars instead of believing in a starless file that is absent.
func TestRemoveStars_RemoteErrorSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, ResultPrefix+"error:tiff has 32 bits per sample")
	}))
	defer srv.Close()

	err := NewVariant("", VariantAuto, srv.URL).RemoveStars(context.Background(), "/a", "/b", Options{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tiff has 32 bits per sample")
}

// TestRemoveStars_RemoteMultilineError: a StarNet failure carries its WHOLE captured log, and the stream
// is line-delimited — so the sentinel must stay one line. Unescaped, the tail of the diagnosis spills
// past it and comes back as progress lines, throwing away the only text that explains why the finish
// kept full stars.
func TestRemoveStars_RemoteMultilineError(t *testing.T) {
	wire := ResultLine(fmt.Errorf("starnet failed (exit 1): exit status 1\nError: tiff has 32 bits per sample\n"))
	require.NotContains(t, wire, "\n", "the encoded result is a single line")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, wire)
	}))
	defer srv.Close()

	var lines []string
	err := NewVariant("", VariantAuto, srv.URL).RemoveStars(context.Background(), "/a", "/b", Options{},
		func(p Progress) {
			if p.Line != "" {
				lines = append(lines, p.Line)
			}
		})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tiff has 32 bits per sample", "the full diagnosis survives the wire")
	assert.Empty(t, lines, "no part of the failure leaks into the progress stream")
}

// TestRemoveStars_RemoteTruncatedStream: a connection closing before the sentinel is an error (the host
// died mid-run), never a silent success.
func TestRemoveStars_RemoteTruncatedStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "50%") // no result sentinel → truncated
	}))
	defer srv.Close()

	err := NewVariant("", VariantAuto, srv.URL).RemoveStars(context.Background(), "/a", "/b", Options{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "before the run completed")
}

// TestRemoveStars_RemoteHTTPError: a non-200 from the service carries its body into the error.
func TestRemoveStars_RemoteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request: unexpected end of JSON input", http.StatusBadRequest)
	}))
	defer srv.Close()

	err := NewVariant("", VariantAuto, srv.URL).RemoveStars(context.Background(), "/a", "/b", Options{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected end of JSON input")
}
