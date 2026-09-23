# Architecture

AstroStack drives **host-installed Siril and GIMP** to sort, grade, calibrate and stack
astrophotography captures. Because those are macOS app bundles that cannot run in a Linux container,
the engine runs on the host and only the support services are containerized.

```
┌─────────────────────────── HOST (macOS) ───────────────────────────┐
│  astrostack (Go)                          siril-mcp (Go, stdio)      │
│   • CLI: inspect / process / video         • MCP tools for Claude    │
│   • HTTP API + SSE progress  (stdlib)       (shares internal/ pkgs)  │
│   • in-process job worker pool                                       │
│        │ exec                 │ exec               │ exec            │
│        ▼                      ▼                    ▼                 │
│   siril-cli              gimp-console-2.10        ffmpeg             │
│   (Siril.app)           (via vendored GIMP MCP)  (video frames)      │
│        │ TCP localhost:5432                                          │
└────────┼────────────────────────────────────────────────────────────┘
         ▼
┌──────────────── docker compose ────────────────┐
│  db (Postgres, named volume, healthcheck)        │
│  frontend (Vue → nginx)            [profile web] │
│  adminer (DB UI)                  [profile tools] │
└──────────────────────────────────────────────────┘
```

On the same host and in the same way (`exec` + parse stdout), the engine also drives the **optional**
tools — **GraXpert** and **StarNet++** (AI background extraction / star removal) and, only when a run
opts in, a local OpenAI-compatible **vision-model server** (`:1234`, e.g. mlx-vlm via `just
run-ia-model`) that auto-tunes the finish. All three **soft-fail**: absent or unreachable, the run
falls back to the pure Siril/GIMP path. The browser's Mosaic planner also calls the engine's
`GET /api/sky/search` (catalogue name→coordinate target search) and `GET /api/sky/starfield`
(a deep-catalogue star-field cutout), both served from local/embedded catalogues.

Every external tool, catalogue, data service and library — with its licence and what breaks without it
— is catalogued in [third-party.md](third-party.md). Nothing is bundled: host tools are invoked, online
feeds are fetched at runtime and cached, and both soft-fail.

## Components

