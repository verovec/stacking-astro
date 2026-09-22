-- E01 pruned the fork to the stacking core: the S3 mirror, the capture sequencer (and its
-- conditions/tracking records) left with their features (cards 0012 and 0014). Their tables go too —
-- dead schema invites dead code back. Children first (FK ON DELETE CASCADE is about rows, not DDL).
DROP TABLE IF EXISTS tracking_samples;
DROP TABLE IF EXISTS capture_conditions;
DROP TABLE IF EXISTS capture_forecasts;
DROP TABLE IF EXISTS capture_frames;
DROP TABLE IF EXISTS capture_sessions;
DROP TABLE IF EXISTS capture_sequences;
DROP TABLE IF EXISTS s3_objects;
DROP TABLE IF EXISTS s3_connections;
