-- 003_dvd_default_mkv: flip DVD-Movie default from MP4 to MKV and
-- tag both DVD seed profiles with an explicit dvd_selection_mode so
-- the pipeline can disambiguate movie-vs-series without relying on
-- the format field.
--
-- The DVD-Movie UPDATE is conditional: it only fires when format,
-- container, and the output template still match the original seed.
-- That keeps user-customised rows untouched.
--
-- DVD-Series only gets the new options key (no destructive change),
-- so its WHERE clause is looser.

UPDATE profiles
   SET format = 'MKV',
       container = 'MKV',
       output_path_template = REPLACE(output_path_template, '.mp4', '.mkv'),
       options_json = json_set(options_json, '$.dvd_selection_mode', 'main_feature'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE disc_type = 'DVD'
   AND name = 'DVD-Movie'
   AND format = 'MP4'
   AND container = 'MP4'
   AND output_path_template = '{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mp4';

UPDATE profiles
   SET options_json = json_set(options_json, '$.dvd_selection_mode', 'per_title'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE disc_type = 'DVD'
   AND name = 'DVD-Series'
   AND format = 'MKV';