| Package | Responsibility |
|---------|----------------|
| `internal/config` | Environment configuration. |
| `internal/fits` | Read FITS headers + pixels — hand-rolled, no external FITS library. |
| `internal/inspect` | Walk a directory, classify each file (light/dark/flat/bias/dark-flat/video), group into sets. Bare-filename legacy captures are labeled from an `info.txt` sidecar (`manifest.go`) that lists the per-sub-run filter order + gain. |
| `internal/siril` | `SirilRunner`: generate `.ssf`, exec `siril-cli`, parse `progress:`/`log:` + `seqstat` CSV. |
| `internal/grade` | Per-frame quality metrics + rejection rules; trail handling. |
| `internal/stacknative` | The Go pixel combiner for the algorithms Siril lacks (trimmed mean, Robust Chauvenet, DSS auto-adaptive / entropy-weighted, local normalization) — streams registered frames in row bands; validated against Siril for the algorithms both implement. |
| `internal/stackalg` | The canonical catalogue of frame-combination and pixel-rejection algorithms (engine-free): what exists, which engine runs it, what its parameters mean, and the count-adaptive default. One source of truth behind the Siril clause, the knob whitelist and the UI menu — see [stacking.md](stacking.md). |
| `internal/calib` | Build master calibration frames (+ the dark **defect map** / bad-pixel scan); match the right masters to each light set; calibration library + deep cross-session pools. |
| `internal/transient` | Cross-frame satellite/plane-trail + cosmic-ray masking on the registered subs, validated against fixed-pattern noise. |
| `internal/photom` | Photometric normalization across mixed-session groups (percentile-curve fit; ON by default for deep-sky — a flat narrowband curve seeds from the header exposure/gain instead of mis-fitting, and the clamp admits genuine cross-gain ratios). |
| `internal/dither` | Pointing-pattern diagnosis from registration offsets (dithered / drift / static) — the walking-noise advisory. |
| `internal/noise` · `internal/imgops` · `internal/optics` | Noise measurement/starlet denoiser, shared image ops, flat-defect QC. |
| `internal/pipeline` | Orchestrate inspect → masters → calibrate → grade → register → stack → combine; soft-fail AI steps in `enhance.go`; palettes, supervisor, per-stage rerun. |
| `internal/preset` | The built-in "best params per situation" catalog (29 recipes) merged with user presets, plus the object-type taxonomy (`ObjectType`) that drives the launch form's target chips. |
| `internal/postprocess` | LRGB+Ha channel combine, color calibration, stretch; optional GIMP touch-ups. |
| `internal/graxpert` | Optional host CLI: GraXpert AI background-gradient extraction / denoise (`GRAXPERT_BIN`). |
| `internal/starnet` | Optional host CLI: StarNet star removal for star-reduced finishing + the star-presence set (`STARNET_BIN`; both the positional StarNet++ v2 and flag-style StarNet2 CLIs, auto-detected). Offloads to the host service (`cmd/starnet-host`, `ASTRO_STARNET_URL`) when no local binary resolves — the containerized case. |
| `internal/llm` | Optional, opt-in: drives a host-run OpenAI-compatible vision model to auto-tune the finish for **every stacking mode** — deep-sky/nebula composite, comet colour composite, milkyway grade, planetary sharpen — via per-mode `candidateRenderer` adapters (`internal/pipeline/supervise_*.go`); the shared render→judge→re-tune loop soft-fails when the server is down. |
| `internal/planetary` | SER/AVI/MP4/MOV/stills lucky-imaging path: native-res disk-masked sharpness ranking, multi-point ZNCC alignment, per-AP top-K selection stack (each region built from its locally-sharpest frames), RL deconvolution, true-luminance colour compose (`true_lum`). Opt-in earthshine reveal (`earthshine_gain`): deterministic limb circle fit + SNR-gated lift of the unlit disc, composited after the Siril finish. |
| `internal/comet` | Pure comet primitives: multi-scale coma detection, robust linear/quadratic track fit, starless ZNCC alignment, sub-pixel translate (driven by `pipeline.ProcessComet`). |
| `internal/mode` | Capture modes (deepsky/nebula/milkyway/planetary/comet/…) → the `Preset` that retunes the whole pipeline. |
| `internal/nightscape` | Milky-Way / one-shot-color foreground+sky composite recipe. |
| `internal/rawconv` | Camera-raw → 16-bit TIFF develop for Siril ingestion: LibRaw `dcraw_emu` preferred (photometric, `-t 0`, exact sRGB), macOS `sips` fallback; also raw thumbnails. |
| `internal/buildinfo` | Engine build identity injected via `-ldflags` (`Version`/`BuiltAt`) — stamped into `/api/health` and every `run.json`. |
| `internal/toolhealth` | Deep environment probes (Siril/GIMP/GraXpert/StarNet/raw developer/LLM + offline-catalogue presence) behind `/api/environment`. |
| `internal/store` | Postgres access via **raw `pgx/v5`** (no ORM/sqlc); schema applied from embedded, versioned `*.up.sql` migrations (`store.Migrate`, tracked in `schema_migrations`). |
| `internal/job` | In-process worker pool (parallel / sequential lanes), job lifecycle incl. pause/resume + cancel semantics, per-input-dir locking. |
| `internal/turns` | The live conversation transport behind supervised runs (steer / confirm / stream). |
| `internal/api` | HTTP handlers (Go 1.22 `http.ServeMux` method routing) + SSE progress; the mosaic-planner sky endpoints (`/api/sky/search`, `/api/sky/starfield`). |
| `astro` · `skycat` · `skyplan` | Ephemeris, sky-object catalog, and visibility scoring behind the mosaic planner and target search. |
| `internal/report` | JSON + markdown run reports. |

## Colour palettes (deep-sky finish)

The deep-sky GIMP finish resolves a user-selectable **colour palette** — the channel→RGB mapping — in
`internal/pipeline/palette.go` (`resolvePalette`), consumed by `prepGimpInputs`:

