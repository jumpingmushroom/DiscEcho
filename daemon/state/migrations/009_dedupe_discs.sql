-- 009_dedupe_discs: collapse duplicate disc rows that share a non-empty
-- toc_hash on the same drive. Before this migration, every successful
-- detection inserted a fresh discs row, including after a failed rip
-- with the same physical disc still in the drive. Prod databases
-- accumulated 7+ rows for popular re-tried discs, all with identical
-- toc_hash + drive_id, with jobs scattered across them.
--
-- For each (drive_id, toc_hash) group, keep the most-recent row
-- (most-recent created_at, tiebreak by id) and reparent jobs.disc_id
-- references onto it; the older rows are deleted. Rows with empty
-- toc_hash are left alone — we can't fingerprint them.

CREATE TEMP TABLE _disc_dedup_map AS
SELECT
  d.id AS dupe_id,
  (
    SELECT d2.id
    FROM discs AS d2
    WHERE d2.drive_id = d.drive_id
      AND d2.toc_hash = d.toc_hash
    ORDER BY d2.created_at DESC, d2.id DESC
    LIMIT 1
  ) AS keeper_id
FROM discs AS d
WHERE d.toc_hash != ''
  AND d.drive_id IS NOT NULL;

UPDATE jobs
SET disc_id = (
  SELECT keeper_id FROM _disc_dedup_map WHERE dupe_id = jobs.disc_id
)
WHERE disc_id IN (
  SELECT dupe_id FROM _disc_dedup_map WHERE dupe_id != keeper_id
);

DELETE FROM discs
WHERE id IN (
  SELECT dupe_id FROM _disc_dedup_map WHERE dupe_id != keeper_id
);

DROP TABLE _disc_dedup_map;

-- Partial unique index prevents future drift: empty toc_hash is excluded
-- so unidentifiable discs can still create multiple rows, but
-- identified ones get exactly one row per drive.
CREATE UNIQUE INDEX idx_discs_drive_toc_unique
  ON discs(drive_id, toc_hash)
  WHERE toc_hash != '';
