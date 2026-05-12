<script lang="ts">
  import { discs, jobs } from '$lib/store';
  import type { Disc, Job } from '$lib/wire';
  import AwaitingDecisionCard from './AwaitingDecisionCard.svelte';

  // Discs are "awaiting decision" when they have candidates (or the
  // empty-no-match case worth surfacing) AND no live job referencing
  // them. Job states that count as "live" are anything not in a
  // terminal sink — queued / running / identifying. Done / failed /
  // cancelled / interrupted jobs do NOT keep the card alive, so a
  // user can pick a different candidate after a failed run.
  const ACTIVE_JOB_STATES: ReadonlyArray<Job['state']> = ['queued', 'running', 'identifying'];

  $: liveDiscIDs = new Set(
    $jobs.filter((j) => ACTIVE_JOB_STATES.includes(j.state)).map((j) => j.disc_id),
  );

  $: pending = Object.values($discs)
    .filter((d: Disc) => !liveDiscIDs.has(d.id))
    .filter((d: Disc) => (d.candidates ?? []).length > 0 || d.type === 'DVD' || d.type === 'BDMV')
    .sort((a, b) => (a.created_at < b.created_at ? 1 : -1))
    .slice(0, 3);
</script>

{#if pending.length > 0}
  <div class="space-y-3" data-testid="awaiting-decision-list">
    {#each pending as d (d.id)}
      <AwaitingDecisionCard disc={d} />
    {/each}
  </div>
{/if}