| Palette | R | G | B | Notes |
|---|---|---|---|---|
| `natural` (default) | R | G | B | broadband LRGB + Hα screen + SPCC; `""` ≡ natural (byte-identical to the pre-palette pipeline) |
| `hargb` | R | G | B | natural with Hα mandatory |
| `hoo` | Hα | OIII | OIII | narrowband bicolour |
| `sho` (Hubble) | SII | Hα | OIII | narrowband |
| `hos` (CFHT) | Hα | OIII | SII | narrowband |
| `foraxx` (Webb-style) | Hα | √(Hα·OIII) | OIII | dynamic green via Siril pixel-math |
| `mono` | L → Hα → first | — | — | single-channel |

### The emission screens (natural family)

Narrowband shot *alongside* LRGB is composited as additive **screen layers** over the broadband base
rather than replacing it, so the data lights the image up instead of sitting unused. All three run the
same machine — continuum-subtract (`excess = line − k·broadband`, so only true emission survives) →
RBF-flatten → autostretch to a dark background → wash-gate → Screen in GIMP — and are declared once as
a table (`internal/pipeline/emissionscreen.go`) rather than as three copies that drift:

| Layer | Continuum ref | Colour | Knob | Default |
|---|---|---|---|---|
| Hα 656 nm | R → L | pure red | `ha_screen` | 0.42 (on) |
| [OIII] 501 nm | G → B → L | teal (red killed) | `oiii_screen` | 0 (opt-in) |
| [SII] 672 nm | R → L | `sii_tint`: deep red *or* gold | `sii_screen` | 0 (opt-in) |

[SII] is the awkward one: at 672 nm it is *deeper* red than Hα, which sRGB cannot express — pure red is
already the end of the ramp — so screening it "more red" would merely brighten the Hα layer. `sii_tint`
picks how to tell them apart instead: `deep_red` (default) keeps a trace of blue for a crimson that
reads as natural, `gold` is the amber accent the Hubble palette established and is far easier to see.

Two knobs weigh the emission contribution as a whole rather than line by line:

| Knob | Range | Default | What it does |
|---|---|---|---|
| `nb_blend` | 0 – 1 | 1 (full) | one weight over **both** emission screens — the "how much narrowband" slider. Mixing a dual-band exposure into a broadband base at full strength drags the dual-band's noise in with its signal, so the contribution is a user choice. It scales the two together because moving them one at a time shifts the Hα/[OIII] colour balance as a side effect, which is what the per-line knobs above are for. 0 = the broadband base alone (the layers are dropped, not written inert) |
| `oiii_boost` | 1 – 1.6 | 1 (off) | soft-shoulder lift on the [OIII] layer: identity below the knee, `tanh` roll-off asymptoting to 0.98, so faint teal gets the full factor while bright cores never clip flat. 1.25 subtle / 1.35 marked / 1.6 over-cooked. Prefer it to raising `oiii_screen`, which lifts the rims along with the cores |

`oiii_boost` replaces a plain multiply, which failed instructively (runbook v8, the "fake cyan plate"):
pushing the layer linearly drove its cores to 1.0 where they went **flat** — every pixel in the core
equal to its neighbours and to white — and a flat core reads as a pasted plate of colour rather than as
light. Turning the boost on therefore *engages* the shoulder, so a core sitting at 1.0 comes down
(~0.93 at 1.1) even though the knob says "boost": that is the anti-clipping doing its job. Both knobs
act at composite time on layers the Tier-A checkpoint already holds (`<outDir>/linear/`), so re-mixing
after a run costs a re-finish and never a re-stack.

The [OIII] and [SII] screens default to **0**, and `nb_blend`/`oiii_boost` to their no-op values, so a
run that does not ask for them emits byte-identical GIMP script to before the knobs existed. A screen-only layer never constrains anything that reasons
about coverage (`paletteResolved.screenOnly`) — it fades where its nights didn't reach, so letting it
bound a multi-night mosaic crop would collapse the canvas to its own footprint.

