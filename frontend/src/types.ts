// API response types mirroring the Go engine's JSON.

export interface Frame {
  path: string;
  type: string;
  filter?: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  iso?: number;
  temp_milli_c: number;
  has_temp: boolean;
  width: number;
  height: number;
  // Colour-filter-array pattern (e.g. "GRBG") for a one-shot-color frame still in its raw Bayer
  // mosaic; absent for monochrome and for already-debayered frames.
  bayer?: string;
  // Plane count: 1 for monochrome and for undebayered CFA, 3 for an already-demosaiced RGB frame.
  // Absent means undetermined (read as 1). With `bayer` this names the three states the pipeline
  // distinguishes — mono, CFA awaiting debayer, and RGB.
  channels?: number;
  object?: string;
  date_obs?: string;
  date_obs_ms?: number;
  // Capture-night key "YYYY-MM-DD" (local-noon bucketed); absent when the frame carries no DATE-OBS.
  session?: string;
  class_source: string;
  filter_confidence?: number;
  wheel_transition?: boolean;
}

export interface SetKey {
  type: string;
  object?: string;
  filter?: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  iso?: number;
  temp_bucket_c: number;
  bin: number;
  // Capture night of a per-night set — present only on multi-night scans, and only for lights/flats.
  session?: string;
  // True for a one-shot-color set. Colour and monochrome frames never share a set: nothing could
  // stack them together or calibrate one with the other's master.
  color?: boolean;
}

export interface FrameSet {
  key: SetKey;
  count: number;
  total_integration_ms: number;
}

// DetectedRun is one contiguous same-filter block found by signal-based channel detection.
export interface DetectedRun {
  filter: string;
  count: number;
  confidence: number;
  first_frame: string;
  last_frame: string;
  wheel_transition?: number;
}

export interface ChannelDetection {
  order: string[];
  overall_confidence: number;
  runs: DetectedRun[];
}

export interface Inventory {
  root: string;
  frames: Frame[];
  sets: FrameSet[];
  videos: Frame[];
  warnings: string[];
  channel_detection?: ChannelDetection;
  // How the capture records colour, decided from its lights: "mono" (a filter wheel, stacked per
  // filter and combined), "osc" (one-shot color, stacked as a single RGB channel), or "mixed" (both
  // in one folder, which no single run can stack).
  color_model?: "mono" | "osc" | "mixed";
  // Per-capture-night summary (sorted by night, undated bucket last); absent when nothing is dated.
  sessions?: SessionInfo[];
}

// SessionInfo summarizes one capture night of a scan (key "" = the undated bucket).
export interface SessionInfo {
  key: string;
  start_ms?: number;
  end_ms?: number;
  counts: Record<string, number>; // frame counts by type (LIGHT/DARK/FLAT/BIAS/…)
  configs?: SessionConfig[];
}

// SessionConfig is one distinct light-capture configuration within a night, with its frame count.
export interface SessionConfig {
  filter?: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  bin: number;
  temp_bucket_c: number;
  count: number;
}

// PreviewImage is the decoded binary buffer from GET /api/preview: a downsampled, linearly-normalized
// 16-bit image (samples 0–65535) the file viewer stretches client-side. c = 1 (mono) or 3 (RGB,
// interleaved). autoLo/autoHi are suggested default black/white points.
export interface PreviewImage {
  w: number;
  h: number;
  c: number;
  autoLo: number;
  autoHi: number;
  data: Uint16Array;
}

export interface Master {
  type: string;
  filter?: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  temp_milli_c: number;
  bin: number;
  frame_count: number;
  path: string;
}

// PhoneMaster is a reusable phone/DSLR calibration master (iPhone DNG darks/bias/flats), keyed by
// ISO / exposure / sensor dimensions rather than gain/offset/bin.
export interface PhoneMaster {
  type: string;
  iso: number;
  exposure_ms: number;
  camera_model?: string;
  width: number;
  height: number;
  frame_count: number;
  path: string;
}

export interface GradeMetric {
  index: number;
  path: string;
  fwhm: number;
  wfwhm: number;
  roundness: number;
  star_count: number;
  background: number;
  trail_detected: boolean;
  trail_score: number;
  rejected: boolean;
  reject_reason?: string;
}

export interface Selection {
  dark?: Master;
  flat?: Master;
  bias?: Master;
  notes?: string[];
}

// Calibration suggestions (POST /api/calib/preview): per inspected light channel, the master
// dark/flat/bias that would be applied — built from the capture's own cal frames (from_capture) or
// reused from the library. `id` is the per-(channel,role) key sent back to exclude one.
export interface CalibSuggestion {
  id: string;
  role: string; // "dark" | "flat" | "bias"
  master: Master;
  from_capture?: boolean;
}
export interface CalibChannel {
  filter: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  temp_bucket_c: number;
  bin: number;
  // Capture night of the light set on a multi-night scan (groups the per-night calibration mapping).
  session?: string;
  suggestions: CalibSuggestion[];
  notes?: string[];
}
export interface CalibPreview {
  channels: CalibChannel[];
}

// AlignPointsEstimate mirrors the Go struct returned by POST /api/planetary/align-points: how many
// stacking reference points the first luminance frame supports at a given minimum detail size.
export interface AlignPointsEstimate {
  frame: string;
  width: number;
  height: number;
  window_px: number;
  cell_px: number;
  per_axis: number;
  total_points: number;
  usable_points: number;
  usable_fraction: number;
  suggested_align_points: number;
  auto_per_axis: number;
  disc: { cx: number; cy: number; r: number; ok: boolean };
}

