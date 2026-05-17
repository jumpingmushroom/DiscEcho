-- v0.18.4 switched the data pipeline implementation from dd to ddrescue.
-- The engine label on existing profile rows still read "dd" and the
-- step_count was 5 (data pipeline actually emits 6 visible steps:
-- detect, identify, rip, move, notify, eject — transcode and compress
-- are marked skipped and filtered out). Bring stored rows in sync with
-- the current ddrescue-based reality.
UPDATE profiles
SET engine = 'ddrescue',
    step_count = 6
WHERE engine = 'dd';