The narrowband palettes assign emission lines straight to R/G/B, so they **disable the Hα screen and
SPCC** and stretch unlinked; natural/hargb keep the SPCC ladder. A palette missing its required filters
falls back down a chain to one that resolves (ultimately mono) and records a `run.json` note — so a run
can request SHO today and simply render natural until OIII/SII data exists. The palette is a **Tier-B**
knob (`palette` in the supervisor/agent whitelist), so it can be switched post-run from the stage
timeline or Refine as a cheap re-finish (rebuild the combine from the persisted channel masters, no
re-stack). A pure **star cluster** (OpenNGC globular/open) additionally gets a gentler natural-colour
finish profile (`applyClusterProfile`): full-opacity L luminance, low saturation, star-core
desaturation + chroma blur, so a dense field reads as natural white-ish stars, not solid colour discs.

## Run provenance & environment health

Two mechanisms make "which code produced this image, and could the tools actually run?" answerable
at a glance:

- **Build provenance** (`internal/buildinfo`): `Version`/`BuiltAt` are injected at build time via
  `-ldflags` (git describe + timestamp; un-stamped `go run`/test binaries read "dev"). The identity
  is exposed at **`/api/health`** (`engine.version` / `engine.built_at`) and stamped into **every
  `run.json`** (`Result.Engine`) — so a result produced by a stale Docker engine is identifiable
  instead of masquerading as current code. Rebuild the container engine with `just stack-build`
  after pulling changes.
- **Environment health** (`internal/toolhealth`, served at **`/api/environment`**): *deep* probes,
  not mere binary lookups — Siril version, GIMP binary, StarNet binary, the raw developer kind
  (`dcraw_emu` vs the `sips` tone-curve fallback), the LLM server, the offline plate-solve
  catalogue situation (local Gaia astrometric file + xp_sampled chunk count → the effective
  `-catalog` value), and the **GraXpert deep probe** — a real tiny extraction run in the
  background, so a present-but-broken install (typically a missing ONNX runtime;
  fix: `pipx inject graxpert onnxruntime`) reads as broken instead of silently degrading
  gradients. The report is cached ~5 minutes and each problem carries a human-readable,
  run-impacting warning the UI can show before a run.

## Why no Redis / Celery

Jobs are persisted in Postgres and executed by a Go in-process worker pool. Siril emits progress on
stdout, which the runner parses and republishes to the browser over Server-Sent Events. A single host
binary keeps the moving parts minimal.

## Observability & resource metrics

A deep-sky run walks a **named step plan** (`internal/pipeline/progress_steps.go`): masters, each
channel, then the preset-derived finish steps (align → combine+background → optional AI colour
denoise → colour calibration+stretch → GIMP composite → optional StarNet/star-fix → export). Every
step boundary emits `▶ <step>` / `✓ <step> done in <dur> — peak tool RSS <n>` journal lines, warnings
are `⚠`-prefixed and surface live the moment they happen (`warnLive`), a failed job publishes a final
`✗` line, and only these markers (plus one `[i/N] <step>` skeleton per step) are mirrored to the
engine's stdout so `docker logs` stays readable. When the stream goes quiet (the CPU-only AI denoise
can be silent for an hour) a per-job **heartbeat** (`internal/job/heartbeat.go`) publishes
`still running: <step> — 14m into this step, no output for 90s · cpu 10.8/12 cores · rss 6.7 GiB`
after 45 s of silence, then every 30 s (SSE + stdout only, never persisted). Resource numbers come
from a single refcounted **engine-wide sampler** (`internal/job/enginemon.go`): it samples this
process's whole subtree (Siril, GraXpert, StarNet, GIMP, ffmpeg are all children) at 1 Hz and
publishes each running job's live CPU/RSS + job-wide peak + host core count — the job header shows
`x.x / N cores` and stays live through pure-Go/GIMP phases. Known limit: a host-offloaded GraXpert
(`ASTRO_GRAXPERT_URL`) runs outside the subtree and is not counted. Per-step wall times land in
`run.json` (`timings`) plus one final `timing: … · total …` line.

