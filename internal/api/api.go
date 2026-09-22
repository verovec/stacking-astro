// Package api exposes the engine over HTTP (stdlib net/http, Go 1.22+ routing): inspect a
// directory, browse capture folders, launch and track background jobs (with SSE progress),
// read the calibration library, and serve output images.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/klauspost/compress/gzhttp"
	"golang.org/x/sync/singleflight"

	"github.com/verove-jordan/astronomy/internal/buildinfo"
	"github.com/verove-jordan/astronomy/internal/config"
	"github.com/verove-jordan/astronomy/internal/inspect"
	"github.com/verove-jordan/astronomy/internal/job"
	"github.com/verove-jordan/astronomy/internal/mode"
	"github.com/verove-jordan/astronomy/internal/pipeline"
	"github.com/verove-jordan/astronomy/internal/preview"
	"github.com/verove-jordan/astronomy/internal/siril"
	"github.com/verove-jordan/astronomy/internal/store"
	"github.com/verove-jordan/astronomy/internal/thumb"
	"github.com/verove-jordan/astronomy/internal/toolhealth"
	"github.com/verove-jordan/astronomy/internal/turns"
)

// Server holds the API dependencies.
type Server struct {
	mgr         *job.Manager
	store       *store.Store
	cfg         *config.Config
	scanCache   *inspect.ScanCache
	agentTurns  *turns.Sessions     // live turns (supervised-job conversations), streamed over SSE
	toolHealth  *toolhealth.Checker // environment health (tool deep probes + catalogue presence)
	sirilRunner *siril.Runner       // one-off synchronous Siril work (star-annotation re-solve); nil-safe for tests
	starsFlight singleflight.Group  // dedupes concurrent star-annotation computes per run dir
}

