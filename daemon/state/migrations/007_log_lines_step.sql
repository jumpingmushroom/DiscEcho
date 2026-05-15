-- 007_log_lines_step: tag every log line with its pipeline step so the
-- webui can group by phase (rip / transcode / move / …) without having
-- to infer phase from log-message prefixes or SSE timing. The default
-- '' is what pre-007 rows get; new rows are written with a real StepID
-- by PersistentSink.OnLog.
ALTER TABLE log_lines ADD COLUMN step TEXT NOT NULL DEFAULT '';
