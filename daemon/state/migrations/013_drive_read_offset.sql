ALTER TABLE drives ADD COLUMN read_offset INTEGER NOT NULL DEFAULT 0;
ALTER TABLE drives ADD COLUMN read_offset_source TEXT NOT NULL DEFAULT '';
-- read_offset_source values:
--   ''       — uncalibrated (default; rip output identical to pre-v0.20)
--   'manual' — user typed the offset in Settings (looked up on AccurateRip drive DB)
--   'auto'   — populated by `whipper offset find` against a calibration disc
-- The UI uses this discriminator to show a "calibrated" vs "uncalibrated" badge
-- without an extra round-trip.
