<script lang="ts">
  import { goto } from '$app/navigation';
  import { drives, jobs, discs, liveStatus } from '$lib/store';
  import AppBar from '$lib/components/AppBar.svelte';
  import TabBar from '$lib/components/TabBar.svelte';
  import LiveDot from '$lib/components/LiveDot.svelte';
  import DriveCard from '$lib/components/DriveCard.svelte';
  import JobRow from '$lib/components/JobRow.svelte';
  import AwaitingDecisionList from '$lib/components/AwaitingDecisionList.svelte';
  import type { Drive, Job } from '$lib/wire';

  const TERMINAL_STATES: ReadonlyArray<Job['state']> = [
    'done',
    'failed',
    'cancelled',
    'interrupted',
  ];

  $: activeJobs = $jobs.filter((j) => !TERMINAL_STATES.includes(j.state));
  $: ripping = activeJobs.filter((j) => j.state !== 'queued').length;
  $: queued = activeJobs.filter((j) => j.state === 'queued').length;
  $: heroLine = formatHeroLine(ripping, queued);
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
  $: activeCount = $drives.filter((d) => d.state !== 'idle').length;

  function formatHeroLine(rip: number, q: number): string {
    if (rip === 0 && q === 0) return '0 jobs';
    if (q === 0) return `${rip} rip${rip === 1 ? '' : 's'}`;
    if (rip === 0) return `${q} queued`;
    return `${rip} rip · ${q} queued`;
  }

  function onDriveClick(drive: Drive): void {
    // identifying drives now have their own AwaitingDecisionList card
    // higher up the page — clicking the drive card just navigates to
    // the running job, when there is one.
    if (drive.state === 'ripping') {
      const j = activeJobs.find((j) => j.drive_id === drive.id && j.state !== 'queued');
      if (j) goto(`/jobs/${j.id}`);
    }
  }
</script>

<div class="min-h-screen pb-24">
  <AppBar title="Drives" subtitle="{activeCount} active">
    <div slot="right" class="flex items-center gap-2">
      <LiveDot label={$liveStatus === 'live' ? 'LIVE' : 'WAIT'} />
    </div>
  </AppBar>

  <!-- Hero band — placeholder until a metrics story lands -->
  <div class="mb-4 px-5">
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-baseline justify-between">
        <div>
          <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
            Currently ripping
          </div>
          <div class="mt-1 text-[28px] font-bold leading-none text-text">
            {ripping + queued}
            <span class="ml-2 text-[14px] font-medium text-text-3">{heroLine}</span>
          </div>
        </div>
        <div class="text-right">
          <div class="text-[11px] text-text-3">today</div>
          <div class="font-mono text-[15px] font-semibold text-text">—</div>
        </div>
      </div>
    </div>
  </div>

  <!-- Awaiting decision -->
  <div class="mb-4 px-5">
    <AwaitingDecisionList />
  </div>

  <!-- Drives -->
  <div class="space-y-3 px-5">
    <div class="pt-2 text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
      Optical drives
    </div>
    {#each $drives as d (d.id)}
      <DriveCard
        drive={d}
        disc={d.current_disc_id ? $discs[d.current_disc_id] : undefined}
        job={activeJobs.find((j) => j.drive_id === d.id && j.state !== 'queued')}
        queuedCount={queuedByDrive[d.id] ?? 0}
        on:click={() => onDriveClick(d)}
      />
    {:else}
      <div class="rounded-2xl border border-dashed border-border p-4 text-center text-text-3">
        No drives detected.
      </div>
    {/each}
  </div>

  <!-- Active queue -->
  <div class="mt-6 space-y-3 px-5">
    <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Active queue</div>
    {#if orderedJobs.length === 0}
      <div class="rounded-2xl border border-dashed border-border p-4 text-center text-text-3">
        No active jobs.
      </div>
    {:else}
      <div class="overflow-hidden rounded-2xl border border-border bg-surface-1">
        {#each orderedJobs as j (j.id)}
          <JobRow job={j} on:click={() => goto(`/jobs/${j.id}`)} />
        {/each}
      </div>
    {/if}
  </div>
</div>

<TabBar active="dashboard" />
