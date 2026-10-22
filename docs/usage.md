# Usage

The `just` recipes, the prerequisites of each run mode, host mode in full, the web UI and the
configuration pointers that used to live in the README. The one-command path is
[`just stack`](../README.md#quick-start); the walkthrough with failure modes is
[getting-started.md](getting-started.md).

## Recipes

`just` on its own lists every recipe. The ones you will actually use:

| Recipe | What it does |
|--------|--------------|
| `just` | List every recipe. |
| `just stack` / `just stack-down` / `just stack-logs` | **The whole app in Docker**: set up, build, run, report, print the URL; stop it; tail its logs. |
| `just doctor` | Which external tools this machine has, and what degrades without each one. |
| `just setup` / `just up` / `just down` | Host-mode first-run setup; start Postgres; stop the stack. |
| `just migrate` / `just migrate-down` | Apply or roll back schema migrations (`dev` migrates on boot, so this is rarely needed). |
| `just inspect DIR` | Print the classified inventory of a capture folder (no processing). |
| `just process MODE FORMAT PATH` | Full auto pipeline. MODE: `deepsky`, `nebula`, `milkyway`, `nightpano`, `planetary`, `comet`, `mosaic`, `sun`, `eclipse`; FORMAT: `image`, `video`, `both`. Pass-through flags after the path (for example `-v --supervise`). |
| `just video FILE` | Shortcut for `process planetary video` (lucky imaging). |
| `just refine RUNDIR` | Re-run **only** the finish (AI supervisor) on an existing run, no re-stacking. |
| `just dev` / `just web` | Host API with hot reload; Vue dev server. |
| `just run-ia-model` / `just ia-model-status` | Serve the local vision model (first run downloads about 28 GB); check it. |
| `just run-starnet-service` / `just run-graxpert-service` | Serve the host StarNet or GraXpert so a containerized engine can reach them. |
| `just download-catalogues` | Offline Gaia catalogues for plate-solving (about 3 GB; `-spcc` adds photometric colour calibration). |
| `just download-deepstars` | The 2.5M-star deep catalogue (nightpano astrometric anchoring, the mosaic planner's star field). |
| `just demo tour` | Record a narrated demo video of the UI ([tools/demo](../tools/demo/README.md)). |
| `just tour-shots` | Regenerate the in-app help-tour screenshots (re-run when the UI changes). |
| `just test` / `just lint` / `just fmt` | Test; lint and type-check; auto-format. |
| `just check` | Lint + test, the pre-push gate. |
| `just clean` | **Destructive**: drop containers, volumes and build output. |

The `just gitnexus-*` recipes drive an author-side code-graph index and need a tool that is not part
of this project: ignore them.

## Prerequisites

**Container mode (`just stack`), this is all you need:**

- [Docker](https://docs.docker.com/desktop/install/mac-install/) (Desktop on macOS, running) and
  [`just`](https://github.com/casey/just) (`brew install just`).

Everything the pipeline drives is baked into the engine image. Nothing else is installed on your
machine.

**Host mode, additionally, on the host itself:**

- **Required**: macOS (Apple Silicon recommended), Go 1.23+, Node 22 with pnpm,
  **Siril 1.4+** (`brew install --cask siril`), ffmpeg (`brew install ffmpeg`).
- **Recommended**: GIMP (the LRGB+Ha finish; absent, Siril `rgbcomp` is the fallback), LibRaw
  (`brew install libraw`, develops DSLR and phone raws), Python 3.12 (Siril's plate-solve and SPCC
  scripting).
- **Optional**: [GraXpert](https://www.graxpert.com) (AI background and denoise),
  [StarNet++ v2](https://www.starnetastro.com) (star reduction), a local vision model
  (`just run-ia-model`) for the [finish supervisor](agent.md).

Run **`just doctor`** to see which of these you have and what each missing one costs; it is the same
report `just stack` prints from inside the container. Copy-pasteable install commands are in
[getting-started.md](getting-started.md#2-install-the-prerequisites).

The optional tools are **soft-fail** (missing means a warning and a fallback; disable them with
`--no-ai`) and are *invoked, never bundled*: their licences stay with your install. StarNet is
therefore never in the image either, and under `just stack` how you reach it depends on the image's
architecture:

- **linux/amd64**: bind-mount your Linux x64 install and set `STARNET_BIN`.
- **linux/arm64** (any Apple Silicon build): there is nothing to mount. StarNet publishes no
  linux/arm64 build, and a macOS binary inside the Linux image is a Mach-O that will never run. Start
  the host service with **`just run-starnet-service`**; the containerized engine already calls it
  (`ASTRO_STARNET_URL`, default `http://host.docker.internal:8085`).

Without either, a containerized run keeps full stars and writes no star-presence set
(`final-starless.png`, `final-{25,50,75}-stars.png`). Either CLI generation works: the positional
`starnet++` and the flag-style `starnet2` are auto-detected (`STARNET_CLI` overrides). For
**offline plate-solving and SPCC**, download the Gaia catalogues once: `just download-catalogues`
(`just download-catalogues-spcc` adds the photometric chunks).

## Host mode

The faster daily loop on macOS: the engine on the host, Postgres in Docker. Siril and GIMP are
desktop applications that cannot run in a Linux container on macOS, so for daily development the Go
engine runs on the host and drives the ones you already have, a deliberate
[exception](architecture.md#deliberate-deviation) to the everything-in-Docker rule. It is faster to
iterate on and it is what the Go tests exercise.

```bash
git clone https://github.com/verovec/stacking-astro.git && cd stacking-astro
just setup                    # Go deps, dev tools, Siril MCP binary, frontend deps (idempotent)
just doctor                   # what is installed, and what degrades without it
just up                       # Postgres (Docker); the engine migrates itself on boot

mkdir -p input                # ASTRO_DATA_DIR, the root the UI may browse (git-ignored, so a
cp -r /path/to/captures input/M31   # clone has none of the data dirs; no sample data ships)

# A) the web UI (two terminals):
just dev                      # API on http://localhost:8080  (host; drives Siril/GIMP)
just web                      # UI  on http://localhost:5173

# B) or one-shot from the CLI:
just process deepsky image input/M31         # mono LRGB+Ha, or colour: auto-detected
just process planetary video input/moon.mp4  # lucky imaging
```

## The AI model

The finish supervisor's vision model (about 28 GB) is **opt-in and decoupled**: `just stack` never
downloads it. Add it with `just run-ia-model` (macOS, native Metal) or `just stack-ai` plus
`just ai-pull` (Linux with an NVIDIA GPU). The per-environment matrix, ports and stack variables are
in [architecture.md, Fully containerized mode](architecture.md#fully-containerized-mode-stack); what
the model does is in [agent.md](agent.md).

## The web UI

Page by page, with what each control means: [ui.md](ui.md).

- **Mosaic**: plan a tiled panel grid for a large object: catalogue target search, a rendered star
  field, tile grids computed from your optics, persisted plans a mosaic run can reference.
- **Processing**: four tabs. Import and inspect (multi-folder inventory, presets, launch), Tasks
  (SSE progress, pause and resume, per-stage rerun, the supervisor panel), Runs (on-disk gallery
  with full-resolution stage export), Library (calibration masters).

Every page has a **help** button that opens a guided tour of that page.

## Configuration

Everything is env-driven. Copy [`.env.example`](../.env.example) (fully commented, grouped) to
`.env`; `just` and Compose both load it, and secrets are never committed. The data roots are all
git-ignored, so **a fresh clone has none of them**: create `ASTRO_DATA_DIR` (default `./input`, the
only folder the UI may browse) before looking for your captures in the file browser. Flagship
variables: `SIRIL_BIN` and `GIMP_BIN` (host tools), `ASTRO_DATA_DIR`, `ASTRO_OUTPUT_DIR` and
`ASTRO_LIBRARY_DIR` (data roots), `ASTRO_LLM_URL` and `ASTRO_LLM_MODEL` (supervisor model),
`ASTRO_SPCC_SENSOR` (must match Siril's database), `ASTRO_LAT` and `ASTRO_LON` (observing site).
Full tables with defaults: [configuration.md](configuration.md).

## Development

- `just check` runs `go vet`, `golangci-lint`, `vue-tsc` and the test suites (it mirrors the
  pre-push gate). **Go tests run on the host** (they exercise host `siril-cli`); start Postgres
  first.
- House conventions live in [`conventions/`](../conventions/); project rules in
  [`CLAUDE.md`](../CLAUDE.md). MCP servers are registered in `.mcp.json` (build once with
  `just build-mcp`, included in `just setup`).
- Verification recipes with objective pass criteria: [verification.md](verification.md).
