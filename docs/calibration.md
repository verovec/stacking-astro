# Calibration: masters, the library, and defect maps

How AstroStack turns raw darks/flats/bias into the masters that calibrate every light — and why
noise keeps dropping as sessions accumulate. Code: `internal/calib` (building, matching, defect
scan), `internal/siril/scripts.go` (the generated Siril scripts).

## Master building

Each calibration *set* (same type + gain + offset + binning + exposure + temperature bucket) is
stacked into one master FITS:

- **darks / bias / dark-flats** — `convert <seq>` then `stack <seq> <rejection> -nonorm`
  (`StackMasterScript`). `convert` (not `link`) means a calibration set captured as **16-bit TIFF**
  (SharpCap lunar darks) stacks exactly like FITS.
- **flats** — optionally bias-calibrated first (`calibrate <seq> -bias=<master>` — a matching
  dark-flat wins over a bias), then `stack … -norm=mul` (multiplicative normalization, correct for
  flat fields). Every master flat also runs an **optical QC** pass (`internal/optics.AnalyzeFlat`):
  dust donuts, blotches, saturation and vignetting are reported as run warnings with a JSON/PNG
  sidecar.

Masters are named by everything that makes them reusable —
`master_<TYPE>[_<filter>][_<exposure>ms]_g<gain>o<offset>_b<bin>[_<temp>C].fits` — and saved to
the **library** (`ASTRO_LIBRARY_DIR`).

### Count-adaptive rejection

Every stack picks its pixel-rejection algorithm from the frame count by default
(`stackalg.AutoReject`), sized to THAT pool's own depth. Each frame type (bias/dark/flat/dark-flat)
carries its own recipe, overridable per run — see [stacking.md](stacking.md):

| Frames | Rejection | Why |
|---|---|---|
| ≤ 7 | `rej percentile 0.2 0.1` | σ estimates are meaningless on a handful of samples |
| 8 – 49 | `rej winsorized 3 3` | the proven default for medium stacks |
| ≥ 50 | `rej generalized 0.3 0.05` (GESD) | markedly better on outlier tails — kills the correlated leftovers (walking noise, trail remnants) a 3σ clip misses |

The light-frame stack sizes the choice to the **surviving** (graded-in) frame count. All
processing is pinned to 32-bit float (`set32bits`), so dark subtraction keeps negative pixels and
the rejection statistics stay unbiased.

## Deep cross-session pools

With the database on, every scan is catalogued, and `BuildDeepMasters` pools raw calibration
frames **across sessions** instead of freezing per-session masters:

- **bias** — sensor-only, pooled freely (no temperature or recency bound);
- **darks** — pooled per camera config + exposure, temperature within `ASTRO_REUSE_TEMP_TOL_C`
  (default ±5 °C) and inside the optional recency window (`ASTRO_REUSE_DARK_RECENCY_DAYS`);