// SetQaReport mirrors internal/setqa.Report (POST /api/quality/sets): the pre-stack stray-light
// check over every light set of a selection. SetQaSet.id is the inspect SetKey.ID exclusion token
// carried by RunRequest.exclude_sets.
export interface SetQaReason {
  code:
    | "border_glow"
    | "strong_gradient"
    | "stack_visible"
    | "outlier_vs_siblings"
    | "channel_imbalance";
  border?: string;
  channel?: string;
  amplitude_pct: number;
  sigma: number;
  text: string;
}
export interface SetQaImpact {
  filter: string;
  filter_frames: number;
  filter_integration_ms: number;
  lost_frames: number;
  lost_integration_ms: number;
  lost_integration_pct: number;
  snr_factor: number;
  empties_filter: boolean;
}
export interface SetQaSet {
  id: string;
  key: SetKey;
  count: number;
  total_integration_ms: number;
  sampled: number;
  measured: boolean;
  affected_frac: number;
  border_sigma: number;
  border_pct: number;
  grad_sigma: number;
  grad_pct: number;
  worst_border?: string;
  stacked_sigma: number;
  score: number;
  flagged: boolean;
  reasons?: SetQaReason[];
  preview_frame?: string;
  impact: SetQaImpact;
}
export interface SetQaReport {
  sets: SetQaSet[];
  flagged: number;
  warnings?: string[];
}

export interface ChannelResult {
  object: string;
  filter: string;
  exposure_ms: number;
  input_frames: number;
  stacked_frames: number;
  output_path?: string;
  preview_path?: string;
  selection: Selection;
  metrics?: GradeMetric[];
  dither?: DitherReport;
  // Per-group photometric-normalization records of a cross-session merge (mirrors run.json).
  photom?: PhotomRecord[];
  // Per-night/per-session provenance of a cross-session merge (masters used, parity, previews).
  groups?: GroupResult[];
  error?: string;
  // Coverage of the anchor canvas by this channel's STACKED frames (grouped runs): fraction at the
  // preset's minimum depth + the grayscale mask thumbnail path.
  covered_frac?: number;
  coverage_mask?: string;
  canvas?: { w: number; h: number; off_x: number; off_y: number };
  mosaic_fill?: {
    filled_frac: number;
    applied: boolean;
    mask_png?: string;
    noise_sigma?: number;
  };
  seam?: unknown;
}

// CombineCrop mirrors Go pipeline.CombineCrop: the coverage-derived crop of the combine inputs.
export interface CombineCrop {
  x: number;
  y: number;
  w: number;
  h: number;
  frac: number; // rectangle area / canvas area
  applied: boolean;
  note?: string;
}

// PhotomRecord mirrors Go photom.GroupRecord: how one group's linear scale was mapped onto the
// reference group before the cross-session stack.
export interface PhotomRecord {
  session_id?: number;
  session?: string; // capture-night key — the UI's join key
  label: string;
  scale: number;
  offset: number;
  resid: number;
  frames: number;
  clamped?: boolean;
  meta_disagree?: boolean;
  meta_seeded?: boolean; // curves too flat to measure — scale IS the header exposure/gain prediction
  ref?: boolean; // the photometric reference group
  applied: boolean;
  // Which ladder rung set the scale: measured | seeded | bg-matched | offset-only | identity.
  method?: string;
  // The fitted transform would have clipped the sky below zero and was degraded (scale/offset show
  // the degraded values).
  reverted?: boolean;
}

// GroupResult mirrors Go pipeline.GroupResult: one calibration group's provenance inside a
// cross-session channel merge (run.json `channels[].groups`).
export interface GroupResult {
  session_id: number;
  current?: boolean;
  session?: string;
  filter: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  temp_bucket_c: number;
  bin: number;
  frames: number;
  dark?: string;
  flat?: string;
  bias?: string;
  flat_source?: string; // "run" | "session-rebuild" | "none"
  parity_flipped?: boolean;
  // Median field rotation (degrees) and footprint overlap (0..1) vs the run's anchor canvas,
  // measured from the merged registration (multi-group channels only).
  rotation_deg?: number;
  overlap_frac?: number;
  photom?: PhotomRecord;
  // How many of this night's frames reached the channel stack vs were rejected (registration +
  // grading) — the per-night contribution ledger.
  stacked_frames?: number;
  rejected_frames?: number;
  prenorm_preview?: string;
  normalized_preview?: string;
}

// DitherReport classifies the capture-time pointing pattern from the registration offsets:
// "dithered" (residual fixed-pattern noise decorrelates and is rejected), "drift"/"static"
// (walking-noise risk — the run-level warning recommends dithering), or "mixed".
export interface DitherReport {
  pattern: string;
  frames: number;
  span_px: number;
  step_median_px: number;
  direction_r: number;
  drift_px_per_frame: number;
  note?: string;
}

// Defect is one issue the vision model diagnosed in a supervised render.
export interface Defect {
  kind: string;
  severity: string; // low | medium | high
  note?: string;
}

// IterationRecord is one pass of the optional local-AI-agent finish supervisor.
export interface IterationRecord {
  index: number;
  tier?: string; // pipeline re-entry tier used: A (composite) | B (finish prep) | C (re-stack)
  png_path: string;
  det_score: number;
  model_score: number;
  combined_score: number;
  reasoning: string;
  defects?: Defect[];
  chosen: boolean;
  params?: Record<string, number>;
}

