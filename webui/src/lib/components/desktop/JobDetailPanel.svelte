<script lang="ts">
  import type { Job } from '$lib/wire';
  import { discs, drives, profiles } from '$lib/store';
  import RipCard from '$lib/components/RipCard.svelte';

  export let job: Job | undefined = undefined;

  $: disc = job ? $discs[job.disc_id] : undefined;
  $: drive = job?.drive_id ? $drives.find((d) => d.id === job.drive_id) : undefined;
  $: profile = job ? $profiles.find((p) => p.id === job.profile_id) : undefined;
</script>

{#if !job}
  <div class="rounded-2xl border border-border bg-surface-1 p-5">
    <div class="py-12 text-center text-[13px] text-text-3">
      Click a drive or queue row to inspect a job.
    </div>
  </div>
{:else if drive}
  <RipCard {drive} {disc} {job} {profile} />
{:else}
  <!-- Job lost its drive (drive removed mid-rip). Render a minimal card
       so the sidebar isn't blank — RipCard requires a drive prop. -->
  <div class="rounded-2xl border border-border bg-surface-1 p-5">
    <div class="text-[13px] text-text-3">
      Job {job.id.slice(0, 8)} — drive no longer connected.
    </div>
  </div>
{/if}
