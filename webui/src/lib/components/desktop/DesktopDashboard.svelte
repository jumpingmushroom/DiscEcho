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
  // The detail panel only renders when the user has explicitly opted in
  // via a queue-row click. No fallback to "first running job" — that
  // duplicated the hero RipCard for the most common case (single rip).
  $: selectedJob = $selectedJobID ? $jobs.find((j) => j.id === $selectedJobID) : undefined;

  function toggleSelected(id: string): void {
    selectedJobID.update((cur) => (cur === id ? null : id));
  }
</script>

<div class="mx-auto min-h-screen max-w-screen-2xl p-6">
  <!-- Hero band — idle drives render as DriveHeroCard; busy drives
       swap to RipCard. A busy drive whose active job is selected in
       the sidebar collapses entirely from the hero band to avoid the
       same RipCard rendering twice on screen. -->
  <div class="mb-6 grid gap-4" style="grid-template-columns: repeat(auto-fit, minmax(280px, 1fr))">
    {#each $drives as d (d.id)}
      {@const activeJob = activeJobs.find((j) => j.drive_id === d.id && j.state !== 'queued')}
      {@const discID = d.current_disc_id ?? activeJob?.disc_id}
      {@const disc = discID ? $discs[discID] : undefined}
      {#if activeJob && d.state !== 'idle' && $selectedJobID === activeJob.id}
        <!-- Hero RipCard collapsed: this drive's job is in the sidebar. -->
      {:else if activeJob && d.state !== 'idle'}
        {@const profile = $profiles.find((p) => p.id === activeJob.profile_id)}
        <div data-drive-id={d.id}>
          <RipCard drive={d} {disc} job={activeJob} {profile} />
        </div>
      {:else}
        <DriveHeroCard
          drive={d}
          {disc}
          job={activeJob}
          queuedCount={queuedByDrive[d.id] ?? 0}
          on:select={(e) => e.detail && toggleSelected(e.detail)}
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

  <!-- Queue + detail. Two-column layout only when a job is selected;
       otherwise the queue takes the full width. -->
  {#if selectedJob}
    <div class="grid gap-6" style="grid-template-columns: 1fr 360px">
      <QueueTable
        jobs={orderedJobs}
        selectedJobID={$selectedJobID}
        on:select={(e) => toggleSelected(e.detail)}
      />
      <div class="sticky top-20 self-start">
        <JobDetailPanel job={selectedJob} />
      </div>
    </div>
  {:else}
    <QueueTable
      jobs={orderedJobs}
      selectedJobID={null}
      on:select={(e) => toggleSelected(e.detail)}
    />
  {/if}
</div>
