<script lang="ts">
  import { discs, jobs } from '$lib/store';
  import type { Disc } from '$lib/wire';
  import AwaitingDecisionCard from './AwaitingDecisionCard.svelte';

  // A disc is "awaiting decision" when it has no active or completed
  // rip job — failed / cancelled / interrupted are intentionally NOT
  // hidden so the user is auto-prompted to retry (clear retry intent).
  // Running and queued jobs DO hide the card: an active job means the
  // disc is already being handled and re-showing the card would auto-
  // rip on its 8 s countdown → 409 conflict → infinite loop.
  // A `done` job hides the disc; from there it surfaces as "already
  // ripped, re-rip?" on the drive card (DriveHeroCard / DriveCard).
  $: discsWithActiveOrDoneJob = new Set(
    $jobs
      .filter((j) => j.state === 'done' || j.state === 'running' || j.state === 'queued')
      .map((j) => j.disc_id),
  );

  // No filter on candidate count: a disc with zero candidates is still
  // an awaiting decision — it just means MusicBrainz / TMDB didn't
  // recognise it, and the card needs to surface that to the user
  // (otherwise the dashboard silently swallows the insert and the
  // drive just snaps back to idle). The card itself renders the right
  // copy + affordances for zero-candidate discs based on disc.type.
  $: pending = Object.values($discs)
    .filter((d: Disc) => !discsWithActiveOrDoneJob.has(d.id))
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
