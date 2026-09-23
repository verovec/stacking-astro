# AstroStack task runner. Thin wrappers only.
# NOTE (host-engine exception, see CLAUDE.md): the Go engine, the Siril MCP and the Go tests
# run on the HOST (they drive host Siril/GIMP). Docker Compose runs Postgres (+ tools/web) only.

set dotenv-load := true
set shell := ["bash", "-uc"]
set positional-arguments := true

bin := "./bin"

# List available recipes.
default:
    @just --list

# First-run setup: .env, Go deps, dev tools, MCP binary, frontend deps. Idempotent.
setup:
    @test -f .env || cp .env.example .env
    go mod download
    @command -v golangci-lint >/dev/null || brew install golangci-lint
    @command -v air >/dev/null || go install github.com/air-verse/air@latest
    just build-mcp
    @test -d frontend/node_modules || (cd frontend && pnpm install)
    @echo "Setup done. Check your tools with 'just doctor', then: just up, and just dev + just web"
    @echo "(the engine migrates the database itself on boot — 'just migrate' is redundant)"

# Host mode; `just stack` prints the containerized answer by running this same command in the engine.
# Report which external tools this machine has, and what degrades without each one.
doctor:
    @go run ./cmd/astrostack doctor

# Start Postgres (compose).
up:
    docker compose up -d db

# Start Postgres + Adminer DB UI (http://localhost:${ADMINER_PORT:-8081}).
tools:
    docker compose --profile tools up -d

# Build + start the production frontend image (http://localhost:${WEB_PORT_PROD:-8082}).
web-prod:
    docker compose --profile web up -d --build frontend

# --- Full-container stack (everything in Docker; portable to a Linux server) -------------------------
# The AI model is decoupled and opt-in: `stack` never pulls or runs it. Bring it up later with `ai-up`
# (container, Linux+GPU) or `run-ia-model` (native mlx, macOS).

# Build the engine + frontend images (no model — Ollama is a pulled image, not built).
stack-build:
    GIT_DESCRIBE=$(git describe --tags --always --dirty) BUILD_TIME=$(date -u +%Y-%m-%dT%H:%MZ) docker compose --profile stack build

# Safe to re-run: the preflight is idempotent and reports only what it changed. The first run
# builds a multi-GB image (Siril/GIMP/GraXpert/ffmpeg) and takes 15-40 minutes.
# Set up + build + run the whole app in containers (db + engine + frontend, no AI), then print the URL.
stack:
    @scripts/stack-preflight.sh
    GIT_DESCRIBE=$(git describe --tags --always --dirty) BUILD_TIME=$(date -u +%Y-%m-%dT%H:%MZ) API_UPSTREAM=engine:8080 docker compose --profile stack up -d --build
    @scripts/stack-ready.sh

# Run the whole app in containers WITH the model (Linux+GPU; needs nvidia-container-toolkit). Then: just ai-pull
stack-ai:
    @scripts/stack-preflight.sh
    API_UPSTREAM=engine:8080 docker compose --profile stack --profile ai up -d --build
    @scripts/stack-ready.sh

# Stop the containerized app services (engine + frontend + ai); leaves Postgres running.
stack-down:
    docker compose --profile stack --profile ai stop engine frontend ai

# Tail the containerized engine logs.
stack-logs:
    docker compose --profile stack logs -f engine

# Open a shell inside the running engine container.
engine-sh:
    docker compose --profile stack exec engine bash

# Start ONLY the Ollama model container, decoupled from the stack (Linux+GPU). Then run `just ai-pull`.
ai-up:
    docker compose --profile ai up -d ai

# Pull the vision-model weights into the running ai container (explicit, ~heavy). Uses ASTRO_LLM_MODEL.
ai-pull:
    docker compose --profile ai exec ai ollama pull "${ASTRO_LLM_MODEL:-qwen2.5vl:32b}"

# Stop the model container.
ai-down:
    docker compose --profile ai stop ai

# Stop the compose stack.
down:
    docker compose down

# Apply database migrations (host engine -> compose Postgres).
migrate:
    go run ./cmd/astrostack migrate up

