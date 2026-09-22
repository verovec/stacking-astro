# The web UI

A Vue 3 single-page app (left rail navigation) with two areas: the **Processing hub** and the
**Mosaic planner**. Routes live in `frontend/src/router/index.ts`.

## Processing hub (`/processing`) — four tabs

### Import (`/processing/import`)

The main page: browse the data dir (plus removable drives), **multi-select capture
folders**, and *Inspect* them into one merged inventory — stat cards per frame type, channel
mapping (overridable when detection confidence is low), light/calibration/file tables, and
warnings. Then configure the run:

- **What did you shoot?** — optional object-type chips (galaxy, emission/planetary/reflection/dark
  nebula, supernova remnant, oxygen cloud, star cluster, comet, moon, planet, sun, Milky Way). They
  **re-order** the preset list — recipes suited to that target first, everything else under "other" —
  and never hide anything, including your own presets. The taxonomy and its order are served by the
  engine (`GET /api/presets` → `object_types`); the UI declares none of its own.
- **Preset** — a catalog of 29 built-in "best params per situation" recipes (galaxy, faint galaxy,
  star cluster, reflection/emission/planetary nebula, dark nebula, supernova remnant, oxygen cloud,
  thin one-shot-colour broadband, camera-lens wide field, three stacking variants, SHO/HOO/Foraxx
  narrowband, moon, planet, comet, five sun recipes, three milkyway looks) plus your own saved
  presets (Save… persists to the DB). A preset prefills mode/format/palette/toggles/advanced params;
  everything stays editable.
- **Mode · Output** — the processing modes; image/video/both.
- **Toggles + Advanced parameters** — SPCC, denoise, Ha handling, wheel-transition drop, palette,
  and the per-mode advanced knob editor; *Run with local AI agent* opts into the
  [finish supervisor](agent.md).
- **Cross-session reuse** — prior sessions of the same target are auto-discovered and folded in
  (deselectable per session).
- **Calibration preview** — which library masters would match each light set, before launching.

**Run pipeline** starts immediately; **Add to queue** appends to a strictly-sequential lane. The
*Processing history* panel below lists every past job with a one-click **Use again**.

### Tasks (`/processing/tasks`)

The job list (running + history) and the **job detail** view:

- While running: live progress over SSE — progress bar, current step, live CPU/RAM of the running
  tool, a rolling log, live preview, and the milestone preview timeline as it grows. **Pause**
  parks a resumable checkpoint (mid-stack pause keeps the finished channels). **Cancel** stops the
  run — a cancel during finishing keeps the partial result and marks the job *cancelled*.
- When finished: the full result panels — final image/video, per-channel stats (frames in/stacked,
  matched Dark/Flat/Bias, and the **Pointing** column with the dither/drift verdict), per-frame
  grade charts, masters used, calibration notes, warnings, run options (provenance), download
  links, and for supervised runs the **supervisor panel** (one card per iteration, scores +
  reasoning + the chosen best). Finished pages read the stored result directly (no event stream).
- **Stage timeline & re-run** — each processing milestone is a card; edit a stage's parameters and
  **re-run from that stage** (cheap tiered re-entry, deep-sky). **Refine** re-runs only the finish
  through the supervisor; **Retry tuned** re-processes with adjusted params.

### Runs (`/processing/runs`)

A gallery of on-disk runs (independent of the DB — anything with a `run.json`), re-rendering the
same result panels.

### Library (`/processing/library`)

The calibration-master library: darks/flats/bias/dark-flats and phone masters with their keys
(gain/offset/bin/exposure/temp) and frame counts (see [calibration.md](calibration.md)).

## Mosaic (`/mosaic`)

Plan a tiled panel grid for a large object before shooting it. Search the object by name
(`GET /api/sky/search`, served from the local catalogues), preview the field over a rendered
star chart (`GET /api/sky/starfield`, with an optional DSS2 sky-survey view), and let the planner
compute the tile grid from your saved equipment setup — panel count, overlap, camera angle. Plans
persist server-side (`mosaic_plans`) and a mosaic processing run can reference one
(`mosaic_plan_id`). See [modes/mosaic.md](modes/mosaic.md).

## In-app help

Every page carries a discreet **help** button beside its heading. It opens a guided tour of that
page: a carousel of real screenshots on the left, an explanation on the right, arrow keys or the
chevrons to step, Esc to close. It **never starts on its own** — it is there when someone wants it
and invisible otherwise.

The tour deliberately shows *pictures* of the page rather than spotlighting live elements. A tour
that highlights real controls can only describe the page as it currently is — nothing selected, a
panel collapsed, a list still loading, no job run yet — and the steps a newcomer most needs are
exactly the ones whose elements are not on screen.

- Registry: `frontend/src/constants/tour.ts`, keyed on the **route name**, holding only step keys.
- Copy: the `tour` namespace in `frontend/src/i18n/{en,fr}.json`. `tour.spec.ts` fails the build if
  a locale is missing a string, if a tour names a route that no longer exists, or if a new named
  route ships with no tour at all.
- Screenshots: `frontend/public/tour/<locale>/<page>-<step>.webp`, regenerated by **`just tour-shots`**
  against a running app (see `tools/demo/scenarios/tour-shots.yaml`). The focus highlight is baked
  into the image, so the modal never needs to know where a control is. A step with no shot yet
  degrades to its caption, so the set can be filled in gradually — but re-run the recipe when the UI
  changes, or the pictures quietly go stale.

## Conventions the UI follows

- All state that matters is server-side; the browser keeps only preferences/favorites.
- Every long operation is a job with SSE progress; nothing blocks the page.
