-- 004_disc_metadata_json: extended display metadata (cast, plot, track
-- list, etc.) lives in a single JSON blob alongside the typed columns.
-- Pane components read this blob to render per-disc-type detail views.

ALTER TABLE discs ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';