// StagePreview is one saved processing-milestone preview PNG (stacked/aligned/combined/colorcal/
// starless/final). `stage` is a key the UI maps to a localized label; `filter` is set for per-channel
// milestones (L/R/G/B/Ha). `index` orders the timeline left→right.
export interface StagePreview {
  index: number;
  stage: string;
  filter?: string;
  // Capture night of a per-session milestone (the prenorm/normalized pairs); absent for run-level ones.
  session?: string;
  // Mosaic panel folder ("p01"…) of a per-panel milestone; absent for run-level ones.
  tile?: string;
  png_path: string;
}

// StageArtifact is one FULL-RESOLUTION exportable stage of a finished run. The timeline previews are
// half-scale 8-bit PNGs; these render the preserved source at native resolution as PNG or TIFF. Only
// stages whose source still holds what its label claims are offered — several linear intermediates
// are processed in place, so they are omitted rather than handed back under the wrong name.
export interface StageArtifact {
  key: string;
  label: string;
  path: string;
  linear: boolean;
  filter?: string;
  order: number;
}

// One auxiliary monochrome deliverable saved next to the colour final: the processed Luminance-only
// image ("luminance") or the combined all-channel integration ("all_channels"). png/tif are also in
// FinalResult.outputs; this typed list drives the dedicated mono viewer in RunResultPanels.
export interface MonoOutput {
  kind: string;
  png: string;
  tif?: string;
}

// One entry of the star-presence set: the same finish with `percent` of the original star brightness
// kept — 0 (starless), 25/50/75, and 100 (the final itself). kind is "starless" | "stars" | "final".
// Every png except the final's is also in FinalResult.outputs; this typed list drives the star-level
// switcher in RunResultPanels.
export interface StarTier {
  kind: string;
  percent: number;
  png: string;
  tif?: string;
}

export interface FinalResult {
  mode: string;
  channels: string[];
  outputs: string[];
  notes?: string[];
  iterations?: IterationRecord[];
  mono_outputs?: MonoOutput[];
  star_tiers?: StarTier[];
}

export interface RunResult {
  input_dir: string;
  output_dir: string;
  object?: string;
  run_id?: string;
  // Engine build that produced this run (internal/buildinfo: "version" or "version (built_at)";
  // "dev" = un-stamped build). Present on nested and flat (planetary) results alike.
  engine?: string;
  detection?: ChannelDetection;
  masters: Master[];
  channels: ChannelResult[];
  final?: FinalResult;
  // The capture night whose canvas every channel master was registered onto (grouped runs only).
  anchor_night?: string;
  // Coverage-derived crop applied to the colour-combine inputs (grouped runs): the common covered
  // rectangle in canvas pixels, or the honest full-field fallback (applied=false + note).
  combine_crop?: CombineCrop;
  // Ragged-stacking-edge trim of the colour-combine inputs (edgecrop.go), measured from the stack's
  // own pixels — so unlike combine_crop it is present on single-session runs too.
  edge_crop?: CombineCrop;
  warnings: string[];
  // Planetary / comet lucky-imaging runs return a flat result (no `final` wrapper): the stacked
  // image outputs plus frame stats. RunResultPanels falls back to these when `final` is absent.
  outputs?: string[];
  notes?: string[];
  source?: string;
  frame_count?: number;
  stacked_frames?: number;
  frames?: PlanetaryFrame[];
  // Supervised-finish passes for a flat (planetary) result — nested results carry these under `final`.
  iterations?: IterationRecord[];
  // Saved processing-milestone previews (stacked/aligned/combined/finish…), for the stage timeline.
  stage_previews?: StagePreview[];
  // The run's resolved options block (run.json `options`) — the PROCESSING mode plus every fine knob,
  // persisted for provenance. `mode` here is deepsky/nebula/planetary/…; do NOT confuse it with
  // `final.mode`, which is the channel COMPOSITION (LRGB/HaLRGB/SHO/mono) and never a processing mode.
  options?: RunOptions;
}

// RunOptions is run.json's `options` block: the processing mode plus the fine knobs the run resolved.
// Open-ended because the knob set grows with the pipeline; only `mode` is relied on by name.
export interface RunOptions {
  mode?: string;
  [key: string]: unknown;
}

// StarLabel is one named object on the final image (stars.json): x/y in final-image pixel coords.
export interface StarLabel {
  x: number;
  y: number;
  name: string;
  secondary?: string; // next designation ("α Lyr" for Vega) / common name for DSOs
  kind: "star" | "dso";
  type?: string; // DSO display type (galaxy, emission_nebula, …)
  mag: number; // 99 = unknown (sorts last)
  diameter_arcmin?: number;
  // The object's catalogued footprint, pre-projected by the engine into final-image pixels (see
  // internal/annotate.Extent). Absent for stars and for DSOs with no catalogued size.
  extent?: StarLabelExtent;
  // What the star catalogue knows about this star (nil for DSOs, and for stars the shallow embedded
  // catalogue could only name).
  star?: StarCatalogInfo;
}

// StarCatalogInfo is the catalogue's record of one identified star, straight from
// internal/annotate.StarInfo. It rides on both the text labels and the plotted detections, so the
// hover card renders one block either way.
//
// absmag / ci / rv_km_s are `number | null | undefined` on purpose: zero is a real measurement for
// all three (an A0 star has B−V = 0), so "absent" cannot be encoded as 0 without inventing data.
export interface StarCatalogInfo {
  name?: string;
  secondary?: string;
  mag?: number; // catalogue V magnitude — MEASURED, unlike DetectedStar.mag
  ra_deg?: number;
  dec_deg?: number;
  dist_pc?: number; // 0/absent = unknown; no star sits at 0 pc
  absmag?: number | null; // absolute magnitude — intrinsic brightness
  ci?: number | null; // B−V colour index — the star's colour, so its temperature
  rv_km_s?: number | null; // radial velocity, km/s; positive = receding
  spect?: string; // MK spectral type, e.g. "G2 V"
  con?: string; // 3-letter IAU constellation
}