## Deliberate deviation

Running the engine and Go tests on the host is an intentional exception to the house "everything in a
container" rule, forced by the host-Siril/host-GIMP dependency (and the optional GraXpert/StarNet++
CLIs, which run the same way). It is the fastest path for daily macOS dev and is documented in
`CLAUDE.md`.

## Filters: one canonical set, recorded three times

`internal/filters` is the single source of truth for filter names — the canonical set
(`L, R, G, B, Ha, OIII, SII`), the aliases capture programs spell them with (`s2`, `sulfur`, `O3`,
`h-alpha`, Johnson `V`→`G`), the display order and which of them are narrowband. `constants/filters.ts`
mirrors it on the frontend, pinned by a spec. Everything that enumerates or orders filters — ingest,
the stacker, the wash gates, chip colours — reads one of those two.

That consolidation is not cosmetic. The lists used to be copy-pasted into a dozen places and drifted:
two of them stopped at `Ha`, so a wheel slot holding `SII` could only ever be named `"S6"`.

## The colour model: one pipeline, not two

A capture is either **monochrome** (a filter wheel, stacked per filter and combined into LRGB) or
**one-shot colour** (every light carries all three primaries). The verdict is
`inspect.Inventory.ColorModel`, decided once while scanning the folder and read by both entry
points, so the CLI and the web UI cannot disagree — they used to, and a colour folder submitted from
the UI as "deepsky" lost every frame in silence.

**Detection.** `Frame.Bayer` (a `BAYERPAT` card) alone cannot answer this: a developed DSLR raw, a
debayered RGB FITS and a colour TIFF have no Bayer pattern and are still colour. `Frame.Channels`
carries the plane count beside it, filled from `NAXIS3` for FITS and from the container header for
TIFF/PNG/JPEG, which gives three distinguishable states — mono, CFA awaiting demosaic, and RGB.
The pre-existing spurious-`BAYERPAT` veto still runs first (older ASICAP captures stamp one even on
a mono camera), so wheel evidence always beats a header artefact. Colour is part of the set key, so
a folder holding two rigs cannot merge them into one stack.

**Processing.** A colour run is the ordinary per-channel pipeline with exactly one channel, named
`RGB` (`filters.Color`). That is what makes it inherit the calibration library, frame grading, trail
masking, set QA, plate-solving, SPCC, GraXpert, StarNet, denoise, star-quality auto-fix, the
supervised finish, stage previews and per-stage rerun, instead of a parallel
implementation drifting away from them. The seams are in `internal/pipeline/color.go`; see
[modes/README.md](modes/README.md#monochrome-or-colour--the-same-modes) for what differs.

**The user can overrule the verdict** — `color_model` on the run request, `auto | mono | osc`
(`inspect.ParseColorChoice`, resolved by `inspect.ResolveColorModel`). Detection is inferred from
headers and pixels, which is right almost always and *unrecoverable* when it is wrong: a header-less
camera, a folder holding two rigs, a capture program stamping `BAYERPAT` on a mono sensor. `auto`
(and an absent field) is a deliberate no-op, so every stored job and every client that never sends
the field keeps the exact routing it had before the knob existed — each mode entry still owns its own
reading of a mixed folder, and those readings differ (deepsky drops the colour lights and warns,
mosaic does the same for anything not pure OSC, comet keeps every light, the per-stage rerun drops
silently to mirror the run it replays). An explicit choice narrows the **lights** to the ones that
match, rewrites the verdict so no entry ever sees `mixed`, and fails loudly when none match rather
than stacking the wrong half of the folder.

Only lights are filtered, never calibration frames. The matcher keys darks/flats/bias on
gain/offset/bin, exposure and temperature — guarded by `calib.KeepMatchingDims`' sensor-dimension
check — and `clearSpuriousBayer` deliberately strips `BAYERPAT` from calibration frames whenever the
scan shows wheel evidence. "Drop every monochrome frame" would therefore delete a colour rig's own
darks, silently, in exactly the mixed folder where the user reached for the knob.

**Calibration order is load-bearing.** A raw CFA mosaic is calibrated CFA-aware and demosaiced
*last* (`calibrate … -cfa -equalize_cfa -debayer`). Demosaicing first would interpolate every hot
pixel and dust shadow across its neighbours, so the defect map and the flat would be correcting a
smeared copy of the artefact rather than the artefact. Camera raws and colour stills are brought in
with Siril `convert` rather than `link`, which only symlinks FITS; monochrome runs still link, so
their scripts are byte-identical to before colour existed.

**The filter is recorded three independent times.** Well-behaved capture software writes the
filter into the folder, the file name and the FITS header, and ingest reads all three
(alias-aware, so a frame labelled `S2` reads as `SII`):

```
<root>/[panel/]<Filter>/Light_300sec_Bin1_filter-SII_-15.0C_gain200_2026-07-29_221403_frame0001.fit
                ^ folder                  ^ file name              plus FILTER = 'SII' in the header
```

Calibration follows the layout ingest already parses: `flats/<Filter>/` (flats are per-filter), and
`darks/` `bias/` `darkflats/` with no filter segment (those group filter-agnostically). Redundancy is
the point — a header stripped by a converter, or a file renamed by hand, still leaves the folder
saying which filter these frames were shot through.

## The star catalogues: two catalogues, one query

Two features need a star catalogue: **nightpano**'s astrometric anchoring (matching detected stars
against the sky to fit the shared lens) and the mosaic planner's rendered **star field**
(`GET /api/sky/starfield`). The two catalogues the engine ships are **not alternatives** — one is a
floor, the other raises it.