// New builds the API server. hub is the shared turn transport (also handed to the job manager) so a
// supervised finish streams over one SSE mechanism.
func New(mgr *job.Manager, st *store.Store, cfg *config.Config, hub *turns.Sessions) *Server {
	s := &Server{
		mgr:         mgr,
		store:       st,
		cfg:         cfg,
		scanCache:   inspect.NewScanCache(),
		agentTurns:  hub,
		toolHealth:  toolhealth.New(cfg),
		sirilRunner: siril.New(cfg.SirilBin, siril.Limits{MaxCPUs: cfg.MaxCPUs, MemRatio: cfg.SirilMemRatio, Nice: cfg.SirilNice}),
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/environment", s.environment)
	mux.HandleFunc("POST /api/inspect", s.inspect)
	mux.HandleFunc("GET /api/browse", s.browse)
	mux.HandleFunc("GET /api/mode-params", s.modeParams)
	mux.HandleFunc("GET /api/presets", s.listPresets)
	mux.HandleFunc("POST /api/presets", s.savePreset)
	mux.HandleFunc("PUT /api/presets/{id}", s.updatePreset)
	mux.HandleFunc("DELETE /api/presets/{id}", s.deletePreset)
	mux.HandleFunc("POST /api/selections", s.saveSelection)
	mux.HandleFunc("PUT /api/selections/{id}", s.updateSelection)
	mux.HandleFunc("DELETE /api/selections/{id}", s.deleteSelection)
	mux.HandleFunc("GET /api/masters", s.masters)
	mux.HandleFunc("GET /api/phone-masters", s.phoneMasters)
	mux.HandleFunc("POST /api/reuse/preview", s.reusePreview)
	mux.HandleFunc("POST /api/calib/preview", s.calibPreview)
	mux.HandleFunc("POST /api/calib/plan", s.calibPlan)
	mux.HandleFunc("POST /api/planetary/align-points", s.planetaryAlignPoints)
	mux.HandleFunc("POST /api/quality/sets", s.setQuality)
	mux.HandleFunc("POST /api/jobs", s.createJob)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.cancelJob)
	mux.HandleFunc("POST /api/jobs/{id}/pause", s.pauseJob)
	mux.HandleFunc("POST /api/jobs/{id}/continue", s.continueJob)
	mux.HandleFunc("POST /api/jobs/{id}/restart", s.restartJob)
	mux.HandleFunc("POST /api/jobs/{id}/refine", s.refineJob)
	mux.HandleFunc("POST /api/jobs/{id}/rerun", s.rerunJob)
	mux.HandleFunc("GET /api/jobs/{id}/stages", s.listJobStages)
	mux.HandleFunc("POST /api/jobs/{id}/stages/export", s.exportJobStage)
	mux.HandleFunc("POST /api/jobs/{id}/denoise-final", s.denoiseFinalJob)
	mux.HandleFunc("POST /api/jobs/{id}/stars", s.computeStars)
	mux.HandleFunc("GET /api/jobs/{id}/stars", s.getStars)
	mux.HandleFunc("GET /api/jobs/{id}/scene3d", s.getScene3D)
	mux.HandleFunc("GET /api/galaxy/points", s.getGalaxyPoints)
	mux.HandleFunc("GET /api/jobs/{id}/iterations", s.jobIterations)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.jobEvents)
	mux.HandleFunc("POST /api/series", s.createSeries)
	mux.HandleFunc("GET /api/series", s.listSeries)
	mux.HandleFunc("GET /api/series/{id}", s.getSeries)
	mux.HandleFunc("POST /api/series/{id}/continue", s.setSeriesStatus("active"))
	mux.HandleFunc("POST /api/series/{id}/stop", s.setSeriesStatus("stopped"))
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("GET /api/processed", s.processed)
	mux.HandleFunc("GET /api/file", s.serveFile)
	mux.HandleFunc("GET /api/preview", s.previewFile)
	mux.HandleFunc("GET /api/thumb", s.serveThumb)
	mux.HandleFunc("GET /api/equipment", s.listEquipment)
	mux.HandleFunc("POST /api/equipment", s.saveEquipment)
	mux.HandleFunc("PUT /api/equipment/{id}", s.updateEquipment)
	mux.HandleFunc("DELETE /api/equipment/{id}", s.deleteEquipment)
	mux.HandleFunc("GET /api/sky/search", s.skySearch)
	mux.HandleFunc("GET /api/sky/starfield", s.skyStarfield)
	mux.HandleFunc("POST /api/mosaic/preview", s.mosaicPreview)
	mux.HandleFunc("GET /api/mosaic/plans", s.listMosaicPlans)
	mux.HandleFunc("POST /api/mosaic/plans", s.createMosaicPlan)
	mux.HandleFunc("GET /api/mosaic/plans/{id}", s.getMosaicPlan)
	mux.HandleFunc("PUT /api/mosaic/plans/{id}", s.updateMosaicPlan)
	mux.HandleFunc("DELETE /api/mosaic/plans/{id}", s.deleteMosaicPlan)
	mux.HandleFunc("PUT /api/mosaic/plans/{id}/tiles/{index}", s.setMosaicTileStatus)
	mux.HandleFunc("POST /api/mosaic/plans/{id}/reconcile", s.reconcileMosaicPlan)
	mux.HandleFunc("GET /api/local/drives", s.localDrives)
	mux.HandleFunc("GET /api/local/sources", s.localSources)
	mux.HandleFunc("GET /api/local/browse", s.localBrowse)
	mux.HandleFunc("GET /api/agent/turns/{id}/events", s.agentTurnEvents)
	mux.HandleFunc("POST /api/agent/turns/{id}/confirm", s.agentTurnConfirm)
	mux.HandleFunc("POST /api/agent/turns/{id}/message", s.agentTurnMessage)
	// CORS outermost so an OPTIONS preflight short-circuits before gzip. gzip compresses only JSON bodies
	// (allow-list) above a min size — PNG tiles (image/png) and SSE (text/event-stream) pass through
	// untouched, and gzhttp preserves http.Flusher so the streaming endpoints keep flushing.
	return cors(gzipJSON(mux))
}

// gzipJSON wraps a handler with content-type-scoped gzip (application/json only). Built once from static
// options, so the wrapper never errors; a misconfig would fail open (no compression) rather than panic.
var gzipJSON = func() func(http.Handler) http.Handler {
	wrap, err := gzhttp.NewWrapper(gzhttp.ContentTypes([]string{"application/json"}), gzhttp.MinSize(512))
	if err != nil {
		return func(h http.Handler) http.Handler { return h }
	}
	return func(h http.Handler) http.Handler { return wrap(h) } // wrap returns http.HandlerFunc (an http.Handler)
}()

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"data_dir":    s.cfg.DataDir,
		"output_dir":  s.cfg.OutputDir,
		"library_dir": s.cfg.LibraryDir,
		"engine":      map[string]string{"version": buildinfo.Version, "built_at": buildinfo.BuiltAt},
	})
}