// StarLabelExtent is a DSO's elliptical footprint in FINAL-IMAGE pixels: semi-axes plus the major
// axis's angle in image space (radians, +x toward +y — the argument canvas `ellipse()` wants). The
// engine resolves sky orientation, so the overlay only has to apply the viewer's zoom.
export interface StarLabelExtent {
  rx_px: number;
  ry_px: number;
  angle_rad: number;
}

// SkyFrame anchors the final image on the sky: its centre plus the midpoints of its far x and y
// edges. Three points fix orientation, field of view AND parity — it mirrors annotate.Frame, which
// is what internal/scene3d feeds newBasis. The engine has always served it inside `solve`; this type
// simply never declared it, so TypeScript dropped it on the floor.
export interface SkyFrame {
  center_ra: number;
  center_dec: number;
  x_edge_ra: number;
  x_edge_dec: number;
  y_edge_ra: number;
  y_edge_dec: number;
}

// StarAnnotations is GET/POST /api/jobs/{id}/stars — the run's persisted stars.json: the star
// count on the linear master (windowed to the final image) plus name labels when the field's
// astrometric solution validated.
export interface StarAnnotations {
  count: number;
  image: { width: number; height: number };
  solved: boolean;
  solve?: {
    method?: string;
    reason?: string;
    scale_arcsec_px?: number;
    mag_zero_point?: number;
    row_order?: string;
    // The image's three sky anchors — the only record of the field's ROLL, and so the only way to
    // place anything external (a galactic direction, the Milky Way) into the 3D scene correctly.
    frame?: SkyFrame;
    // Which catalogue named the stars: "athyg" (the downloaded 2.5-million-star one) or "embedded"
    // (the built-in magnitude-9 extract), plus how many plotted stars it actually resolved.
    star_catalog?: string;
    identified?: number;
  };
  labels: StarLabel[];
  // Detected star positions in final-image pixels, brightest first — the individuals behind `count`.
  // Capped engine-side, so `stars.length < count` means this is the brightest slice. Present even
  // when `solved` is false: counting needs no astrometric solution.
  stars?: DetectedStar[];
  computed_at?: string;
}

// Scene3DManifest is GET /api/jobs/{id}/scene3d — the run's 3D field map, minus the star field
// itself (that rides in `points`, 24 bytes per star, so it can go straight into a GPU buffer).
// `available` false is a normal answer, not an error: `reason` then says why the run has no scene.
export interface Scene3DManifest {
  version: number;
  available: boolean;
  reason?: string;
  // True when the run's annotation is simply too old to build a scene from — recomputing the stars
  // fixes it, as opposed to a run that can never have one.
  needs_recompute?: boolean;
  image: { width: number; height: number };
  camera: Scene3DCamera;
  depth: Scene3DDepth;
  stars: Scene3DCounts;
  photometric: Scene3DPhotometric;
  billboards?: Scene3DBillboard[];
  // Full paths for /api/file — the binary star field and the billboard texture.
  points?: string;
  backdrop?: string;
  computed_at?: string;
}

// Scene3DCamera is the pinhole that reproduces the photograph: a camera at the origin looking down
// +Z with these half-field tangents renders the star field back into the picture it came from.
export interface Scene3DCamera {
  tan_half_w: number;
  tan_half_h: number;
  fov_y_deg: number;
  center_ra: number;
  center_dec: number;
  // False on a mirrored field (a session shot through a star diagonal).
  right_handed: boolean;
}

// Scene3DDepth is the field's distance structure in parsec. near/far are the 5th/95th percentiles
// and are what the logarithmic depth warp spans — the raw extremes are one foreground dwarf and one
// background giant, and letting either set the scale flattens everything between them.
export interface Scene3DDepth {
  near_pc: number;
  far_pc: number;
  min_pc: number;
  max_pc: number;
  median_pc: number;
}

export interface Scene3DCounts {
  plotted: number;
  placed: number;
  measured: number;
  estimated: number;
  unknown: number;
  identified: number;
  named: number;
  // Stars drawn at their blackbody hue rather than at the sampled pixel colour, and those with a
  // real measured space velocity.
  physical_colour: number;
  moving: number;
}

// Scene3DPhotometric grades the estimated distances. holdout_median_ratio near 1 means estimates
// land where the measured parallaxes say they should; holdout_scatter_dex is how far a single one
// may be trusted, in decades of distance.
export interface Scene3DPhotometric {
  calibrated: boolean;
  reason?: string;
  pairs: number;
  rms?: number;
  holdout_n?: number;
  holdout_median_ratio?: number;
  holdout_scatter_dex?: number;
}

// Scene3DBillboard is one catalogued object placed at its distance. The footprint is in
// final-image pixels, already projected by the engine; the object's line of sight follows from that
// centre and the camera, so it is not shipped separately (and cannot disagree with the footprint).
export interface Scene3DBillboard {
  name: string;
  secondary?: string;
  type?: string;
  dist_pc: number;
  // "measured" = derived from this frame's own member stars; "table" = the catalogued value.
  dist_source: string;
  table_dist_pc?: number;
  members?: number;
  sigma_dex?: number;
  x: number;
  y: number;
  rx_px: number;
  ry_px: number;
  angle_rad: number;
  // How the object occupies space, when enough is known to say. Absent = the flat plane.
  shape?: Scene3DShape;
}

