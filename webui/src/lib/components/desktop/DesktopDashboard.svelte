<script lang="ts">
  import { drives, jobs, discs, profiles, selectedJobID } from '$lib/store';
  import DriveHeroCard from './DriveHeroCard.svelte';
  import QueueTable from './QueueTable.svelte';
  import JobDetailPanel from './JobDetailPanel.svelte';
  import RipCard from '$lib/components/RipCard.svelte';
  import AwaitingDecisionList from '../AwaitingDecisionList.svelte';
  import type { Job } from '$lib/wire';

  const TERMINAL_STATES: ReadonlyArray<Job['state']> = [
    'done',
    'failed',
    'cancelled',
    'interrupted',
  ];
  const TERMINAL_OR_QUEUED: ReadonlyArray<Job['state']> = [...TERMINAL_STATES, 'queued'];

  $: activeJobs = $jobs.filter((j) => !TERMINAL_STATES.includes(j.state));
  $: orderedJobs = [...activeJobs].sort((a, b) => {
    const aQ = a.state === 'queued' ? 1 : 0;
    const bQ = b.state === 'queued' ? 1 : 0;
    return aQ - bQ;
  });
  $: queuedByDrive = $jobs.reduce<Record<string, number>>((acc, j) => {
    if (j.state === 'queued' && j.drive_id) {
      acc[j.drive_id] = (acc[j.drive_id] ?? 0) + 1;
    }
    return acc;
  }, {});
  $: effectiveJob = (() => {
    if ($selectedJobID) return $jobs.find((j) => j.id === $selectedJobID);
    return $jobs.find((j) => !TERMINAL_OR_QUEUED.includes(j.state));
  })();
</script>

<div class="mx-auto min-h-screen max-w-screen-2xl p-6">
  <!-- Hero band — idle drives render as DriveHeroCard; busy drives swap
       to RipCard so the running rip surfaces in one place, not two. -->
  <div class="mb-6 grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(280px, 1fr))">
    {#each $drives as d (d.id)}
      {@const activeJob = activeJobs.find((j) => j.drive_id === d.id && j.state !== 'queued')}
      {@const discID = d.current_disc_id ?? activeJob?.disc_id}
      {@const disc = discID ? $discs[discID] : undefined}
      {#if activeJob && d.state !== 'idle'}
        {@const profile = $profiles.find((p) => p.id === activeJob.profile_id)}
        <button type="button" class="text-left" on:click={() => selectedJobID.set(activeJob.id)}>
          <RipCard drive={d} {disc} job={activeJob} {profile} />
        </button>
      {:else}
        <DriveHeroCard
          drive={d}
          {disc}
          job={activeJob}
          queuedCount={queuedByDrive[d.id] ?? 0}
          on:select={(e) => selectedJobID.set(e.detail)}
        />
      {/if}
    {:else}
      <div class="rounded-2xl border border-dashed border-border p-6 text-center text-text-3">
        No drives detected.
      </div>
    {/each}
  </div>

  <!-- Awaiting decision -->
  <div class="mb-6">
    <AwaitingDecisionList />
  </div>

  <!-- Queue + detail -->
  <div class="grid gap-6" style="grid-template-columns: 1fr 360px">
    <QueueTable
      jobs={orderedJobs}
      selectedJobID={$selectedJobID}
      on:select={(e) => selectedJobID.set(e.detail)}
    />
    <div class="sticky top-20 self-start">
      <JobDetailPanel job={effectiveJob} />
    </div>
  </div>
</div>
