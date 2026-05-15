<script lang="ts">
  import { discs, jobs } from '$lib/store';
  import type { Disc } from '$lib/wire';
  import AwaitingDecisionCard from './AwaitingDecisionCard.svelte';

  // A disc is "awaiting decision" when the user hasn't yet picked a
  // candidate for it. Picking creates a Job — so the moment ANY job
  // (running, queued, OR terminal) exists for the disc's id, the
  // decision was made, regardless of how that job ended. Re-rips from
  // history will be a separate, explicit affordance; until that ships
  // the disc stays off this surface so an old failed rip doesn't
  // re-prompt every time the page loads.
  $: decidedDiscIDs = new Set($jobs.map((j) => j.disc_id));

  // No filter on candidate count: a disc with zero candidates is still
  // an awaiting decision — it just means MusicBrainz / TMDB didn't
  // recognise it, and the card needs to surface that to the user
  // (otherwise the dashboard silently swallows the insert and the
  // drive just snaps back to idle). The card itself renders the right
  // copy + affordances for zero-candidate discs based on disc.type.
  $: pending = Object.values($discs)
    .filter((d: Disc) => !decidedDiscIDs.has(d.id))
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