- **flats / dark-flats** — deliberately **session-local** (dust and vignetting belong to one
  night's optical train).

Master noise adds to every calibrated light in quadrature and falls as 1/√N of the pool — so a
library that keeps accumulating darks and offsets directly lowers the noise floor of every future
run.

Pool hygiene and reuse:

- **Reuse signatures** — a master carries a `.sig` sidecar (sha256 of each pool frame's
  path|size|mtime). An unchanged pool reuses the on-disk master instead of re-stacking (minutes
  saved on large pools); writes are atomic (temp + rename) so concurrent runs sharing the library
  never see a half-written master.
- **`dropMissing`** — catalogued frames whose file no longer exists on disk are skipped with a
  counted warning (one ghost path would sink the whole Siril stack).
- **`dropNonFITS`** — anything that isn't a FITS file is excluded from a pool with a counted
  warning (a processed image that once slipped into the catalog must never be stacked as
  calibration).

## Dark defect map (bad-pixel map)

Classic cosmetic correction (`-cc=dark`) can only see pixels that are hot or cold **in the
master** — it is blind to *unstable* (random-telegraph / flickering) pixels whose master value
looks normal, and those are exactly the pixels that survive dark subtraction as random residuals
and smear into **walking noise** on drifting, undithered sequences.

When a master dark is built (and lazily for pre-existing library masters), the raw dark pool is
scanned per pixel (`calib.ScanDarkDefects`):

1. **Temporal statistics** — mean and sigma of each pixel across all darks in the pool.
2. **Local baseline** — both maps are compared to a 5×5 separable local median, so large-scale
   structure (amp glow, vignetting) never reads as defects.
3. **Flagging** — hot/cold: mean 3σ off its local baseline (robust MAD scale); **unstable/RTS**:
   temporal sigma 6σ above its local baseline.
4. **Safety** — needs ≥ 8 darks (temporal σ is untrustworthy below that); the list is capped at
   0.5 % of the sensor keeping the strongest detections, so a pathological scan stays harmless.

The result is written beside the master in Siril's `find_hot` format —
`library/master_DARK_…_defects.lst` (`P x y H|C`, orientation-aware). At calibration time the
matched dark's map is applied per frame as
**`calibrate … -cc=bpm <defects.lst>`**, replacing `-cc=dark`; without a map the classic
`-cc=dark` still runs. On the reference ASI1600MM Pro this finds ~1,500 genuinely unstable pixels
on top of the warm/cold population.

## Matching rules (which masters calibrate a light set)

`calib.MatchForLight`:

| Master | Rule |
|---|---|
| dark | same gain/offset/bin **and same exposure**, nearest temperature within ±5 °C; deepest pool wins ties |
| dark (no exact exposure) | a same-camera dark of a *different* exposure + a bias → Siril **dark optimization** (`-opt`) scales its thermal signal onto the lights |
| flat | same filter preferred; else *any* session flat (most dust sits on the sensor window, common to every filter) — noted in the run |
| flat (one-shot colour) | **same clip filter, required** when both the lights' night and the flat's are classified: a broadband flat is never a candidate for a dual-band night, and vice versa. Either side unclassified → the row above, unchanged |
| bias | same gain/offset/bin, deepest pool |
| bad-pixel map | the matched dark's `_defects.lst` sidecar when present |

Every choice, fallback and skip is recorded as a human-readable note in `run.json`
(`channels[].selection.notes`).

### The clip-filter rule, in full

A dual-band clip (Ha + OIII) passes two narrow windows; an unfiltered train passes the whole
continuum. Their illumination profiles have nothing in common, so dividing dual-band lights by a
broadband flat corrupts the colour response of every frame it touches. Calibration frames carry an
empty `FILTER` and one-shot-colour lights read `RGB`, so the filter-matched pass above *always*
missed for them and fell through to the filter-blind one — which handed over the wrong flat silently.

- **Where the verdict comes from.** The pixel classifier measures LIGHT sets only
  (`internal/inspect/filterset.go`); a flat is an evenly-lit panel with no sky to classify. A flat
  therefore inherits its **capture night**, via `Inventory.NightFilterSet`, and only when that
  night's light sets agree. A clip swapped mid-session leaves the night `unknown` rather than
  guessing which half a flat belongs to.
- **Darks, dark-flats and bias are never gated.** They are closed-shutter exposures: no light
  reaches the sensor, so nothing in the optical train can have touched them. Gating them would
  strand perfectly good masters.
- **Unknown means "as before".** If either side is unclassified — every monochrome capture, every
  colour night the pixels could not settle, every library master (`master_frames` has no filter-set
  column) — the pool is untouched and ranking is bit-for-bit what it was.
- **Prior sessions stay unknown.** The catalog never recorded a night's clip filter, so folded-in
  prior groups are not gated. Guessing from the current night would gate a flat on a filter nobody
  ever measured on it.
- **`force_calibration_frames` overrides it**, like every other gate — the flat is applied and the
  run says, in as many words, that these are *X* lights calibrated with a *Y* flat.
- **A refusal is never silent.** When the only flats on offer were cross-set, the run names both
  sets and says how many were excluded, so "no flat" can be told apart from "a flat I refused".

Before the run, the Import view's per-night panel shows the same thing: each light set's clip
filter, the night and clip filter of the flat it would get, and a **flat-fallback** chip whenever
that flat is refused, borrowed from another night, forced, or missing
(`RunPlanPreview` → `PlanGroup.filter_set` / `flat_fallback`).

**Colour (one-shot) lights match the same way**, on gain/offset/bin/exposure/temperature — a colour
sensor's darks and flats are no different in kind. Two things are specific to them:

- Colour is part of the set key, so a monochrome master can never be matched to a colour light set
  (or the reverse) even when every other field agrees.
- A raw CFA mosaic is calibrated **CFA-aware and demosaiced last**
  (`calibrate … -cfa -equalize_cfa -debayer`). `-cfa` keeps the cosmetic-defect and flat maths on
  the Bayer grid and `-equalize_cfa` balances the flat's own colour channels. Demosaicing before
  calibration would interpolate every hot pixel and dust shadow across its neighbours, so the defect
  map and the flat would be correcting a smeared copy of the artefact instead of the artefact.
- With **no masters at all** — a DSLR session shot without darks or flats — the demosaic pass still
  runs on its own. Skipping it leaves a green checkerboard all the way through the stack.

Camera raws (NEF/CR2/CR3/ARW/RAF/DNG) are decoded by Siril's own libraw during `convert`. When that
fails (Apple ProRAW is the known case) they are developed by LibRaw's `dcraw_emu` instead, in linear
light — see `internal/rawconv`.

## A clip filter is not a filter wheel

Both rigs write a `FILTER` card, and the card alone cannot tell them apart:

- an **ASI1600MM** (monochrome) behind a wheel, whose older ASICAP build stamps a `BAYERPAT` anyway.
  The pattern is meaningless and must be cleared, or the session routes down the colour path and
  loses every frame.
- an **ASI2600MC** (colour) behind a **dual-band clip filter**, whose capture program writes
  `FILTER='L-eXtreme'`. That name is not a wheel slot, the `BAYERPAT` is real, and clearing it
  destroys the session.

So the question is answered from the pixels (`internal/inspect/cfaprobe.go`). A genuine mosaic puts
its four Bayer sub-lattices under different dyes — skyglow and flat-field QE split them by tens of
percent — while the two greens, sharing a dye, stay together. A monochrome sensor is one response:
its sub-lattices differ only by noise, and a vignette or gradient moves all four at once instead of
separating R from B. As with filter sets, the sampled frames must AGREE or the probe declines.

Two consequences for `clearSpuriousBayer`:

- **It runs per instrument.** It used to be scan-wide, so one mono rig anywhere in a folder stripped
  the `BAYERPAT` from *every* calibration frame in it — including another camera's. A mixed intake
  silently lost its colour calibration.
- **A probed-CFA frame is never wheel evidence** and is never cleared, whatever its `FILTER` says.

A declined probe keeps the old mono-rig behaviour (every existing mono session depends on it) but
says so in a warning rather than acting in silence.

## Filter sets on a one-shot-colour rig

A colour camera has no filter wheel, so it writes no `FILTER` card: the same body shooting broadband
one night and through a dual-band Ha/OIII clip the next produces two sets of frames that are
indistinguishable by metadata. They are **not** interchangeable — a dual-band frame carries two
emission lines on a near-black sky, a broadband frame a full continuum — so stacking them together,
or dividing one by the other's flat, is wrong.

Inspect therefore measures it from the pixels, per light set
(`internal/inspect/filterset.go`), and records `filter_set` = `broadband | dualband | unknown`:

| signal | dual-band | broadband |
|---|---|---|
| sky above the bias pedestal, per 120 s | 2–6 ADU | 30–45 ADU |
| background colour | red-dominant (R > G > B) | green/blue-dominant |

**Both signals must agree, or the answer is `unknown`.** Amplitude alone is fooled by a bright sky or
a faster lens; colour alone by a red light-pollution gradient. An honest `unknown` costs nothing
because every consumer falls back to its pre-filter-set behaviour, whereas a wrong verdict silently
mis-stacks a whole night.

The **bias pedestal is required**, not optional. An uncalibrated frame sits at several hundred ADU of
pure electronic offset — twenty times the broadband sky — so with no bias or dark in the scan there
is nothing to subtract and every set stays `unknown`. A CFA mosaic is sampled at its Bayer positions
(the colour is carried by position, not by a plane), and Siril's normalized `[0,1]` float output is
rescaled to 16-bit ADU so a frame measures the same before and after a Siril pass.

You can overrule it per set from the Import light-set table, or on the wire via
`POST /api/inspect {"filter_set_overrides": {"<set id>": "dualband"}}`. An override wins outright and
applies even where nothing could be measured; detection only fills the blanks — the same precedence
the `info.txt` manifest keeps over the FITS header. Overridden rows are marked in the UI, because a
measured verdict and an asserted one mean very different things when a run comes out wrong.

## Phone (iPhone DNG) calibration masters

Milky-way captures calibrate through a **separate** library (`phone_calib_masters` table,
`internal/calib/phone.go`): masters keyed by **ISO + exposure + sensor dimensions + camera model**
(not gain/offset/bin), because they are applied **per-pixel in Go, in linear light** by the
nightscape recipe — never by Siril. Masters are dimension-guarded (a mismatched-resolution master
is dropped, never applied), built automatically when a milkyway run includes cal frames, and
reused by later runs. Kept apart so the deep-sky matcher can never pick a phone master. See
[modes/milkyway.md](modes/milkyway.md).

## Capture-side advice the engine gives you

Calibration removes the *deterministic* part of the noise; what remains at fixed sensor positions
only goes away if the sky moves randomly against the sensor between subs. The pipeline diagnoses
this per run (the **Pointing** verdict — see [pipeline.md](pipeline.md)): a session classified
*drift* or *static* gets a run warning recommending **random dithering** (~10 px between subs) —
with dithering, the adaptive rejection removes residual fixed-pattern noise entirely.