// environment reports whether every external tool the pipeline drives can ACTUALLY run (deep
// probes, not binary lookups) plus the offline plate-solve catalogue situation — so the UI can warn
// before a run instead of the user diagnosing a silently-degraded image afterwards.
func (s *Server) environment(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.toolHealth.Report(r.Context()))
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid body")
		return
	}
	roots, ok := s.resolveRoots(body.Path, body.Paths)
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	inv, err := s.scanCache.ScanMany(r.Context(), roots, inspect.DefaultScanOptions())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// browseEntry is one row of the capture-folder browser. Local is always true now that the S3 mirror
// left the fork; the field survives so the UI contract stays stable.
type browseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Local bool   `json:"local,omitempty"`
}

// servableLocal returns the path when it exists on local disk. The S3 serve fallback left with the
// mirroring feature (E01 card 0012) — results and previews now live on local disk only.
func servableLocal(abs string) (string, bool) {
	if _, err := os.Stat(abs); err != nil {
		return "", false
	}
	return abs, true
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		path = s.cfg.DataDir
	}
	abs, ok := s.withinData(path)
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		serverError(w, err)
		return
	}
	// Directories first, then files — each group already name-sorted by os.ReadDir. Files are listed
	// (so the browser shows folder contents) but are not selectable for processing; dotfiles are hidden.
	var dirs, files []browseEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		it := browseEntry{Name: e.Name(), Path: filepath.Join(abs, e.Name()), IsDir: e.IsDir(), Local: true}
		if e.IsDir() {
			dirs = append(dirs, it)
		} else {
			files = append(files, it)
		}
	}
	out := append(dirs, files...)
	if out == nil {
		out = []browseEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": abs, "entries": out})
}

// modeParams returns a stacking mode's effective tunable knobs (the values in effect for its preset)
// plus the human-readable knob menu, so the UI can prefill the Advanced-parameters box. It flattens
// through the SAME pipeline.ParamsFor the run merge uses, so the prefill can never drift from what the
// run actually applies. An empty/unknown mode falls back to deep-sky. GET /api/mode-params?mode=deepsky
func (s *Server) modeParams(w http.ResponseWriter, r *http.Request) {
	mo, err := mode.ParseMode(r.URL.Query().Get("mode"))
	if err != nil {
		mo = mode.Deepsky
	}
	preset := mode.For(mo)
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":     string(mo),
		"defaults": pipeline.ParamsFor(preset),
		"ranges":   pipeline.KnobRangesFor(mo),
		"menu":     pipeline.KnobMenuFor(mo),
		// The stacking-algorithm catalogue behind the launch form's "Stacking & rejection" panel.
		// Served from the engine so the dropdown can never offer an algorithm it cannot run; null
		// for the modes that stack natively (planetary/sun/milkyway).
		"stack_menu": pipeline.StackMenuFor(mo),
	})
}

func (s *Server) masters(w http.ResponseWriter, r *http.Request) {
	masters, err := s.store.ListMasters(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"masters": masters})
}

// phoneMasters returns the reusable phone/DSLR calibration masters (iPhone DNG darks/bias/flats) built
// by the milkyway path, keyed by ISO/exposure/dimensions.
func (s *Server) phoneMasters(w http.ResponseWriter, r *http.Request) {
	masters, err := s.store.ListPhoneMasters(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"masters": masters})
}

// reusePreview reports the prior light sessions and added integration a run on the given directory
// would fold in (the "auto-discover + confirm" data), without processing.
func (s *Server) reusePreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid body")
		return
	}
	roots, ok := s.resolveRoots(body.Path, body.Paths)
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	if !s.cfg.ReuseEnabled {
		writeJSON(w, http.StatusOK, &pipeline.ReusePreview{Object: ""})
		return
	}
	pv, err := pipeline.PreviewReuseMany(r.Context(), s.store, s.scanCache, roots, s.cfg.SirilCatalogDir, s.cfg.ReuseConeDeg)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

