-- 005_jobs_output_bytes: track encoded output size per completed job
-- so the dashboard's LIBRARY SIZE widget can sum across all done jobs
-- without walking the filesystem at every page load.

ALTER TABLE jobs ADD COLUMN output_bytes INTEGER NOT NULL DEFAULT 0;
