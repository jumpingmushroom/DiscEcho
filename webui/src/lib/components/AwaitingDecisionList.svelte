<script lang="ts">
  import { discs, jobs } from '$lib/store';
  import type { Disc } from '$lib/wire';
  import AwaitingDecisionCard from './AwaitingDecisionCard.svelte';

  // A disc is "awaiting decision" when no prior rip *succeeded* for
  // it. Failed, cancelled, and interrupted jobs leave the disc on
  // this surface so the user is auto-prompted to retry — they clearly
  // wanted a rip and it didn't finish. Only a `done` job hides the
  // disc; from there it surfaces as "already ripped, re-rip?" on the
  // drive card when re-inserted (DriveHeroCard / mobile DriveCard).
  $: discsWithDoneRip = new Set($jobs.filter((j) => j.state === 'done').map((j) => j.disc_id));

  // No filter on candidate count: a disc with zero candidates is still
  // an awaiting decision — it just means MusicBrainz / TMDB didn't
  // recognise it, and the card needs to surface that to the user
  // (otherwise the dashboard silently swallows the insert and the
  // drive just snaps back to idle). The card itself renders the right
  // copy + affordances for zero-candidate discs based on disc.type.
  $: pending = Object.values($discs)
    .filter((d: Disc) => !discsWithDoneRip.has(d.id))
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
