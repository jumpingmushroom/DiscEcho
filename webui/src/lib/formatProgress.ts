// formatProgress renders a job percentage. Default integer rendering
// (Math.round) flattens any value below 0.5% to "0%", which makes the
// dashboard look stuck during the long first-bytes window of a
// damaged-data-disc rip — ddrescue can spend several minutes on the
// initial bad spot at 0.2-0.4%, and the user reads "0%" as "not
// moving." Show one decimal place below 1%, integers otherwise.
export function formatProgress(pct: number | null | undefined): string {
  if (pct == null || Number.isNaN(pct) || pct <= 0) return '0%';
  if (pct >= 99.95) return '100%';
  if (pct < 1) return `${pct.toFixed(1)}%`;
  return `${Math.round(pct)}%`;
}