# Roll back the last migration.
migrate-down:
    go run ./cmd/astrostack migrate down

# Run the API server on the host with hot reload (drives host Siril/GIMP).
dev:
    air

# Run the frontend dev server on the host (Vite).
web:
    cd frontend && pnpm dev

# Serve the local vision model for the finish supervisor (host; first run downloads ~28 GB).
run-ia-model:
    @scripts/ia-model.sh

# Stop a backgrounded model server (run-ia-model is normally foreground; Ctrl-C there).
stop-ia-model:
    @pkill -f mlx_vlm.server || true

# Health-check the local model server.
ia-model-status:
    @curl -fsS "http://127.0.0.1:${ASTRO_LLM_PORT:-1234}/health" && echo " OK"

# Serve the host GraXpert on demand so the containerized engine can offload denoise / background
# extraction to a NATIVE process (Docker on macOS can't reach the host binary or the GPU). Foreground;
# then set ASTRO_GRAXPERT_URL=http://host.docker.internal:8083 for `just stack` and restart the engine.
run-graxpert-service:
    go run ./cmd/graxpert-host

# Health-check the host GraXpert service.
graxpert-service-status:
    @curl -fsS "http://127.0.0.1:${ASTRO_GRAXPERT_PORT:-8083}/health" && echo " OK"

# Serve the host StarNet on demand. A containerized engine cannot exec StarNet at all (no linux/arm64
# build exists, and a macOS binary can't run in the Linux image), so without this `just stack` silently
# keeps full stars. Foreground; the `stack` engine already points ASTRO_STARNET_URL here by default.
run-starnet-service:
    go run ./cmd/starnet-host

# Health-check the host StarNet service.
starnet-service-status:
    @curl -fsS "http://127.0.0.1:${ASTRO_STARNET_PORT:-8085}/health" && echo " OK"

# Inspect a capture directory and print the inventory (host).
inspect DIR:
    go run ./cmd/astrostack inspect "{{DIR}}"

# Rebuild the embedded star catalogue (internal/deepstars/catalogue/hyg_mag9.csv.gz) behind the
# mosaic planner's star field and nightpano's astrometric anchoring. Fetches the HYG database
# (network at generation time ONLY). MAG = faintest star kept.
# Re-run only to refresh or change depth; commit the regenerated file.
gen-deepstars-data MAG="9.0":
    go run ./cmd/astrostack deepstars-data --mag "{{MAG}}"

# One-time download of the DEEP star catalogue (ATHYG v3.2 — Tycho-2 + Gaia DR3 + HYG names; ~200 MB
# of CSV → ~130 MB .bin, ~15 s to convert) into library/catalogues. The embedded extract stops at
# magnitude 9, so a typical eleventh-magnitude detection is anonymous and has no distance; ATHYG
# names it, gives it a spectral type, and gives the parallax the 3D field map places it with. It also
# feeds the plate-solve check: on sparse fields the magnitude-9 set left only 2-5 usable check stars
# and the solve failed validation outright. Optional — absent, everything falls back to the embedded
# extract. MAG drops stars fainter than the limit (0 = keep all).
download-deepstars MAG="0":
    go run ./cmd/astrostack deepstars-athyg --mag "{{MAG}}"

# One-time download of Siril's OFFLINE Gaia plate-solve catalogue (~1.1 GB → ~3 GB) into
# library/catalogues — makes plate-solving (and therefore SPCC colour calibration) work with no
# network, on the host AND in the Docker engine (same files via the /data/library volume).
download-catalogues:
    @scripts/download-catalogues.sh

# Also fetch the offline SPCC xp_sampled chunks (~5 GB, 48 files). Optional: without them SPCC
# falls back to the online Gaia archive (needs network at run time).
download-catalogues-spcc:
    @scripts/download-catalogues.sh --spcc

