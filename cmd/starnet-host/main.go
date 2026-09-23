// Command starnet-host is a tiny native HTTP service that runs the host-installed StarNet on demand, so
// the containerized engine can still remove stars. Unlike GraXpert — whose offload is about reaching the
// host GPU — this one exists because the container has no runnable StarNet AT ALL: upstream publishes
// Linux x64, Windows x64 and both macOS builds, but no linux/arm64 one, and a macOS binary bind-mounted
// into the Linux image is a Mach-O that will never execute there. Without this service an Apple-Silicon
// `just stack` silently keeps full stars.
//
// It shares the engine's absolute file paths (the compose bind mounts), so requests carry only paths,
// never TIFF bytes. Mirrors cmd/graxpert-host and the finish-supervisor's host model (`just
// run-ia-model`): a host tool, invoked, never vendored. Run it with `just run-starnet-service`.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/verove-jordan/astronomy/internal/config"
	"github.com/verove-jordan/astronomy/internal/starnet"
)

func main() {
	cfg := config.Load()
	// A LOCAL runner (empty URL) — this service is the offload target, so it must exec StarNet directly,
	// never forward to itself.
	runner := starnet.NewVariant(cfg.StarnetBin, starnet.Variant(cfg.StarnetCLI), "")
	addr := "127.0.0.1:" + port()

	// No WriteTimeout: a star removal streams for many minutes on a large frame; the client's request
	// context bounds it.
	srv := &http.Server{Addr: addr, Handler: routes(runner), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("starnet-host: serving on http://%s (STARNET_BIN=%q). Ctrl-C to stop.", addr, cfg.StarnetBin)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("starnet-host: %v", err)
	}
}

// routes is the service's whole surface: a health probe the engine's Available() calls, and one
// streaming run endpoint. Separate from main so the tests drive the real handlers.
func routes(runner *starnet.Runner) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := runner.Available(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) { handleRun(runner, w, r) })
	return mux
}

func port() string {
	if p := os.Getenv("ASTRO_STARNET_PORT"); p != "" {
		return p
	}
	return "8085"
}

// handleRun runs one star removal, streaming StarNet's output back line-by-line, then a terminal
// ResultPrefix line ("ok" or "error:<msg>"). The progress lines flow through unchanged, so the engine's
// job log looks identical whether StarNet ran locally or here.
func handleRun(runner *starnet.Runner, w http.ResponseWriter, r *http.Request) {
	var req starnet.RemoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.In == "" || req.Out == "" {
		http.Error(w, "bad request: in and out are required", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	onProgress := func(p starnet.Progress) {
		if p.Line == "" {
			return
		}
		fmt.Fprintln(w, p.Line)
		flusher.Flush()
	}

	err := runner.RemoveStars(r.Context(), req.In, req.Out, starnet.Options{Stride: req.Stride}, onProgress)
	fmt.Fprintln(w, starnet.ResultLine(err))
	flusher.Flush()
}