// Scene3DShape is the engine's decision about an object's three-dimensional form. `source` is the
// honesty tier and is always shown: "measured" means the geometry follows from catalogued numbers,
// "assumed" that the form is a standard assumption at a measured size, "modelled" that no
// measurement of the third dimension exists at all. `note` says so in words, `cite` names the
// published structure a curated shape follows.
export interface Scene3DShape {
  kind: "plane" | "disc" | "shell" | "volume";
  source: "measured" | "assumed" | "modelled";
  note: string;
  cite?: string;
  inclination_deg?: number;
  position_angle_deg?: number;
  // True whenever the near and far edges cannot be told apart — which, from an ellipse alone, always.
  flip_ambiguous?: boolean;
  radius_pc?: number;
  thickness_pc?: number;
  profile?: Scene3DVolumeProfile;
}

export interface Scene3DVolumeProfile {
  depth_pc: number;
  exponent: number;
  bowl?: number;
  hollow?: number;
}

export interface DetectedStar {
  x: number;
  y: number;
  // Half-max radius in final-image px — markers scale to this so a bloated bright star reads big and
  // a faint pinprick small, instead of every star wearing the same ring.
  r_px?: number;
  // The star's own colour, sampled from the linear master and lifted toward white so an outline in
  // it stays legible ("#a8c8ff" hot, "#ffcc99" cool). Absent for mono masters.
  hex?: string;
  // Estimated apparent magnitude (99 = the frame could not be photometrically anchored). Derived
  // from solve.mag_zero_point, itself fitted on the catalogue stars identified in this frame.
  mag?: number;
  // The catalogue entry this detection was identified as, when one projects onto it. Far more
  // detections carry this than carry a text label: labels are capped and spaced so the image stays
  // readable, but hovering any marker should still answer "what is this?".
  star?: StarCatalogInfo;
}

// PlanetaryFrame is one lucky-imaging frame's quality record (kept/rejected + sharpness score).
export interface PlanetaryFrame {
  index: number;
  file: string;
  filter?: string;
  score: number;
  kept: boolean;
}

// JobParams mirrors the POST /api/jobs body (also returned in Job.params).
export interface JobParams {
  path?: string;
  paths?: string[];
  mode?: string;
  format?: string;
  filter_map?: Record<string, string>;
  drop_wheel_transition?: boolean;
  color_calibration?: boolean;
  denoise?: boolean;
  ha_exclude_stars?: boolean;
  mosaic?: boolean; // legacy alias of union_canvas (multi-night, same pointing)
  union_canvas?: boolean;
  mosaic_plan_id?: number; // tiled-mosaic mode: the saved plan this run stacks
  // Extra monochrome side-outputs (deepsky/nebula): processed Luminance-only (default on) and the
  // combined all-channel integration (default off), saved next to the colour final.
  output_luminance?: boolean;
  output_mono_stack?: boolean;
  // Deep-sky colour palette (natural|hargb|hoo|sho|hos|foraxx|mono); empty/absent → natural.
  palette?: string;
  supervise?: boolean;
  // Gated deterministic star repair (deepsky/nebula; default on). The stretch_headroom knob it (and the
  // supervisor/refine) tunes rides in `params` below, like the other fine knobs.
  auto_fix_stars?: boolean;
  sequential?: boolean;
  // Masters-only calibration build flag (kind "masters" — no pipeline recipe).
  build_masters?: boolean;
  // Milkyway (nightscape) run options.
  look?: string;
  brightness?: number;
  orientation?: string;
  dark_dir?: string;
  flat_dir?: string;
  bias_dir?: string;
  // Cross-session reuse toggles.
  reuse_disabled?: boolean;
  reuse_sessions?: string[];
  calib_exclude?: string[];
  // Light sets excluded by the Import stray-light check (inspect SetKey.ID tokens dropped
  // before grouping). Shown readonly on the job page.
  exclude_sets?: string[];
  // Force the available dark/flat/bias masters onto the lights even when gain/exposure/temperature don't
  // match (relaxes the calibration matcher). Default false = strict, physically-matched calibration.
  force_calibration_frames?: boolean;
  // Frozen snapshot of the calibration masters matched at queue time (which darks/flats/bias are included
  // and with what params) — shown on the job page. See CalibrationPanel (readonly).
  calib_plan?: CalibPreview;
  // Fine tunable-knob overrides (same whitelist/clamps as the supervisor) + the free-text objective
  // the agent carries, its re-entry ceiling and iteration cap.
  params?: Record<string, unknown>;
  goal?: string;
  // Imaging target for plate-solve/SPCC seeding — a catalogue name ("M66") or "RA,Dec" — for
  // captures whose headers/folders can't identify the field. Never renames the run.
  target?: string;
  tier?: string;
  max_iters?: number;
  // Agent improvement series this job belongs to (0/absent = none).
  series_id?: number;
}

// PresetPayload is the situation recipe a processing preset carries: the subset of the /api/jobs body a
// preset re-applies to the launch form (mirrors internal/preset.Payload). Input-specific fields (paths,
// calibration, reuse, orientation) are deliberately absent — a preset is a recipe, not a run.
export interface PresetPayload {
  mode?: string;
  format?: string;
  palette?: string;
  look?: string;
  brightness?: string;
  goal?: string;
  color_calibration?: boolean;
  denoise?: boolean;
  ha_exclude_stars?: boolean;
  mosaic?: boolean;
  output_luminance?: boolean;
  output_mono_stack?: boolean;
  drop_wheel_transition?: boolean;
  supervise?: boolean;
  params?: Record<string, unknown>;
}

