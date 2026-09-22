-- Recreates the pruned schema exactly as migrations 0006/0009/0013/0018/0019/0020/0022 left it
-- (definitions copied verbatim, comments trimmed). Data is NOT restored — the features are gone.

CREATE TABLE s3_connections (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT    NOT NULL,
    endpoint      TEXT    NOT NULL DEFAULT '',
    region        TEXT    NOT NULL DEFAULT 'us-east-1',
    access_key_id TEXT    NOT NULL,
    secret_enc    BYTEA   NOT NULL,
    use_ssl       BOOLEAN NOT NULL DEFAULT TRUE,
    is_default    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    BIGINT  NOT NULL,
    updated_at    BIGINT  NOT NULL,
    default_storage_class TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX idx_s3_connections_default ON s3_connections(is_default) WHERE is_default;

CREATE TABLE s3_objects (
    id         BIGSERIAL PRIMARY KEY,
    bucket     TEXT   NOT NULL,
    prefix     TEXT   NOT NULL DEFAULT '',
    local_rel  TEXT   NOT NULL,
    s3_key     TEXT   NOT NULL,
    size       BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX idx_s3_objects_scope_rel ON s3_objects(bucket, prefix, local_rel text_pattern_ops);
CREATE INDEX idx_s3_objects_key ON s3_objects(bucket, s3_key text_pattern_ops);

CREATE TABLE capture_sequences (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT    NOT NULL,
    payload    JSONB   NOT NULL DEFAULT '{}',
    favorite   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at BIGINT  NOT NULL,
    updated_at BIGINT  NOT NULL
);
CREATE UNIQUE INDEX idx_capture_sequences_name_lower ON capture_sequences (LOWER(name));

CREATE TABLE capture_sessions (
    id             BIGSERIAL PRIMARY KEY,
    object         TEXT   NOT NULL DEFAULT '',
    root           TEXT   NOT NULL DEFAULT '',
    panel          TEXT   NOT NULL DEFAULT '',
    mosaic_plan_id BIGINT NOT NULL DEFAULT 0,
    tile_index     INTEGER NOT NULL DEFAULT -1,
    sequence       JSONB  NOT NULL DEFAULT '{}',
    status         TEXT   NOT NULL DEFAULT 'running',
    progress       JSONB  NOT NULL DEFAULT '{}',
    total_frames   INTEGER NOT NULL DEFAULT 0,
    frames_done    INTEGER NOT NULL DEFAULT 0,
    started_at     BIGINT NOT NULL DEFAULT 0,
    ended_at       BIGINT NOT NULL DEFAULT 0,
    created_at     BIGINT NOT NULL,
    updated_at     BIGINT NOT NULL,
    site_lat           DOUBLE PRECISION NOT NULL DEFAULT 0,
    site_lon           DOUBLE PRECISION NOT NULL DEFAULT 0,
    site_elevation_m   DOUBLE PRECISION NOT NULL DEFAULT 0,
    conditions_summary JSONB            NOT NULL DEFAULT '{}',
    request            JSONB            NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_capture_sessions_started ON capture_sessions (started_at DESC);
CREATE INDEX idx_capture_sessions_plan ON capture_sessions (mosaic_plan_id) WHERE mosaic_plan_id <> 0;

CREATE TABLE capture_frames (
    id           BIGSERIAL PRIMARY KEY,
    session_id   BIGINT NOT NULL REFERENCES capture_sessions(id) ON DELETE CASCADE,
    path         TEXT   NOT NULL,
    filter       TEXT   NOT NULL DEFAULT '',
    frame_type   TEXT   NOT NULL DEFAULT 'light',
    exposure_us  BIGINT NOT NULL DEFAULT 0,
    gain         BIGINT NOT NULL DEFAULT 0,
    frame_offset BIGINT NOT NULL DEFAULT 0,
    bin          INTEGER NOT NULL DEFAULT 1,
    temp_milli_c INTEGER NOT NULL DEFAULT 0,
    panel        TEXT   NOT NULL DEFAULT '',
    sequence_no  INTEGER NOT NULL DEFAULT 0,
    started_at   BIGINT NOT NULL DEFAULT 0,
    created_at   BIGINT NOT NULL
);
CREATE INDEX idx_capture_frames_session ON capture_frames (session_id);
CREATE INDEX idx_capture_frames_started ON capture_frames (started_at DESC);

CREATE TABLE capture_conditions (
    id             BIGSERIAL PRIMARY KEY,
    session_id     BIGINT NOT NULL REFERENCES capture_sessions(id) ON DELETE CASCADE,
    at_ms          BIGINT NOT NULL,
    session_status TEXT   NOT NULL DEFAULT 'running',
    cloud_pct      DOUBLE PRECISION NOT NULL DEFAULT 0,
    cloud_low      DOUBLE PRECISION NOT NULL DEFAULT 0,
    cloud_mid      DOUBLE PRECISION NOT NULL DEFAULT 0,
    cloud_high     DOUBLE PRECISION NOT NULL DEFAULT 0,
    seeing_arcsec  DOUBLE PRECISION NOT NULL DEFAULT 0,
    transparency   DOUBLE PRECISION NOT NULL DEFAULT 0,
    humidity_pct   DOUBLE PRECISION NOT NULL DEFAULT 0,
    dew_point_c    DOUBLE PRECISION NOT NULL DEFAULT 0,
    temp_c         DOUBLE PRECISION NOT NULL DEFAULT 0,
    dew_spread_c   DOUBLE PRECISION NOT NULL DEFAULT 0,
    dew_risk       TEXT             NOT NULL DEFAULT '',
    wind_kmh       DOUBLE PRECISION NOT NULL DEFAULT 0,
    gust_kmh       DOUBLE PRECISION NOT NULL DEFAULT 0,
    jet300_kmh     DOUBLE PRECISION NOT NULL DEFAULT 0,
    cape           DOUBLE PRECISION NOT NULL DEFAULT 0,
    lifted_index   DOUBLE PRECISION NOT NULL DEFAULT 0,
    visibility_m   DOUBLE PRECISION NOT NULL DEFAULT 0,
    precip_pct     DOUBLE PRECISION NOT NULL DEFAULT 0,
    aod            DOUBLE PRECISION NOT NULL DEFAULT 0,
    verdict        DOUBLE PRECISION NOT NULL DEFAULT 0,
    kp_now         DOUBLE PRECISION NOT NULL DEFAULT 0,
    kp_max         DOUBLE PRECISION NOT NULL DEFAULT 0,
    aurora         TEXT             NOT NULL DEFAULT '',
    moon_illum           DOUBLE PRECISION NOT NULL DEFAULT 0,
    moon_alt_deg         DOUBLE PRECISION NOT NULL DEFAULT 0,
    moon_az_deg          DOUBLE PRECISION NOT NULL DEFAULT 0,
    moon_phase_angle_deg DOUBLE PRECISION NOT NULL DEFAULT 0,
    moon_sep_deg         DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_alt_deg       DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_az_deg        DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_airmass       DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_valid         BOOLEAN          NOT NULL DEFAULT FALSE,
    sqm    DOUBLE PRECISION NOT NULL DEFAULT 0,
    bortle INTEGER          NOT NULL DEFAULT 0,
    forecast_age_ms BIGINT NOT NULL DEFAULT 0,
    source          TEXT   NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL
);
CREATE INDEX capture_conditions_session_idx ON capture_conditions (session_id, at_ms);

CREATE TABLE capture_forecasts (
    id         BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES capture_sessions(id) ON DELETE CASCADE,
    kind       TEXT   NOT NULL,
    at_ms      BIGINT NOT NULL,
    payload    JSONB  NOT NULL DEFAULT '{}',
    created_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX capture_forecasts_session_kind_idx ON capture_forecasts (session_id, kind);

CREATE TABLE tracking_samples (
    id           BIGSERIAL PRIMARY KEY,
    session_id   BIGINT NOT NULL REFERENCES capture_sessions(id) ON DELETE CASCADE,
    t_sec        DOUBLE PRECISION NOT NULL,
    ra_arcsec    DOUBLE PRECISION NOT NULL,
    dec_arcsec   DOUBLE PRECISION NOT NULL,
    source       TEXT NOT NULL DEFAULT 'match',
    created_at   BIGINT NOT NULL
);
CREATE INDEX tracking_samples_session_idx ON tracking_samples (session_id, t_sec);