// calibPreview reports which library master dark/flat/bias would calibrate each inspected light channel,
// so the Import "Calibration" panel can show + let the user exclude them. POST /api/calib/preview
func (s *Server) calibPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
		Force bool     `json:"force"` // apply mismatched masters anyway (force_calibration_frames preview)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid body")
		return
	}
	roots, ok := s.resolveRoots(body.Path, body.Paths)
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	pv, err := pipeline.PreviewCalibration(r.Context(), s.scanCache, s.store, roots, body.Force)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

// calibPlan reports the JOINED per-session run plan: every (session, night, config) group the run
// would form — current capture nights AND folded prior sessions — with the dark/flat/bias each would
// get (library / capture-built / per-night session rebuild). POST /api/calib/plan
func (s *Server) calibPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path          string   `json:"path"`
		Paths         []string `json:"paths"`
		Force         bool     `json:"force"`
		ReuseDisabled bool     `json:"reuse_disabled"`
		Sessions      []int64  `json:"reuse_sessions"` // restrict folded prior sessions (empty = all)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid body")
		return
	}
	roots, ok := s.resolveRoots(body.Path, body.Paths)
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	var provider pipeline.ReuseProvider
	if s.cfg.ReuseEnabled && !body.ReuseDisabled {
		provider = s.store
	}
	var sessions map[int64]bool
	if len(body.Sessions) > 0 {
		sessions = make(map[int64]bool, len(body.Sessions))
		for _, id := range body.Sessions {
			sessions[id] = true
		}
	}
	pv, err := pipeline.PreviewRunPlan(r.Context(), provider, s.scanCache, s.store, roots,
		s.cfg.SirilCatalogDir, s.cfg.ReuseConeDeg, body.Force, sessions)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req job.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid body")
		return
	}
	roots, ok := s.resolveRoots(req.Path, req.Paths)
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	req.Path = roots[0] // primary dir: session, target lock, run naming
	if len(roots) > 1 {
		req.Paths = roots // multi-folder selection, merged into one session
	} else {
		req.Paths = nil // single folder → unchanged single-session run
	}
	if req.BuildMasters {
		// A masters-only calibration build is not a pipeline mode — the job kind reads "masters" in
		// history and execute() intercepts it before mode parsing (like Transfer/Backup).
		req.Mode = "masters"
	} else {
		if req.Mode == "" {
			req.Mode = string(mode.Deepsky)
		}
		if req.Format == "" {
			req.Format = string(mode.FormatImage)
		}
		if _, err := mode.ParseMode(req.Mode); err != nil {
			badRequest(w, err.Error())
			return
		}
		if _, err := mode.ParseFormat(req.Format); err != nil {
			badRequest(w, err.Error())
			return
		}
	}
	id, err := s.mgr.Enqueue(r.Context(), req)
	if err != nil {
		serverError(w, err)
		return
	}
	// turn_id is non-empty for a supervised run so the client can open its live conversation; "" otherwise.
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "turn_id": s.mgr.TurnFor(id)})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": s.mgr.Cancel(id)})
}

// pauseJob asks a running job to pause at its next safe boundary so it can be resumed later. Returns
// {"paused": bool} — false when the job is not running in this process. POST /api/jobs/{id}/pause
func (s *Server) pauseJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"paused": s.mgr.Pause(id)})
}

// continueJob resumes a paused job from its checkpoint (re-pushing a kept result, or reusing the channel
// masters already stacked). POST /api/jobs/{id}/continue
func (s *Server) continueJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	if err := s.mgr.Continue(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id})
}

// restartJob re-runs a finished (failed/cancelled) job as a new job with the same parameters and returns
// the new job id, so the UI can navigate to it. POST /api/jobs/{id}/restart
func (s *Server) restartJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	newID, err := s.mgr.Restart(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	// turn_id is set when the restarted job is supervised, so its live AI-finish panel binds to the
	// NEW job's turn (Restart replays the stored request, which may carry Supervise). Empty otherwise.
	writeJSON(w, http.StatusAccepted, map[string]any{"id": newID, "turn_id": s.mgr.TurnFor(newID)})
}

