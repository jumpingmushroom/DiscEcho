CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE drives (
  id           TEXT PRIMARY KEY,
  model        TEXT NOT NULL,
  bus          TEXT NOT NULL,
  dev_path     TEXT NOT NULL UNIQUE,
  state        TEXT NOT NULL CHECK (state IN ('idle','identifying','ripping','ejecting','error')),
  last_seen_at TEXT NOT NULL,
  notes        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE discs (
  id                TEXT PRIMARY KEY,
  drive_id          TEXT REFERENCES drives(id) ON DELETE SET NULL,
  type              TEXT NOT NULL CHECK (type IN ('AUDIO_CD','DVD','BDMV','UHD','PSX','PS2','XBOX','SAT','DC','VCD','DATA')),
  title             TEXT NOT NULL DEFAULT '',
  year              INTEGER NOT NULL DEFAULT 0,
  runtime_seconds   INTEGER NOT NULL DEFAULT 0,
  size_bytes_raw    INTEGER NOT NULL DEFAULT 0,
  toc_hash          TEXT NOT NULL DEFAULT '',
  metadata_provider TEXT NOT NULL DEFAULT '',
  metadata_id       TEXT NOT NULL DEFAULT '',
  candidates_json   TEXT NOT NULL DEFAULT '[]',
  created_at        TEXT NOT NULL
);

CREATE TABLE profiles (
  id                   TEXT PRIMARY KEY,
  disc_type            TEXT NOT NULL,
  name                 TEXT NOT NULL UNIQUE,
  engine               TEXT NOT NULL,
  format               TEXT NOT NULL,
  preset               TEXT NOT NULL,
  options_json         TEXT NOT NULL DEFAULT '{}',
  output_path_template TEXT NOT NULL,
  enabled              INTEGER NOT NULL DEFAULT 1,
  step_count           INTEGER NOT NULL,
  created_at           TEXT NOT NULL,
  updated_at           TEXT NOT NULL
);

CREATE TABLE jobs (
  id              TEXT PRIMARY KEY,
  disc_id         TEXT NOT NULL REFERENCES discs(id) ON DELETE CASCADE,
  drive_id        TEXT REFERENCES drives(id) ON DELETE SET NULL,
  profile_id      TEXT NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
  state           TEXT NOT NULL CHECK (state IN ('queued','identifying','running','paused','done','failed','cancelled','interrupted')),
  active_step     TEXT NOT NULL DEFAULT '',
  progress        REAL NOT NULL DEFAULT 0,
  speed           TEXT NOT NULL DEFAULT '',
  eta_seconds     INTEGER NOT NULL DEFAULT 0,
  elapsed_seconds INTEGER NOT NULL DEFAULT 0,
  started_at      TEXT NOT NULL DEFAULT '',
  finished_at     TEXT NOT NULL DEFAULT '',
  error_message   TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL
);

CREATE INDEX idx_jobs_state    ON jobs(state);
CREATE INDEX idx_jobs_drive    ON jobs(drive_id, state);
CREATE INDEX idx_jobs_finished ON jobs(finished_at);

CREATE TABLE job_steps (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id        TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  step          TEXT NOT NULL CHECK (step IN ('detect','identify','rip','transcode','compress','move','notify','eject')),
  state         TEXT NOT NULL CHECK (state IN ('pending','running','done','skipped','failed')),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  started_at    TEXT NOT NULL DEFAULT '',
  finished_at   TEXT NOT NULL DEFAULT '',
  notes_json    TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_steps_job ON job_steps(job_id);

CREATE TABLE log_lines (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id  TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  t       TEXT NOT NULL,
  level   TEXT NOT NULL CHECK (level IN ('debug','info','warn','error')),
  message TEXT NOT NULL
);

CREATE INDEX idx_logs_job ON log_lines(job_id, id);

CREATE TABLE notifications (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  url        TEXT NOT NULL,
  tags       TEXT NOT NULL DEFAULT '',
  triggers   TEXT NOT NULL DEFAULT 'done,failed',
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
