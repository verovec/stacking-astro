# HTTP API reference

The engine serves a JSON API on `API_ADDR` (default `:8080`); the web UI is its only intended
client, but every endpoint is plain HTTP and works headless (`curl`). **There is no
authentication** — the API is designed for a trusted local network; do not expose it publicly.
Routes are registered in `internal/api/api.go`.

Conventions: request/response bodies are JSON; errors are `{"error": "..."}` with a 4xx/5xx
status; paths in query/body must live under the configured data/output roots (the server
rejects anything else); two endpoints stream Server-Sent Events (SSE).

## Health & environment

| Method + path | Purpose |
|---|---|
| `GET /api/health` | Engine status, build stamp, configured dirs |
| `GET /api/environment` | Tool availability (Siril/GIMP/GraXpert/StarNet/model), catalogue status |

## Inspection & presets

| Method + path | Purpose |
|---|---|
| `POST /api/inspect` | Classify a capture folder (frames, sets, warnings) |
| `GET /api/browse` | Browse the data dir (folders, capture hints) |
| `GET /api/local/drives` · `/browse` · `/sources` | Removable-drive browsing; `/sources` lists the app's configured roots (data/output/work) |
| `GET /api/mode-params` | Per-mode tunable parameter schema |
| `GET /api/presets` | Built-in + user presets |
| `POST /api/presets` · `PUT /api/presets/{id}` · `DELETE /api/presets/{id}` | Save / rename / delete a user preset |
| `GET/POST /api/selections` (+ `PUT/DELETE /{id}`) | Saved multi-folder import selections |
| `GET /api/masters` · `GET /api/phone-masters` | Calibration library contents |
| `POST /api/reuse/preview` | What a cross-session reuse plan would fold in |
| `POST /api/calib/preview` | Which masters would match a light set |
| `POST /api/calib/plan` | Per-night calibration mapping preview for a grouped run |
| `POST /api/quality/sets` | Per-set quality statistics for an inspected inventory |
| `POST /api/planetary/align-points` | Preview the planetary alignment-point grid |

## Jobs & series

| Method + path | Purpose |
|---|---|
| `POST /api/jobs` | Launch a processing job |
| `GET /api/jobs` · `GET /api/jobs/{id}` | List / fetch (result embedded when finished) |
| `POST /api/jobs/{id}/cancel` · `/pause` · `/continue` · `/restart` | Lifecycle (pause/resume is checkpointed) |
| `POST /api/jobs/{id}/refine` | Supervised re-finish of a completed run (no re-stack) |
| `POST /api/jobs/{id}/rerun` | Re-enter from an edited stage (per-stage rerun) |
| `POST /api/jobs/{id}/denoise-final` | Extra denoise pass on the final image |
| `GET /api/jobs/{id}/iterations` | Supervisor iterations of a run |
| `GET /api/jobs/{id}/stages` · `POST /api/jobs/{id}/stages/export` | Stage timeline / full-resolution stage export |
| `GET /api/jobs/{id}/events` | **SSE** — progress, log lines, previews, resources. Sends a snapshot first; for a finished job it closes immediately after the snapshot (clients should not stream terminal jobs) |
| `POST /api/series` · `GET /api/series` · `GET /api/series/{id}` | Goal-driven improvement campaigns |
| `POST /api/series/{id}/continue` · `/stop` | Resume / stop a campaign |

### Run-request fields the engine otherwise guesses

`POST /api/jobs` takes `{"path","mode","format"}` at minimum. Three fields are worth calling out
because **omitting them is not neutral** — the engine substitutes its own answer:

| Field | Values | Omitted → |
|---|---|---|
| `color_model` | `auto` \| `mono` \| `osc` | `auto`: the scan's own mono/OSC verdict decides. An explicit value drops the lights that contradict it and errors if none match. Invalid values are rejected with 400. |
| `focal_mm` | mm, > 0 | the engine's **configured** telescope focal length. A camera-lens session solved at that scale cannot plate-solve at all. |
| `pixel_um` | µm, > 0 | the frame's own `XPIXSZ`, else the configured rig's pixel size. |

## Supervised conversations

| Method + path | Purpose |
|---|---|
| `GET /api/agent/turns/{id}/events` | **SSE** — streamed supervised-run passes |
| `POST /api/agent/turns/{id}/confirm` | Approve/deny a gated action (e.g. Tier-C re-stack) |
| `POST /api/agent/turns/{id}/message` | Steer a live supervised run with free text |

## Runs & files

| Method + path | Purpose |
|---|---|
| `GET /api/runs` | Completed runs (from `run.json` on disk) |
| `GET /api/processed` | Per-folder processed status |
| `GET /api/file` | Serve an output file |
| `GET /api/preview` | Downsampled linear preview buffer of a capture file |
| `GET /api/thumb` | Cached thumbnail |

## Mosaic planning & sky search

| Method + path | Purpose |
|---|---|
| `GET /api/sky/search` | Catalogue target search (name → coordinates, type, size) |
| `GET /api/sky/starfield` | Rendered star-field cutout around a centre (deep catalogue) |
| `GET/POST /api/mosaic/plans` (+ `GET/PUT/DELETE /{id}`) | Persisted mosaic tile plans |
| `PUT /api/mosaic/plans/{id}/tiles/{index}` | Update one tile of a plan |
| `POST /api/mosaic/plans/{id}/reconcile` | Reconcile a plan against captured panels |
| `POST /api/mosaic/preview` | Preview a tile grid for an object + optics |

## Equipment

| Method + path | Purpose |
|---|---|
| `GET/POST /api/equipment` (+ `PUT/DELETE /{id}`) | Saved equipment setups (optics/camera profiles) |
