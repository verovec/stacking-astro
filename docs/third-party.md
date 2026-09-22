# Third-party software, data and services

Everything AstroStack uses that it did not write. The engine is a thin orchestrator: almost all of the
astronomy — the stacking, the catalogues — comes from someone else's work, and this page is the record
of whose.

AstroStack itself is **MIT** (`LICENSE`). That covers only the code in this repository. Nothing here is
vendored into the binary except the small embedded data files listed under
[Embedded data](#embedded-data-compiled-into-the-binary); every tool is **invoked**, never bundled, and
every downloaded catalogue is fetched at runtime under its own terms.

**One obligation to keep in mind if you ever redistribute or commercialise this:**

- **Several catalogues are CC BY-SA**, which is a share-alike licence on the *data*, not the code:
  HYG, ATHYG and OpenNGC. Keep their attribution with any redistributed copy.

Nothing here is a hard dependency in the sense of blocking a run: every optional host tool
falls back to the Siril/GIMP path, and a missing catalogue download degrades to an embedded
extract or a documented default. See [architecture.md](architecture.md).

---

## Host software the engine drives

Invoked as external processes (`os/exec`) or over a local socket. None is redistributed; you install
them yourself, under their own licence.

| Tool | Role | Licence | Config |
|---|---|---|---|
| **Siril** 1.4.x | The stacking engine: calibration, registration, stacking, plate-solving, SPCC. Driven with generated `.ssf` scripts. | GPL-3.0 | `SIRIL_BIN` |
| **GIMP** 2.10 | The finish compositor, driven through the vendored MCP or `gimp-console` batch. | GPL-3.0 | `GIMP_BIN`, `GIMP_HOST`/`GIMP_PORT` |
| **ffmpeg** / **ffprobe** | Video frame extraction (planetary/lunar/solar) and video output. `ffprobe` detects >8-bit sources so extraction keeps 16 bits. | LGPL/GPL depending on build | `FFMPEG_BIN`, `FFPROBE_BIN` |
| **GraXpert** | *Optional.* AI background-gradient extraction and denoise. Absent → Siril `subsky`. | see upstream (graxpert.com) | `GRAXPERT_BIN`, `GRAXPERT_URL` |
| **StarNet++ v2** | *Optional.* Star removal for star-reduced finishing. Absent → full stars. **Licence is not redistributable**, which is why the Docker engine image does not bake it in. | see upstream (starnetastro.com) | `STARNET_BIN` |
| **LibRaw** (`dcraw_emu`) | Preferred camera-raw → 16-bit TIFF developer (photometric, exact sRGB). macOS `sips` is the fallback. | LGPL-2.1 / CDDL | (PATH) |
| **`sirilpy`** (+ a Python 3.12 venv) | Siril's *own* scripting module, which Siril requires before it will plate-solve or run SPCC. Not something AstroStack calls — but if it is missing, Siril fails with "Python version check failed" and colour calibration silently degrades. Siril installs it under `<work>/.local/share/siril/.python_module`; a hand-built venv may be needed on macOS. | GPL (with Siril) | — |
| **PostgreSQL** 16 | The only stateful service. Imaging data only — no sky datum is ever persisted. | PostgreSQL License | `DATABASE_URL` |
| **Docker / Compose** | Runs Postgres, the frontend image, and the full-container `stack` profile. | Apache-2.0 | — |
| **Go** 1.23.3 (pinned, `GOTOOLCHAIN=local`) · **Node 22** / **pnpm** | Build toolchains. | BSD-3-Clause · MIT | — |

**Vendored, not authored here:** `mcp-servers/gimp/server.py` — the GIMP MCP server. It is the one
piece of Python in the repository and is copied as-is, because GIMP's own scripting is Python/Scheme.
Do not add new Python; see `CLAUDE.md`.

**Container base images** (`compose.yaml`, `docker/*.Dockerfile`): `postgres:16-alpine`,
`adminer:4`, `node:22-alpine`, `nginx:1.27-alpine`, `golang:1.23-bookworm`, `ubuntu:24.04`, and
`ollama/ollama` for the optional Linux+GPU model profile. The engine image additionally installs
Siril (x86_64 AppImage from free-astro.org, or the `ppa:lock042/siril` build on arm64), GIMP, ffmpeg
and GraXpert.

### Optional local AI

The finish supervisor drives a **host-run, OpenAI-compatible** model server —
it never calls a hosted API and no image leaves the machine. Default on macOS is
`mlx-community/Qwen2.5-VL-32B-Instruct-6bit` served by **mlx-vlm** (`just run-ia-model`, ~26 GB on
first download); on Linux+GPU the `ai` Compose profile serves it through **Ollama**. LM Studio is a
drop-in alternative. An empty or unreachable `ASTRO_LLM_URL` means the normal single-pass finish runs
— the feature is opt-in and soft-fails.

---

## Astronomical catalogues and data

All fetched on demand or downloaded once, all soft-failing to an embedded extract or a fallback.

| Source | Used for | Licence |
|---|---|---|
| **Gaia DR3** (Siril's extracts, Zenodo records `14692304` / `14738271`) | Offline plate-solving (~1.1 GB → ~3 GB) and optionally SPCC colour calibration (~5 GB). `just download-catalogues[-spcc]`. | ESA/Gaia/DPAC — attribution required |
| **Siril SPCC database** `gitlab.com/free-astro/siril-spcc-database` | Sensor/filter spectral responses for colour calibration. Baked into the engine image. | GPL (with Siril) |
| **HYG Database v4.1** (David Nash / astronexus.com) | The embedded mag ≤ 9 star extract behind the mosaic planner's star field and nightpano's astrometric anchoring. | **CC BY-SA 4.0** |
| **ATHYG v3.2** (astronexus) | The deep star catalogue — ~2.5 M stars to about mag 13, with distance, spectral type, colour index, absolute magnitude and radial velocity. Downloaded and converted by `astrostack deepstars-athyg` into `<library>/catalogues/athyg_v32.bin` (~130 MB, **never committed**, beside the Gaia files); absent → the embedded mag ≤ 9 extract, which is shallower but never broken. | **CC BY-SA 4.0** |
| **OpenNGC** (Mattia Verga) | Morphological type, size, surface brightness and common names overlaid onto the NGC/IC records — what makes NGC 6946 read as "Fireworks Galaxy, galaxy" instead of "other" in target search, and what identifies a pure star cluster for the finish profile. | **CC BY-SA 4.0** |
| **JPL Solar System Dynamics — approximate positions of the major planets** (Standish) | The Keplerian element table behind `internal/astro`'s planetary positions: six elements and six rates per planet, fitted for 1800–2050, arcminute-class over that span. Compiled in (`internal/astro/heliocentric.go`), no download. | Public domain (NASA/JPL-Caltech) |
| **Messier / NGC / IC / Sharpless / LDN** (Siril's bundled catalogue) | Deep-sky coordinates, sizes and magnitudes for target search and name→coordinate resolution. | public-domain astronomical data, shipped with GPL Siril |
| **Aladin Lite v3 + CDS surveys** `aladin.cds.unistra.fr` | The optional sky-image viewer (DSS2 colour) in the mosaic planner. Loaded on demand from the CDS CDN. | CDS, Université de Strasbourg |

### Embedded data (compiled into the binary)

These are the only external data files inside the executable. All are small, all carry their
provenance and licence in a sibling `README.md`, and all are refreshable with a `just` recipe.

| File | Contents | Licence |
|---|---|---|
| `internal/deepstars/catalogue/hyg_mag9.csv.gz` (1.4 MB) | 83 479 stars at mag ≤ 9 | CC BY-SA 4.0 (HYG) |
| `internal/skycat/catalogue/*.csv` (~1 MB) | Messier, NGC, IC, Sh2, LDN + the OpenNGC type overlay | public domain + CC BY-SA 4.0 (OpenNGC) |

---

## Libraries

### Go

Direct dependencies only; the full transitive set with resolved licences is `go.mod` + `go.sum`. The
project deliberately runs a **small dependency surface** — a pinned Go 1.23.3 toolchain means new deps
must be old-compatible, so most astronomy is hand-rolled in `internal/astro` rather than pulled in.

| Module | Role | Licence |
|---|---|---|
| `github.com/jackc/pgx/v5` | PostgreSQL driver (raw pgx, no ORM) | MIT |
| `github.com/soniakeys/meeus/v3` + `/unit` | Eclipse-mode geometry (Besselian time scales, julian dates) | MIT |
| `github.com/klauspost/compress` | gzip HTTP middleware | Apache-2.0 |
| `golang.org/x/image` · `x/sync` · `x/sys` | TIFF codecs · `singleflight`/`errgroup` · syscalls | BSD-3-Clause |
| `github.com/stretchr/testify` | Test assertions | MIT |

Notably **not** a dependency: FITS I/O. `internal/fits` implements the header and pixel reader by
hand, as does `internal/astro` (positional astronomy) — no FITS library is linked in.

### Frontend

| Package | Role | Licence |
|---|---|---|
| `vue` · `vue-router` · `pinia` · `vue-i18n` | The app framework, routing, state, i18n (en + fr) | MIT |
| `echarts` + `vue-echarts` | Grade charts, timelines | Apache-2.0 |
| `markdown-it` | Renders supervised-conversation replies | MIT |
| `tailwindcss` · `postcss` · `autoprefixer` | Styling | MIT |
| `vite` · `typescript` · `vue-tsc` · `vitest` · `@vue/test-utils` · `happy-dom` · `@pinia/testing` · `prettier` | Build and test toolchain | MIT (TypeScript: Apache-2.0) |

---

## Status notes

Kept here so the table above never quietly drifts from reality:

- **The two star catalogues are not alternatives.** The embedded mag ≤ 9 HYG extract is compiled in
  and always present; ATHYG is an opt-in download that deepens it to ~mag 13. `deepstars.Load` falls
  back to the embedded set whenever the `.bin` is absent or unreadable, so a missing download means
  a shallower catalogue, never a broken feature.

## Keeping this current

Adding an external dependency means adding a row here. To re-derive the tables:

```sh
grep -rhoE 'https://[^"`) ]+' internal/ cmd/ scripts/ justfile | sed -E 's#(https://[^/]+).*#\1#' | sort -u
go list -m -f '{{.Path}} {{.Dir}}' all     # then read each module's LICENSE
python3 -c "import json;d=json.load(open('frontend/package.json'));print(*d['dependencies'])"
```

Per-source detail lives next to the code: `internal/skycat/catalogue/README.md` and
`internal/deepstars/catalogue/README.md`. Every environment variable is in
[configuration.md](configuration.md) and `.env.example`.
