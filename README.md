<p align="center">
  <img src="docs/assets/logo.svg" width="120" height="120" alt="stacking-astro logo">
</p>

<h1 align="center">stacking-astro</h1>

<p align="center"><strong>English</strong> &middot; <a href="README.fr.md">Français</a></p>

<p align="center">
  <strong>An astrophotography stacking studio: auto-sort, calibrate, stack and finish what you shot.</strong><br>
  A Go engine and a Vue web UI that drive Siril and GIMP through nine recipes, from deep-sky and
  planetary to solar, comets, mosaics and Milky Way panoramas.
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> &middot;
  <a href="#how-it-works">How it works</a> &middot;
  <a href="#modes">Modes</a> &middot;
  <a href="#tech-stack">Tech stack</a> &middot;
  <a href="#documentation">Documentation</a>
</p>

<p align="center">
  <img alt="Go 1.23" src="https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&amp;logoColor=white">
  <img alt="Vue 3" src="https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&amp;logoColor=white">
  <img alt="Postgres 16" src="https://img.shields.io/badge/Postgres-16-4169E1?logo=postgresql&amp;logoColor=white">
  <img alt="Siril 1.4" src="https://img.shields.io/badge/Siril-1.4-5c6bc0">
  <img alt="Docker Compose" src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&amp;logoColor=white">
  <img alt="MIT licence" src="https://img.shields.io/badge/licence-MIT-blue">
</p>

---

A night of imaging leaves a folder of lights, darks, flats and bias frames, sometimes video, and
turning them into one image means sorting, calibrating, grading, registering, stacking and
finishing. **stacking-astro** points at a capture folder, works out what is in there, **processes**
it through one of nine recipes, and lets you **review** the result with its full provenance, plus a
**mosaic planner** for tiling a large object across panels.

Every step is a Go engine and a Vue web UI over tools that already do the hard parts well: **Siril**
for the heavy lifting, **GIMP** for the finish, with optional **GraXpert** and **StarNet++** and an
opt-in local vision model that critiques and re-tunes the finish. Those tools are invoked, never
bundled, and a missing optional one degrades the result instead of failing the run. The rig it was
built around is a Takahashi FC-100 DF with a ZWO ASI 1600MM Pro and a RedCat 51 with an ASI2600MC,
but the rig is just configuration: a DSLR, a one-shot-colour camera or an iPhone works with nothing
to set up.

