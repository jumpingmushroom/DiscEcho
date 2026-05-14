-- 006_dvd_quality_options: encode quality is now a real, per-profile
-- setting (quality_rf + encoder_preset) rather than a constant
-- hardcoded in the DVD pipeline. Profile seeders early-out on existing
-- rows, so this migration is the only path that updates DBs created
-- before this release.
--
-- First: backfill the two new options into every DVD HandBrake profile
-- (including any custom ones) so the pipeline has explicit values to
-- read instead of falling back to its defaults.
UPDATE profiles
SET options_json = json_set(
      json_set(
        COALESCE(NULLIF(options_json, ''), '{}'),
        '$.quality_rf', 18),
      '$.encoder_preset', 'slow')
WHERE engine = 'HandBrake' AND disc_type = 'DVD';

-- Then: refresh the cosmetic preset/quality_preset display strings on
-- the two seeded DVD profiles so the UI stops showing the old "RF 20"
-- text while the encode actually runs at RF 18. Targeted by seed name
-- so a user's renamed/custom profile keeps whatever label they chose.
UPDATE profiles
SET preset = 'x264 RF 18 · slow', quality_preset = 'x264 RF 18 · slow'
WHERE engine = 'HandBrake' AND disc_type = 'DVD' AND name = 'DVD-Movie';

UPDATE profiles
SET preset = 'x264 RF 18 · slow · per-title', quality_preset = 'x264 RF 18 · slow · per-title'
WHERE engine = 'HandBrake' AND disc_type = 'DVD' AND name = 'DVD-Series';