| | Embedded (`internal/deepstars/catalogue/hyg_mag9.csv.gz`) | Deep (`<library>/catalogues/athyg_v32.bin`) |
|---|---|---|
| Source | HYG v4.1 | **ATHYG v3.2** = Tycho-2 + Gaia DR3 + HYG's names |
| Stars | 83 479, to magnitude 9 | **2 552 164**, to about magnitude 13 |
| Size / where | 1.4 MB, `go:embed`ed, always present | ~130 MB, downloaded, gitignored, never committed |
| Extra fields | — | distance, spectral type, B−V, absolute magnitude, radial velocity |
| Installed by | nothing — it is compiled in | `just download-deepstars` |

`deepstars.Load(path)` returns the deep catalogue when the file is there and the embedded one when it
is not, so **a missing download means a shallower catalogue, never a broken feature** — CI, a fresh
clone and an offline machine all keep working unchanged (nightpano warns and degrades when the deep
catalogue is absent).

**Why it is a custom binary and not the CSV.** ATHYG ships as two ~99 MB gzipped CSVs; parsing those
into 2.5 million Go structs is ~600 MB resident, which the engine cannot spend beside Siril. So
`deepstars.Build` converts them once into a **declination-sorted, fixed-width record file** (52 bytes
per star, `format.go` owns the layout). A cone query then binary-searches the dec band with `ReadAt`
and streams only that slab: an M42-sized field costs a few hundred KB of reads and **~9 ms**, and
nothing but the small string tables (proper names, spectral types, constellations — interned, so a
record carries a 2-byte index) is ever resident. Declination is the sort key precisely because,
unlike RA, it has no wrap-around, so a band is always one contiguous range.

Two traps the builder is armoured against, both found the hard way:

- **RA is in HOURS** in ATHYG, as in HYG — ×15 to get degrees.
- **The second file has no header row.** The release splits one CSV by byte count, not by document,
  so a header-expecting parser silently eats its first star. The builder applies a hard-coded column
  order and *verifies it* against the first file's real header, so an upstream schema change fails
  the build instead of shifting every field.

Fields where zero is itself a measurement (B−V = 0 is a real A0 star; a star really can have zero
radial velocity) carry an explicit absent-sentinel rather than being encoded as 0, and a "distance"
past 100 kpc — what a negative parallax produces — is dropped, because a wrong number is worse than
a blank one. Licence and attribution: `docs/third-party.md`.

## Fully containerized mode (`stack`)