This repository is the legacy Go and Vue engine. Its native macOS successor is
[kappa-sigma](https://github.com/verovec/kappa-sigma), which ports these rules to Swift while this
engine stays the reference.

## Quick start

**Prerequisites:** Docker (Desktop on macOS, running) and [`just`](https://github.com/casey/just).
Everything the pipeline drives is baked into the engine image; nothing else is installed on your
machine.

```bash
git clone https://github.com/verovec/stacking-astro.git && cd stacking-astro
just stack          # create .env and the data dirs, check Docker and the ports, build, run, print the URL
just stack-logs     # tail the containerized engine
just stack-down     # stop the engine and the frontend, keep Postgres
```

`just stack` is idempotent: re-run it any time. **The first build takes 15 to 40 minutes** and
produces a multi-GB image, because it bakes in Linux Siril, GIMP, GraXpert and ffmpeg; later runs
reuse it. Open the URL it prints (<http://localhost:8082> by default), go to **Processing, Import**,
point it at a capture folder under `./input`, and launch a run. Every page has a **help** button that
opens a guided tour of that page.

Host mode, the faster daily loop on macOS (the Go engine on the host driving your own Siril and
GIMP, only Postgres in Docker), is in [`docs/usage.md`](docs/usage.md#host-mode), and the
walkthrough with failure modes spelled out is [`docs/getting-started.md`](docs/getting-started.md).

## How it works

A run walks a capture folder through one pipeline, retuned per mode by a `Preset`, and writes a
per-run output directory `output/<object>/<runID>/` with a durable `run.json`:

```
  Inspect       walk the folder, classify lights, darks, flats, bias and video; mono or colour is detected, not configured
     |
  Calibrate     build or reuse the masters from the cross-session library, with dark defect maps
     |
  Grade         per-frame quality metrics, rejection rules, satellite and plane trail masking
     |
  Stack         register and stack per channel in Siril, or in the Go combiner for the algorithms Siril lacks
     |
  Combine       LRGB and the Ha/OIII/SII emission screens, palettes, plate-solve and SPCC colour calibration, stretch
     |
  Finish        GIMP composite; optional GraXpert, StarNet and the AI finish supervisor (render, judge, re-tune)
     |
  Review        run gallery with provenance, per-stage previews, full-resolution stage export, refine without re-stacking
```

There are two ways to run it. **`just stack`** puts everything in Docker: the engine image bakes in
Linux Siril, GIMP and GraXpert, and it is the one-command path for a new machine or a server.
**Host mode** runs the Go engine on your Mac against your own Siril and GIMP with only Postgres in
Docker; it is faster to iterate on and is what the Go tests exercise. That second mode is a
deliberate exception to the everything-in-a-container rule, because Siril and GIMP are desktop
applications that cannot run in a Linux container on macOS. See
[`docs/architecture.md`](docs/architecture.md) and, stage by stage,
[`docs/pipeline.md`](docs/pipeline.md).

## Modes

**Colour is automatic.** Every mode accepts monochrome filter-wheel frames or one-shot colour (a
DSLR or mirrorless raw in NEF, CR2, CR3, ARW, RAF or DNG, a colour camera's Bayer FITS, or plain RGB
TIFF, PNG or JPEG). It is detected while inspecting the folder and stacked as a single RGB channel,
with calibration applied in CFA space before demosaicing. Nothing to configure.

| Mode | Input | What it does |
|------|-------|--------------|
| [`deepsky`](docs/modes/deepsky.md) | mono FITS (L/R/G/B/Ha/OIII/SII), or colour | calibrate, grade, stack per channel, co-register, GIMP LRGB composite with Ha/OIII/SII emission screens (palettes: natural, HaRGB, HOO, SHO, HOS, Foraxx, mono). Colour finishes the RGB master directly |
| [`nebula`](docs/modes/nebula.md) | mono FITS, or colour | deepsky retuned for faint emission: lenient grading, Ha-forward, star reduction |
| [`milkyway`](docs/modes/milkyway.md) | one-shot colour (iPhone ProRAW/HEIC, DSLR raw) | photometric develop, sky-only stack, foreground composite and graded look; optional meteor recovery |
| [`nightpano`](docs/modes/nightpano.md) | a hand-swept arc of pointings | each panel stacked by the milkyway recipe, then plate-solved at a 70 degree field, fitted to one shared lens and reprojected onto a spherical canvas |
| [`planetary`](docs/modes/planetary.md) | video (SER/AVI/MP4/MOV) or stills | lucky imaging: sharpness rank, multi-point align, AP-weighted stack, deconvolve |
| [`comet`](docs/modes/comet.md) | timestamped FITS, mono or colour | dual star and comet stacks over one global alignment, plus an auto-fit motion track |
| [`mosaic`](docs/modes/mosaic.md) | overlapping panels of one large object | per-panel deepsky stacks, plate-solve each, reproject onto one canvas with a feathered blend |
| [`sun`](docs/modes/sun.md) | Ha or white-light video or stills | exposure-tier composite, limb-registered lucky imaging, PSF measured off the limb |
| [`eclipse`](docs/modes/eclipse.md) | a partially eclipsed Sun | the solar recipe fitted to two circles, the Moon masked out of the stack and every on-disc measurement; can render the whole event as one progression sheet |

Per-mode deep dives: [`docs/modes/`](docs/modes/README.md). From the CLI:
`just process MODE FORMAT PATH` (FORMAT is `image`, `video` or `both`); the full recipe table is in
[`docs/usage.md`](docs/usage.md).

## Tech stack

| Layer | Tech | Location |
|-------|------|----------|
| Engine | Go 1.23: CLI, stdlib `net/http` API with SSE progress, in-process worker pool (no Redis) | `cmd/astrostack`, `internal/` |
| Web UI | Vue 3, Pinia, Vue Router, Vite, TypeScript, Tailwind 3 | `frontend/` |
| Storage | Postgres 16 through raw `pgx/v5`, embedded SQL migrations, no ORM | `internal/store`, `migrations/` |
| Processing tools | Siril 1.4 driven by generated `.ssf` scripts, GIMP 2.10, ffmpeg; optional GraXpert and StarNet++ | `internal/siril`, `internal/postprocess`, `internal/graxpert`, `internal/starnet` |
| Containers | Postgres in Compose; the `stack` profile adds the engine image (Linux Siril, GIMP, GraXpert, ffmpeg) and an nginx frontend | `compose.yaml`, `docker/engine.Dockerfile`, `docker/frontend.Dockerfile` |
| AI finish supervisor | a local OpenAI-compatible vision model, opt-in (mlx on macOS, Ollama on Linux with a GPU) | `internal/llm`, `internal/pipeline/supervise_*.go` |
| MCP servers | `siril-mcp` (Go) and the vendored GIMP MCP (Python) for Claude | `cmd/siril-mcp`, `mcp-servers/gimp` |
| Tests | `go test` on the host (it exercises host `siril-cli`), `golangci-lint`, `vue-tsc` | `just check` |

Details in [Architecture](docs/architecture.md).

## Documentation

| Topic | Where |
|-------|-------|
| Start here: clone to first stacked image, with the failure modes | [`docs/getting-started.md`](docs/getting-started.md) |
| Recipes, prerequisites, host mode, the web UI | [`docs/usage.md`](docs/usage.md) |
| Every environment variable | [`docs/configuration.md`](docs/configuration.md) |
| System shape, components, containerized mode, provenance and tool health | [`docs/architecture.md`](docs/architecture.md) |
| How stacking is made, stage by stage | [`docs/pipeline.md`](docs/pipeline.md) |
| Per-mode deep dives, one page each | [`docs/modes/`](docs/modes/README.md) |
| The HTTP API reference | [`docs/api.md`](docs/api.md) |
| Every external tool, catalogue and library, with its licence | [`docs/third-party.md`](docs/third-party.md) |
| Everything else | [`docs/`](docs/README.md) |

## Contributing

Issues and pull requests are welcome. Run `just check` before pushing: it mirrors the pre-push gate
(`go vet`, `golangci-lint`, `vue-tsc` and the test suites, with Postgres up). The house conventions
are in [`conventions/`](conventions/), the project rules in [`CLAUDE.md`](CLAUDE.md), and the
verification recipes with objective pass criteria in
[`docs/verification.md`](docs/verification.md).

## License

[MIT](LICENSE) © 2026 Jordan Verove, for the code in this repository.

stacking-astro orchestrates a great deal of other people's work: Siril and GIMP do the stacking and
the finishing, and the sky itself comes from the HYG, ATHYG and OpenNGC catalogues and Gaia DR3.
Every tool is invoked rather than bundled, and every catalogue is downloaded at runtime under its
own terms. One of those terms binds a redistributor: the **HYG, ATHYG and OpenNGC catalogues are
CC BY-SA**. The complete list, with licences and the reasoning behind each choice, is in
[`docs/third-party.md`](docs/third-party.md).
