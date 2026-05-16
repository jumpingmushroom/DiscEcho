import type { Job } from '$lib/wire';

/**
 * Returns the most recently finished `done` job for a disc, or `undefined`
 * if there is none. Used by drive cards to surface the "already ripped
 * <date> — re-rip?" affordance.
 *
 * Sort is by `finished_at` descending; a missing `finished_at` sorts last
 * (defensive — `done` jobs should always have a finish timestamp).
 */
export function lastDoneJobForDisc(jobs: Job[], discID: string | undefined): Job | undefined {
  if (!discID) return undefined;
  return jobs
    .filter((j) => j.disc_id === discID && j.state === 'done')
    .sort((a, b) => ((a.finished_at ?? '') < (b.finished_at ?? '') ? 1 : -1))[0];
}