For portability and Linux-server deploys, the same code also runs **entirely in Docker** — because the
tool paths are all env vars, no Go changes are needed, only Linux builds of the tools baked into an
`engine` image. One `compose.yaml` serves both modes via profiles:

- `just stack` → `db` + **`engine`** (Go `serve` + Linux **Siril 1.4.x AppImage / GIMP 2.10 /
  GraXpert / ffmpeg** baked in, `docker/engine.Dockerfile`) + `frontend`. The engine reaches Postgres
  at `db:5432` and self-migrates. The engine persists **absolute** filesystem paths (in Postgres +
  `run.json`), so the stack bind-mounts `input/`, `library/`, `output/` and `work/` (all read-write) at their
  **same absolute host paths** and runs with the repo root as CWD (`working_dir: ${PWD}`) — pre-existing
  Library/Runs/Tasks/reuse rows resolve unchanged and host-dev ⇄ stack stay interchangeable. nginx
  templates its `/api` upstream (`API_UPSTREAM=engine:8080`).
- The finish-supervisor **VLM is decoupled and opt-in** — `stack` never pulls it. On **Linux+GPU** the
  `ai` profile (Ollama, `nvidia-container-toolkit`) serves it in-container (`ASTRO_LLM_URL=http://ai:11434/v1`);
  on **macOS** Docker has no Metal, so the model stays native (`just run-ia-model`) and the container
  points back at the host. The engine talks a stable OpenAI-compatible contract either way.

Trade-offs: the engine image builds for the **host architecture** (arm64 on Apple Silicon, amd64 on
Linux), so Siril/GIMP run **natively — no emulation**. Siril has no arm64 build, so the Dockerfile
branches on `TARGETARCH`: amd64 gets the pinned **1.4.3 x86_64 AppImage** (extracted from its squashfs
without executing the AppImage runtime), arm64 gets **1.4.x from the maintainer PPA**
(`ppa:lock042/siril`, overridable with `--build-arg SIRIL_PPA=`). The 1.4 floor is not optional: the
pipeline emits 1.4 script syntax and `internal/siril/runner.go` refuses to start against anything
older, so Ubuntu's own `universe` package (1.2.1) would fail every run. A native build now VERIFIES
this — `siril-cli --version` must succeed when the image and build architectures match, and the build
fails naming the PPA if it does not; only a genuine cross-arch build keeps the old warning, since
there the binary cannot be executed at all. The patch level can still differ from the pinned 1.4.3,
and the WCS/parity logic in `reuse_process.go` was written against 1.4.3, so prefer a native amd64
host (or host-dev on macOS) when exact multi-session parity matters. The arm64 package also ships its
deep-sky **object catalogue in a legacy
semicolon `.txt` format** (RA in hours, split N/S sign column) the engine's CSV parser can't read, so the
target search and the name→coord resolver fall back to a **catalogue snapshot compiled into the engine
binary** (`internal/skycat/catalogue/*.csv` via `go:embed`; `skycat.Load` prefers the on-disk Siril
catalogue and drops to the embed only when none is readable). Target search therefore works on every
arch regardless of the installed Siril, while the on-disk catalogue is still used wherever it *is* readable
(the macOS host + the amd64 AppImage, whose CSVs live in the `catalogue/` subdir `ASTRO_SIRIL_CATALOG_DIR`
points at). The **Siril SPCC sensor/filter database** is baked into the image at a pinned commit
(`SPCC_DB_REF` build arg → `/opt/siril-spcc-database`) and symlinked by the entrypoint into Siril's
user data dir (`$XDG_DATA_HOME/siril/siril-spcc-database`) — the GUI normally downloads it on first
use, which a headless container never does, and without it `spcc` aborts even on a plate-solved image
(the colour ladder then degrades to the star-field fallback). With the local Gaia catalogues under
`library/catalogues` (`just download-catalogues`) plate-solve + colour calibration run fully offline
in the container. Known issue: the **arm64 distro Siril 1.4.4 segfaults inside SPCC's aperture
photometry** (local and online catalogues alike); the engine's colour ladder falls to **PCC** on the
same solve (`internal/postprocess/colorcal.go`), which completes fine — so arm64 containers get a
photometric balance from Gaia photometry rather than per-star spectra until upstream fixes SPCC.
**StarNet++** is not baked in (licence not redistributable) and — unlike every other tool here — cannot
simply be mounted on Apple Silicon: upstream ships Linux x64, Windows x64 and both macOS builds, but no
**linux/arm64** one, and a macOS binary bind-mounted into the Linux image is a Mach-O that will never
exec. So there are two paths, chosen by architecture: on **linux/amd64**, mount your Linux x64 install
and set `STARNET_BIN`; on **linux/arm64**, run the host service (`just run-starnet-service`,
`cmd/starnet-host`) that `ASTRO_STARNET_URL` already points at under `just stack` — the same
offload shape as `ASTRO_GRAXPERT_URL`, except a resolvable local binary always wins over the URL (the
offload exists because no local binary can exist, not to reach a faster device). Without either it
soft-fails to full stars. The one thing that cannot run in a container on macOS is the **VLM** (no
GPU/Metal) — keep it native there.

