// formatBytes renders a byte count in the largest binary unit where
// the value is at least 1. Used by the dashboard's TODAY RIPPED /
// LIBRARY SIZE widgets.
const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 B';
  if (n < 1024) return `${Math.round(n)} B`;
  let value = n;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(1)} ${UNITS[unit]}`;
}

// Loose shape of disc.metadata_json — provider-dependent. We only
// touch tracks[].duration_seconds for the track-summary line so a
// minimal pick keeps the helper resilient when MB/TMDB extend the
// payload.
interface MetadataTrack {
  duration_seconds?: number;
}

// trackSummary parses the raw metadata_json blob and returns a compact
// "N tracks · 12m" string for the drive-card identity row, or an
// empty string if the blob is missing/malformed/empty. Caller renders
// the summary only when this is non-empty.
export function trackSummary(metadataJSON: string | undefined): string {
  if (!metadataJSON) return '';
  let parsed: { tracks?: MetadataTrack[] };
  try {
    parsed = JSON.parse(metadataJSON) as { tracks?: MetadataTrack[] };
  } catch {
    return '';
  }
  const tracks = parsed.tracks;
  if (!Array.isArray(tracks) || tracks.length === 0) return '';
  const totalSec = tracks.reduce((acc, t) => acc + (t.duration_seconds ?? 0), 0);
  const trackPart = `${tracks.length} ${tracks.length === 1 ? 'track' : 'tracks'}`;
  if (totalSec <= 0) return trackPart;
  const totalMin = Math.round(totalSec / 60);
  if (totalMin < 60) return `${trackPart} · ${totalMin}m`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return `${trackPart} · ${h}h ${m}m`;
}
