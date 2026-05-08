ALTER TABLE profiles ADD COLUMN container       TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN video_codec     TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN quality_preset  TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN hdr_pipeline    TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN drive_policy    TEXT NOT NULL DEFAULT 'any';
ALTER TABLE profiles ADD COLUMN auto_eject      INTEGER NOT NULL DEFAULT 1;

-- Backfill from existing flat columns so the typed editor has values to bind.
-- Format and Preset stay populated for one release as a fallback; a follow-up
-- migration drops them once the typed columns are the only source.
UPDATE profiles SET container      = format WHERE container      = '';
UPDATE profiles SET quality_preset = preset WHERE quality_preset = '';