### Which mode per environment

| Environment | Command | Engine + Siril/GIMP | AI model (VLM) | Use it for |
|---|---|---|---|---|
| **macOS — daily dev** | `just up` + `just dev` + `just web` | **native on host** (fast) | native mlx: `just run-ia-model` | everything — the normal workflow |
| **macOS — full container** | `just stack` | container (**native arm64**) — Siril/GIMP run natively (Siril 1.4.x from `ppa:lock042/siril`) | native mlx on host | a full local stack; use amd64 or host-dev for exact 1.4.3 patch parity |
| **Linux + NVIDIA GPU** | `just stack-ai` + `just ai-pull` | container (**native amd64**) — full processing | container (Ollama, GPU) | **true 100 % dockerized**, incl. the VLM |
| **Linux — no GPU** | `just stack` | container (native amd64) — full processing | skip, or point at any OpenAI-compatible server | headless processing without a GPU |

### Ports

| Port | Service | Mode |
|---|---|---|
| `5432` | Postgres | all |
| `8080` | engine API | host-dev (`just dev`) **or** container (`just stack`) — one at a time |
| `8082` | frontend (nginx) | `stack` / `web` |
| `11434` | Ollama VLM | `ai` (Linux + GPU) |
| `1234` | native mlx VLM | macOS host (`just run-ia-model`) |
| `8081` | Adminer | `tools` |
| `5173` | Vite dev server | host-dev (`just web`) |

### Stack configuration (`.env`)

Beyond the host-dev variables, the containerized stack reads (host tool paths like `SIRIL_BIN` are
**baked into the engine image** and don't apply here):

| Variable | Default | Description |
|---|---|---|
| `API_UPSTREAM` | `host.docker.internal:8080` | nginx `/api` target; `just stack` sets it to `engine:8080`. |
| `ENGINE_PORT` | `8080` | Host port the containerized engine's API is published on. |
| `UID` / `GID` | *(unset → 10001)* | Linux: run the engine as the UID/GID owning the data dirs (`id -u`/`id -g`). |
| `ASTRO_LLM_URL` / `ASTRO_LLM_MODEL` | host mlx / — | VLM endpoint + model id (see the table above). |
| `OLLAMA_TAG` / `OLLAMA_PORT` | `0.6.8` / `11434` | Ollama image tag (verify one for your driver) + port. |

**Your existing data & runs keep working.** The engine stores **absolute** paths in Postgres +
`run.json`, so the stack mounts your `./input`, `./library`, `./output`, `./work` at their **same
absolute host paths** and runs with the repo root as CWD. Previous Library masters, Runs, Tasks and
cross-session reuse resolve unchanged, and you can switch between host-dev and the stack freely.
Keep captures under `./input` (or symlink them there). `input` is mounted **read-write**: the pipeline
only ever reads it, but the removable-drive import copies capture folders into it,
and a read-only mount broke that.