// PresetItem is one entry in the preset picker: a built-in (builtin=true, id=0, name is a slug the UI
// translates, category set) or a user-saved preset (builtin=false, name is the user's text). Mirrors
// internal/preset.Item.
export interface PresetItem {
  id: number;
  name: string;
  category?: string;
  builtin: boolean;
  favorite?: boolean; // user presets only — built-ins are never starred
  payload: PresetPayload;
  created_at?: number;
  updated_at?: number;
}

// JobResume is the pause/resume checkpoint (jobs.resume). Cause distinguishes a manual pause (stays
// paused until the user continues) from an error pause (auto-resumed with backoff).
export interface JobResume {
  phase?: string;
  cause?: "manual" | "error";
  attempts?: number;
  next_retry_ms?: number;
  reason?: string;
}

export interface Job {
  id: number;
  session_id: number;
  kind: string;
  status: string;
  progress: number;
  current_step: string;
  error: string;
  params?: JobParams;
  resume?: JobResume;
  log_tail?: string;
  result: RunResult;
  started_at_ms: number; // 0 until the job leaves the queue and starts processing
  finished_at_ms: number; // 0 until the job reaches a terminal state
  series_id?: number; // agent improvement series (0 = none)
  created_at: number;
  updated_at: number;
}

// Series is one durable agent improvement campaign over a target; each attempt is a normal job
// linked by jobs.series_id (GET /api/series, GET /api/series/{id}).
export interface Series {
  id: number;
  object: string;
  kind: string;
  input_path: string;
  goal: string;
  status: string; // active | done | stopped
  auto_continue: boolean;
  max_attempts: number;
  target_score: number;
  best_job_id: number;
  best_score: number;
  created_at: number;
  updated_at: number;
  attempts?: number; // attempt count — present on GET /api/series list rows only
}

export interface BrowseEntry {
  name: string;
  path: string;
  is_dir: boolean;
  local?: boolean; // present on local disk (always true from the current backend)
}

// One capture folder of a past processing (GET /api/processed). `exists` = still on local disk (usable);
// `local` mirrors it. `rel` is the DataDir-relative slash path.
export interface ProcessedPath {
  path: string;
  exists: boolean;
  local: boolean;
  rel: string;
}

// One past processing (a job) and the capture folders it consumed (GET /api/processed).
// `signature` is the backend-computed folder-set key (store.SelectionSignature) used to dedup
// history rows and join saved-selection names/stars; optional for old-backend tolerance.
export interface ProcessedGroup {
  job_id: number;
  kind: string;
  object?: string;
  mode?: string;
  format?: string;
  status: string;
  created_at_ms: number;
  signature?: string;
  paths: ProcessedPath[];
}

// A saved (named/starred) selection riding along GET /api/processed, with its folders annotated by
// the same existence machinery as the groups — an orphaned selection (jobs pruned) still renders.
export interface SavedSelectionInfo {
  id: number;
  name: string;
  favorite: boolean;
  signature: string;
  mode?: string;
  format?: string;
  updated_at_ms: number;
  paths: ProcessedPath[];
}

// Per-folder processing info derived client-side to annotate the folder browser. groupColor is set
// only when the folder's (most recent) processing spanned multiple folders, so siblings of one
// processing share a colour.
export interface ProcessedFolder {
  jobId: number;
  object?: string;
  kind: string;
  runs: number; // how many past jobs included this folder
  groupColor?: string;
  groupSize?: number;
}

// A de-duplicated past folder-set offered for re-running in the Import "Processing history". Several
// jobs over the same set collapse into one entry (runs counts them); the most recent supplies the rest.
// `selection` is the saved name/star joined by signature; `jobId: 0` marks an orphaned saved selection
// (its jobs aged out of the history window) synthesized from the saved row alone.
export interface ProcessingHistoryEntry {
  jobId: number;
  object?: string;
  mode?: string;
  format?: string;
  status: string;
  createdAtMs: number;
  runs: number;
  signature: string;
  selection?: { id: number; name: string; favorite: boolean };
  paths: ProcessedPath[];
}

// LogLine is one console line with the wall-clock time it was captured (null for legacy/untimed lines).
// seq is a monotonic id assigned at produce time so the log list keeps stable :key across ring-buffer
// trims (index keys would re-patch the whole list on every trimmed line).
export interface LogLine {
  ts: number | null;
  text: string;
  seq?: number;
}

// RunSummary is one durable on-disk run (GET /api/runs).
export interface RunSummary {
  object: string;
  run_id: string;
  dir: string;
  run_json: string;
  final_preview?: string;
  mode?: string;
  channels?: string[];
  created_at_ms: number;
  // Engine build that produced the run (from its run.json; absent when the summary predates stamping).
  engine?: string;
}

// Health is the GET /api/health snapshot; engine identifies the serving build ("dev" = un-stamped).
export interface EngineBuild {
  version: string;
  built_at: string;
}
export interface Health {
  status: string;
  data_dir: string;
  output_dir: string;
  library_dir: string;
  engine: EngineBuild;
}

