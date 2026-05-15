-- 008_global_eject_setting: introduce two new globals — operation.mode
-- (batch|manual) and rip.eject_on_finish (bool) — and retire the
-- per-profile auto_eject column. Mode controls whether the UI's 8s
-- auto-confirm fires after identify; rip.eject_on_finish controls
-- tray eject at the end of every rip (ignored in manual mode).
--
-- The drop is destructive but the data is replaced by a single global,
-- defaulted to ON so existing installs keep the same eject behaviour.
INSERT INTO settings (key, value, updated_at)
  VALUES ('operation.mode', 'batch', strftime('%Y-%m-%dT%H:%M:%fZ','now'))
  ON CONFLICT(key) DO NOTHING;

INSERT INTO settings (key, value, updated_at)
  VALUES ('rip.eject_on_finish', 'true', strftime('%Y-%m-%dT%H:%M:%fZ','now'))
  ON CONFLICT(key) DO NOTHING;

ALTER TABLE profiles DROP COLUMN auto_eject;