// refineJob re-finishes a completed run under the AI supervisor (no re-stack) as a new job, and returns
// the new job id so the UI can follow its live iteration stream. The optional body tunes the loop.
// POST /api/jobs/{id}/refine  { "max_iters"?: int, "tier"?: "A"|"B"|"C", "allow_restack"?: bool }
func (s *Server) refineJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	var req job.RefineRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid body")
			return
		}
	}
	req.RunDir = "" // never trust a client path; Refine resolves it from the source run
	newID, err := s.mgr.Refine(r.Context(), id, req)
	if err != nil {
		serverError(w, err)
		return
	}
	// A refine is always supervised, so turn_id is set — the UI opens its live conversation immediately.
	writeJSON(w, http.StatusAccepted, map[string]any{"id": newID, "turn_id": s.mgr.TurnFor(newID)})
}

// rerunJob re-runs a completed deepsky/nebula run from the stage an edited parameter requires, in place,
// as a new job (non-supervised) — the manual counterpart of refineJob. Returns the new job id so the UI
// can follow its live progress. POST /api/jobs/{id}/rerun  { "stage"?: string, "params"?: object }
func (s *Server) rerunJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	var req struct {
		Stage  string          `json:"stage"`
		Params json.RawMessage `json:"params"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid body")
			return
		}
	}
	newID, err := s.mgr.Rerun(r.Context(), id, req.Stage, req.Params)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": newID})
}

// denoiseFinalJob runs GraXpert AI denoise on a completed run's final image on demand (offloaded to the
// host GraXpert service when configured) and returns the new job id. POST /api/jobs/{id}/denoise-final
func (s *Server) denoiseFinalJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	newID, err := s.mgr.DenoiseFinal(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": newID})
}

// listJobs returns a page of jobs newest-first (id desc = date desc) with the total, so the Tasks page
// paginates ("load more") instead of loading the entire history. GET /api/jobs?offset=&limit=
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	offset := clampAtoi(r.URL.Query().Get("offset"), 0, 0, 1<<30)
	limit := clampAtoi(r.URL.Query().Get("limit"), 20, 1, 100)
	jobs, err := s.store.ListJobs(r.Context(), limit, offset)
	if err != nil {
		serverError(w, err)
		return
	}
	total, _ := s.store.CountJobs(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs, "total": total, "offset": offset, "limit": limit,
	})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	jb, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, jb)
}

// jobIterations returns a run's AI-supervised finish iterations (tier, scores, defects, chosen), so the
// agent (and UI) can read a refine's history. GET /api/jobs/{id}/iterations
func (s *Server) jobIterations(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	iters, err := s.store.ListFinishIterations(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"iterations": iters})
}

func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "invalid job id")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		serverError(w, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // never let a proxy buffer the live stream

	events, unsubscribe := s.mgr.Subscribe(id)
	defer unsubscribe()

	// Send a snapshot first so a late subscriber sees current state — including the latest preview and the
	// milestone previews accumulated so far, so a page reloaded mid-run restores its live preview and
	// intermediary-image timeline instead of waiting for the next live event.
	jb, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		// Without the snapshot we cannot know the job is terminal — dangling here would hold a silent
		// SSE open forever on a finished job. Close instead; the client retries or shows the fetched job.
		return
	}
	done := isTerminal(jb.Status)
	snap := job.Event{JobID: id, Status: jb.Status, Progress: jb.Progress, Step: jb.CurrentStep, Done: done}
	if !done {
		snap.Preview, snap.StagePreviews = s.mgr.PreviewSnapshot(id)
	}
	sendEvent(w, flusher, snap)
	if done {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-events:
			if !open {
				return
			}
			sendEvent(w, flusher, e)
			if e.Done {
				return
			}
		}
	}
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	abs, ok := s.within(r.URL.Query().Get("path"), s.cfg.OutputDir)
	if !ok {
		badRequest(w, "path must be inside the output directory")
		return
	}
	served, ok := servableLocal(abs)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Result images live at STABLE paths (e.g. output/<obj>_stack.png), so a re-run overwrites the same
	// URL. Force revalidation (ServeFile still answers 304 via Last-Modified when unchanged) so a fresh
	// stack/render is never masked by a cached copy from the previous run.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, served)
}

// previewFile decodes a capture file (FITS/TIFF/raw/PNG/JPEG) under the data dir into a downsampled,
// linearly-normalized 16-bit buffer the viewer stretches client-side. Streams a compact binary body
// (header + uint16 samples; see preview.Preview.Encode), not JSON.
func (s *Server) previewFile(w http.ResponseWriter, r *http.Request) {
	abs, ok := s.withinData(r.URL.Query().Get("path"))
	if !ok {
		badRequest(w, "path must be inside the data directory")
		return
	}
	if !preview.SupportedExt(abs) {
		badRequest(w, "unsupported file type for preview")
		return
	}
	served, ok := servableLocal(abs)
	if !ok {
		http.NotFound(w, r)
		return
	}
	maxEdge := s.cfg.PreviewMaxEdge
	if q := r.URL.Query().Get("max"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			maxEdge = n
		}
	}
	pv, err := preview.Load(r.Context(), served, maxEdge)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_ = pv.Encode(w)
}

// serveThumb returns a small JPEG thumbnail of a run output image. The gallery uses this instead of the
// full-resolution PNG — the page's main slowdown. Confined to the output dir and disk-cached.
func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request) {
	abs, ok := s.within(r.URL.Query().Get("path"), s.cfg.OutputDir)
	if !ok {
		badRequest(w, "path must be inside the output directory")
		return
	}
	served, ok := servableLocal(abs)
	if !ok {
		http.NotFound(w, r)
		return
	}
	dim := clampAtoi(r.URL.Query().Get("w"), 480, 32, 1024)
	data, err := thumb.Cached(filepath.Join(s.cfg.WorkDir, "cache", "thumbs"), served, dim, 80)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// atoi64 parses an optional epoch-ms query parameter. Anything unparseable reads as 0, which every
// caller treats as "do not filter on it" — a malformed date must narrow nothing, not everything.
func atoi64(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// clampAtoi parses s as an int (falling back to def) and clamps the result into [lo, hi].
func clampAtoi(s string, def, lo, hi int) int {
	n := def
	if v, err := strconv.Atoi(s); err == nil {
		n = v
	}
	if n < lo {
		n = lo
	}
	if n > hi {
		n = hi
	}
	return n
}

// runSummary is one durable run discovered on disk (see GET /api/runs).
type runSummary struct {
	Object       string   `json:"object"`
	RunID        string   `json:"run_id"`
	Dir          string   `json:"dir"`
	RunJSON      string   `json:"run_json"`
	FinalPreview string   `json:"final_preview,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Channels     []string `json:"channels,omitempty"`
	Engine       string   `json:"engine,omitempty"` // build that produced the run (stale-engine chip)
	CreatedAtMs  int64    `json:"created_at_ms"`
}