// Environment health (GET /api/environment): deep per-tool probes + the offline plate-solve
// catalogue situation, with human-readable run-impacting warnings. Cached ~5 min server-side.
export interface EnvTool {
  ok: boolean;
  detail?: string; // version / resolved kind / probe state (may be "probing")
  err?: string;
}
export interface EnvPlateSolve {
  local_gaia_astro: boolean;
  xpsamp_chunks: number;
  local_asnet: boolean;
  catalog: string; // effective platesolve -catalog value ("" = Siril default/online)
}
export interface Environment {
  siril: EnvTool;
  gimp: EnvTool;
  graxpert: EnvTool;
  starnet: EnvTool;
  raw_developer: EnvTool;
  llm: EnvTool;
  plate_solve: EnvPlateSolve;
  checked_ms: number;
  warnings?: string[];
}

// Cross-session reuse: prior light data a run can fold in to grow total integration.
export interface ReuseSessionInfo {
  session_id: number;
  frames: number;
  integration_ms: number;
  filters: string[];
  // Distinct capture nights this prior session contributes ("YYYY-MM-DD", sorted).
  nights?: string[];
}

// ---- POST /api/calib/plan: the joined per-session run plan (pipeline.RunPlanPreview) ----

// PlanMaster is one master a group would use, with its provenance.
export interface PlanMaster {
  source: string; // "library" | "capture" | "session-rebuild"
  master?: Master;
  raw_flats?: number; // session-rebuild: how many raw flats will stack
  suggest_id?: string; // the calib_exclude key (same identity as the calibration preview)
}

// PlanGroup is one (session, night, config) calibration group and its masters (nil role = skipped).
export interface PlanGroup {
  session_id: number;
  current?: boolean;
  session?: string;
  exposure_ms: number;
  gain: number;
  offset: number;
  temp_bucket_c: number;
  bin: number;
  frames: number;
  dark?: PlanMaster;
  flat?: PlanMaster;
  bias?: PlanMaster;
  notes?: string[];
}

export interface PlanChannel {
  filter: string;
  groups: PlanGroup[];
}

// PlanSession is one (session, capture-night) contribution to the run, current capture first.
export interface PlanSession {
  session_id: number;
  current?: boolean;
  session?: string;
  frames: number;
  integration_ms: number;
  filters?: string[];
}

export interface RunPlanPreview {
  object: string;
  has_coords: boolean;
  sessions?: PlanSession[];
  channels: PlanChannel[];
  reuse: ReuseSummary;
  // The capture night whose canvas every channel master will be registered onto (grouped runs).
  anchor_night?: string;
  warnings?: string[];
}

export interface ReuseSummary {
  prior_sessions: number;
  prior_frames: number;
  added_integration_ms: number;
  sessions?: ReuseSessionInfo[];
}

export interface ReusePreview {
  object: string;
  has_coords: boolean;
  current_frames: number;
  current_integration_ms: number;
  reuse: ReuseSummary;
}

export interface SkyLocation {
  lat: number;
  lon: number;
  elevation_m: number;
  timezone: string;
  source: string; // "config" | "query"
}

export interface SkyEyepiece {
  label: string;
  focal_mm: number;
  afov_deg: number;
}

export interface SkyEquipment {
  focal_mm: number;
  aperture_mm: number;
  pixel_um: number;
  sensor_w_px: number;
  sensor_h_px: number;
  image_scale_arcsec_px: number;
  fov_w_deg: number;
  fov_h_deg: number;
  f_ratio: number;
  barlow_x: number; // Barlow/amplifier factor folded into scale, FOV, f-ratio & magnification (1 = none)
  reducer_x: number; // focal reducer folded into the same values, independently of the Barlow (1 = none)
  mode?: "camera" | "visual"; // present only in eyepiece (visual) mode
  eyepieces?: SkyEyepiece[]; // the configured visual kit (visual mode)
}

export interface SkyQueryEcho {
  at_utc_ms: number;
  at_local: string;
  location: SkyLocation;
  equipment: SkyEquipment;
  min_alt_deg: number;
  twilight: string;
  limit: number;
}

// A saved observing location (Tonight favorites), persisted in localStorage. id is a coordinate key so
// saving the same spot twice is idempotent; label is the place name (from search) or a "lat, lon" string.
export interface LocationFavorite {
  id: string;
  label: string;
  lat: number;
  lon: number;
  elevation_m?: number;
}

// A named, reusable telescope + camera + eyepiece rig the user can save and pick from later. Now
// persisted server-side (table equipment_setups, see stores/equipment.ts) so the desktop that plans a
// mosaic and the phone that shoots it agree on the optics; the id is the row id as a string. Numeric
// fields are optional so a partially-filled rig can still be saved.
export interface EquipmentSetup {
  id: string;
  name: string;
  focal_mm?: number;
  aperture_mm?: number;
  barlow?: number;
  reducer?: number;
  pixel_um?: number;
  sensor_w?: number;
  sensor_h?: number;
  camera_name?: string; // filled by "use connected camera" once a camera is attached
  eyepieces: SkyEyepiece[];
}

// EquipmentSetupRow is the wire shape of GET/POST /api/equipment. Field names match the plan-request
// optics (sensor_w_px/barlow_x); stores/equipment.ts projects it onto EquipmentSetup.
export interface EquipmentSetupRow {
  id: number;
  name: string;
  focal_mm: number;
  aperture_mm: number;
  pixel_um: number;
  sensor_w_px: number;
  sensor_h_px: number;
  barlow_x: number;
  reducer_x: number;
  camera_name: string;
  eyepieces: SkyEyepiece[];
  favorite: boolean;
  created_at: number;
  updated_at: number;
}