# Run the full auto pipeline (host). MODE: deepsky|nebula|milkyway|nightpano|planetary|comet|mosaic|sun|eclipse  FORMAT: image|video|both
# e.g. just process deepsky image ~/Astro/M31   ·   just process planetary video ~/Astro/moon.mp4
process MODE FORMAT PATH *args:
    go run ./cmd/astrostack process {{args}} {{MODE}} {{FORMAT}} "{{PATH}}"

# Re-run the finish via the local AI agent on an existing run dir — no re-stack. e.g. just refine output/M101/<runID>
refine RUNDIR *args:
    go run ./cmd/astrostack refine {{args}} "{{RUNDIR}}"

# Process a lunar/planetary video (host).
video FILE *args:
    go run ./cmd/astrostack video {{args}} "{{FILE}}"

# Record a demo video of the web UI from a YAML scenario (host; needs the app running — see just dev/web).
# Generate a scenario with the /demo-video Claude command. e.g. just demo overview · just demo tour --headless
demo scenario="tour" *args:
    @scripts/demo.sh {{scenario}} {{args}}

# Regenerate the in-app help-tour screenshots into frontend/public/tour/ (host; needs the app running).
# Re-run whenever the UI changes, then commit frontend/public/tour/. e.g. just tour-shots --locales en
tour-shots *args:
    @scripts/tour-shots.sh {{args}}

# Run the Siril MCP server in the foreground (manual testing).
mcp-siril:
    go run ./cmd/siril-mcp

# Build the Siril MCP binary into ./bin (used by .mcp.json).
build-mcp:
    @mkdir -p {{bin}}
    go build -o {{bin}}/siril-mcp ./cmd/siril-mcp

# Build the host GraXpert service binary into ./bin.
build-graxpert-host:
    @mkdir -p {{bin}}
    go build -o {{bin}}/graxpert-host ./cmd/graxpert-host

# Build the host StarNet service binary into ./bin.
build-starnet-host:
    @mkdir -p {{bin}}
    go build -o {{bin}}/starnet-host ./cmd/starnet-host

# Build all binaries + the frontend (build identity stamped via ldflags — shows in /api/health,
# every run record, and the UI's engine chip, so a stale-engine run is identifiable at a glance).
build:
    @mkdir -p {{bin}}
    go build -ldflags "-X github.com/verove-jordan/astronomy/internal/buildinfo.Version=$(git describe --tags --always --dirty) -X github.com/verove-jordan/astronomy/internal/buildinfo.BuiltAt=$(date -u +%Y-%m-%dT%H:%MZ)" -o {{bin}}/astrostack ./cmd/astrostack
    go build -o {{bin}}/siril-mcp ./cmd/siril-mcp
    @test -d frontend/node_modules && (cd frontend && pnpm build) || true

# Run the Go tests (on host; they exercise host Siril). Pass-through args: just test ./internal/grade
test *args:
    go test {{ if args == "" { "./..." } else { args } }}

# Lint + format-check + type-check (read-only; fails on issues).
lint:
    go vet ./...
    golangci-lint run
    @test -d frontend/node_modules && (cd frontend && pnpm vue-tsc --noEmit) || true

# Auto-format the codebase in place.
fmt:
    gofmt -w .
    @test -d frontend/node_modules && (cd frontend && pnpm format) || true

# The pre-push gate: lint + test.
check: lint test

# Refresh the gitnexus code-graph (incremental). Run before impact/context queries on fresh edits.
gitnexus-sync:
    @.claude/hooks/gitnexus-sync.sh

# Force a full gitnexus re-index (after big refactors or a branch switch).
gitnexus-reindex:
    @.claude/hooks/gitnexus-sync.sh --force

# Show the gitnexus index status for this repo.
gitnexus-status:
    @gitnexus status

# Tail compose logs.
logs:
    docker compose logs -f

# Open a psql shell in the db container.
sh:
    docker compose exec db psql -U "${POSTGRES_USER:-astro}" -d "${POSTGRES_DB:-astrostack}"

# Remove containers, volumes, and build artifacts. DESTRUCTIVE.
[confirm("This drops the Postgres volume and build artifacts. Continue?")]
clean:
    docker compose down -v
    rm -rf {{bin}} tmp frontend/dist
