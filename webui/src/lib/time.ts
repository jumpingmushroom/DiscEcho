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

/**
 * dayGroupLabel returns a human-friendly day-group key:
 *   Today / Yesterday / "N days ago" (2..6) / "N weeks ago" (1..4) /
 *   absolute date past 30 days. Used by the History screen to bucket rows.
 *
 * Comparison is by calendar day, not 24-hour windows, so a midnight
 * boundary correctly bumps "Today" → "Yesterday".
 */
export function dayGroupLabel(iso: string, now: Date = new Date()): string {
  if (!iso) return '';
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return '';

  const startOfDay = (d: Date): Date => new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const days = Math.floor((startOfDay(now).getTime() - startOfDay(t).getTime()) / 86400000);

  if (days <= 0) return 'Today';
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days} days ago`;
  if (days < 30) {
    const weeks = Math.floor(days / 7);
    return weeks === 1 ? '1 week ago' : `${weeks} weeks ago`;
  }
  return t.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}