// ---- Mosaic planner (mirrors internal/mosaicplan + /api/mosaic wire shapes) ----

// AladinTarget is the minimal object the Aladin sky view centers on. The mosaic planner synthesizes
// it from the resolved preview echo (the Tonight planner's richer SkyTarget left with E01 card 0015).
export interface AladinTarget {
  name: string;
  ra_deg: number;
  dec_deg: number;
}

export interface MosaicTile {
  index: number; // row*cols+col — stable identity for capture-status keys
  row: number;
  col: number;
  order: number; // 1-based serpentine capture order
  folder: string; // "p01"… — the panel-subfolder convention
  ra_deg: number;
  dec_deg: number;
  corners: [number, number][]; // [ra,dec] × TL,TR,BR,BL in frame orientation
  alt_deg: number;
  az_deg: number;
  transit_utc_ms: number;
  meridian_side: "east" | "west";
}

export interface MosaicGrid {
  rows: number;
  cols: number;
  tile_w_deg: number;
  tile_h_deg: number;
  step_w_deg: number;
  step_h_deg: number;
  camera_pa_deg: number;
  overlap_frac: number;
}

export interface MosaicQueryEcho {
  target?: string;
  ra_deg: number;
  dec_deg: number;
  size_arcmin: number;
  size_minor_arcmin?: number;
  object_pa_deg?: number;
  center_ra_deg: number; // effective grid centre (the object unless hand-framed)
  center_dec_deg: number;
  center_moved?: boolean;
  fov_w_deg: number;
  fov_h_deg: number;
  image_scale_arcsec_px: number;
  margin_arcmin: number;
  lat: number;
  lon: number;
  at_utc_ms: number;
}

export interface MosaicPreview {
  query: MosaicQueryEcho;
  grid: MosaicGrid;
  tiles: MosaicTile[];
  warnings?: string[];
}

// SkySearchResult is one hit from GET /api/sky/search — free-text lookup over the WHOLE merged
// deep-sky catalogue (not just tonight's visible list). Optional fields are absent when the
// catalogue has no value, so "unknown size" is distinguishable from a genuine 0.
export interface SkySearchResult {
  name: string;
  ra_deg: number;
  dec_deg: number;
  type?: string;
  source?: string;
  size_arcmin?: number;
  size_minor_arcmin?: number;
  position_angle_deg?: number;
  mag?: number;
  morphology?: string;
  common_names?: string[];
  aliases?: string[];
}

// MosaicRequestBody is what the UI sends (preview + plan create/update). Absent fields fall back
// to catalogue values (for a named target) or the engine's configured rig/site.
export interface MosaicRequestBody {
  target_name?: string;
  ra_deg?: number;
  dec_deg?: number;
  center_ra_deg?: number; // hand-framed grid centre (dragged on the map); both or neither
  center_dec_deg?: number;
  size_arcmin?: number;
  size_minor_arcmin?: number;
  object_pa_deg?: number;
  optics?: {
    focal_mm?: number;
    aperture_mm?: number;
    pixel_um?: number;
    sensor_w_px?: number;
    sensor_h_px?: number;
    barlow_x?: number;
    reducer_x?: number;
  };
  overlap_frac?: number;
  margin_arcmin?: number;
  camera_pa_deg?: number;
  rows_override?: number;
  cols_override?: number;
  lat?: number;
  lon?: number;
  at?: string; // RFC3339
}

// MosaicPlanRequest is the server's resolved snapshot stored on a saved plan (Go mosaicplan.Request).
export interface MosaicPlanRequest {
  ra_deg: number;
  dec_deg: number;
  size_arcmin: number;
  size_minor_arcmin: number;
  object_pa_deg: number;
  has_object_pa: boolean;
  center_ra_deg: number;
  center_dec_deg: number;
  has_center: boolean;
  optics: {
    focal_mm: number;
    aperture_mm: number;
    pixel_um: number;
    sensor_w_px: number;
    sensor_h_px: number;
    barlow_x: number;
    reducer_x: number;
  };
  overlap_frac: number;
  margin_arcmin: number;
  camera_pa_deg: number;
  rows_override: number;
  cols_override: number;
  lat: number;
  lon: number;
  at: string;
}

export type MosaicTileStatus = "pending" | "captured" | "skipped";

// What one panel+filter actually holds on disk, reconciled from the frames themselves
// (POST /api/mosaic/plans/{id}/reconcile → internal/mosaic/progress.go).
export interface MosaicFilterProgress {
  frames: number;
  seconds: number;
  last_ms?: number;
  nights?: number;
}

// panel folder ("p01") → filter ("L") → tally.
export type MosaicTileProgress = Record<
  string,
  Record<string, MosaicFilterProgress>
>;

// The per-filter goal for every tile: what makes a tile "done" without the user ticking a box.
export interface MosaicCaptureTarget {
  filter: string;
  frames: number;
  exposure_ms?: number;
  gain?: number;
  offset?: number;
  bin?: number;
  dither?: number;
}

export interface MosaicPlanRow {
  id: number;
  name: string;
  object_name: string;
  request: MosaicPlanRequest;
  grid: MosaicGrid;
  tiles: MosaicTile[];
  tile_status: Record<string, MosaicTileStatus>;
  capture_targets: MosaicCaptureTarget[];
  tile_progress: MosaicTileProgress;
  capture_root?: string;
  reconciled_at?: number;
  orientation_done: boolean;
  created_at: number;
  updated_at: number;
}

export interface StarfieldStar {
  ra_deg: number;
  dec_deg: number;
  mag: number;
}