// listRuns scans the output directory for run.json records so any past run can be reopened from disk,
// independent of the database (e.g. CLI runs). Results are paginated (newest first) so a large gallery
// stays fast: every run is cheaply stat-ed for ordering, but only the requested page is read+summarized.
// runFileRef points at one run's run.json for the gallery.
type runFileRef struct {
	path  string // absolute local run.json path
	mtime int64
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	outAbs, err := filepath.Abs(s.cfg.OutputDir)
	if err != nil {
		serverError(w, err)
		return
	}
	matches, _ := filepath.Glob(filepath.Join(outAbs, "*", "*", "run.json"))

	// Order by mtime (newest first) with a cheap Stat per file — no run.json is read here.
	files := make([]runFileRef, 0, len(matches))
	seen := make(map[string]bool, len(matches))
	for _, p := range matches {
		var mtime int64
		if info, err := os.Stat(p); err == nil {
			mtime = info.ModTime().UnixMilli()
		}
		files = append(files, runFileRef{path: p, mtime: mtime})
		seen[p] = true
	}

	q := r.URL.Query()

	sort.Slice(files, func(i, j int) bool { return files[i].mtime > files[j].mtime })

	total := len(files)
	offset := clampAtoi(q.Get("offset"), 0, 0, total)
	limit := clampAtoi(q.Get("limit"), 12, 1, 100)
	end := offset + limit
	if end > total {
		end = total
	}

	runs := make([]runSummary, 0, end-offset)
	for _, f := range files[offset:end] {
		runs = append(runs, summarizeRun(f.path)) // ReadFile only for the page
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runs": runs, "total": total, "offset": offset, "limit": limit,
	})
}

func summarizeRun(runJSONPath string) runSummary {
	var mtime int64
	if info, err := os.Stat(runJSONPath); err == nil {
		mtime = info.ModTime().UnixMilli()
	}
	data, _ := os.ReadFile(runJSONPath)
	return summarizeRunBytes(data, runJSONPath, mtime)
}

