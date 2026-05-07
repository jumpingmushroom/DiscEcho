// Helpers for rendering durations and relative times in the UI.

/**
 * formatDuration renders seconds as "Xs", "Xm Ys", or "Xh Ym Zs".
 * Negative or zero inputs render as "0s".
 */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s';
  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

/**
 * relativeTime returns "just now" / "Xm ago" / "Xh ago" within 24 hours,
 * otherwise a localised absolute date. Invalid input → empty string.
 */
export function relativeTime(iso: string, now: Date = new Date()): string {
  if (!iso) return '';
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return '';

  const diffSec = Math.floor((now.getTime() - t.getTime()) / 1000);
  if (diffSec < 5) return 'just now';
  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  return t.toLocaleString();
}