// summarizeRunBytes derives a run summary from run.json bytes (empty when unreadable) and the local path the
// run occupies — shared by the on-disk gallery and the S3-mirror fallback (summarizeS3Run). Output paths in
// run.json are absolute under OutputDir, so its previews resolve through local-first/S3-fallback serving.
func summarizeRunBytes(data []byte, runJSONPath string, mtimeMs int64) runSummary {
	sum := runSummary{
		Dir:         filepath.Dir(runJSONPath),
		RunJSON:     runJSONPath,
		RunID:       filepath.Base(filepath.Dir(runJSONPath)),
		Object:      filepath.Base(filepath.Dir(filepath.Dir(runJSONPath))),
		CreatedAtMs: mtimeMs,
	}
	if len(data) > 0 {
		var rj struct {
			Object string `json:"object"`
			RunID  string `json:"run_id"`
			Engine string `json:"engine"`
			Final  *struct {
				Mode     string   `json:"mode"`
				Channels []string `json:"channels"`
				Outputs  []string `json:"outputs"`
			} `json:"final"`
		}
		if json.Unmarshal(data, &rj) == nil {
			if rj.Object != "" {
				sum.Object = rj.Object
			}
			if rj.RunID != "" {
				sum.RunID = rj.RunID
			}
			sum.Engine = rj.Engine
			if rj.Final != nil {
				sum.Mode, sum.Channels = rj.Final.Mode, rj.Final.Channels
				for _, o := range rj.Final.Outputs {
					if strings.HasSuffix(o, ".png") {
						sum.FinalPreview = o
						break
					}
				}
			}
		}
	}
	return sum
}

// isTerminal reports whether a job status is final (no more events will arrive).
func isTerminal(status string) bool {
	return status == store.JobSucceeded || status == store.JobFailed || status == store.JobCancelled
}

// --- helpers ---

func (s *Server) withinData(p string) (string, bool) { return s.within(p, s.cfg.DataDir) }

// resolveRoots confines each selected capture folder to the data dir and returns the cleaned absolute
// paths. A legacy single `path` is treated as a one-element list when `paths` is empty. Returns
// ok=false (caller replies 400) when nothing is given or any path escapes the data dir.
func (s *Server) resolveRoots(path string, paths []string) ([]string, bool) {
	if len(paths) == 0 && path != "" {
		paths = []string{path}
	}
	if len(paths) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, ok := s.withinData(p)
		if !ok {
			return nil, false
		}
		out = append(out, abs)
	}
	return out, true
}

func (s *Server) within(p, root string) (string, bool) {
	if p == "" {
		return "", false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	if abs == rootAbs || strings.HasPrefix(abs, rootAbs+string(os.PathSeparator)) {
		return abs, true
	}
	return "", false
}

func sendEvent(w http.ResponseWriter, f http.Flusher, e job.Event) {
	b, _ := json.Marshal(e)
	fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONCached writes v as JSON with an ETag + Cache-Control, but returns 304 Not Modified (no body) when
// the request's If-None-Match matches — so re-fetching an unchanged resource (e.g. the weather frames index)
// is a tiny conditional round-trip instead of a full re-download. Empty etag → a plain writeJSON.
func writeJSONCached(w http.ResponseWriter, r *http.Request, status int, etag, cacheControl string, v any) {
	if etag == "" {
		writeJSON(w, status, v)
		return
	}
	w.Header().Set("ETag", etag)
	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, status, v)
}

// etagMatches reports whether the If-None-Match header (a comma list, tokens optionally W/-prefixed, or "*")
// contains etag. Weak/strong prefixes are ignored (our ETags are weak validators).
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, tok := range strings.Split(header, ",") {
		tok = strings.TrimPrefix(strings.TrimSpace(tok), "W/")
		if tok == "*" || tok == want {
			return true
		}
	}
	return false
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// The app is normally same-origin (a reverse proxy forwards /api), so CORS is unused. But if BASE is
		// pointed at this engine cross-origin, tiles are loaded via <img>: allow that explicitly so a browser
		// never opaque-blocks a tile (or masks an error body as a bare net::ERR). Harmless for same-origin.
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
