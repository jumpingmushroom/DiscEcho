<script lang="ts">
  import { goto } from '$app/navigation';
  import { drives, jobs, discs, pendingDiscID, liveStatus } from '$lib/store';
  import AppBar from '$lib/components/AppBar.svelte';
  import TabBar from '$lib/components/TabBar.svelte';
  import LiveDot from '$lib/components/LiveDot.svelte';
  import DriveCard from '$lib/components/DriveCard.svelte';
  import JobRow from '$lib/components/JobRow.svelte';
  import DiscIdSheet from '$lib/components/DiscIdSheet.svelte';
  import type { Drive } from '$lib/wire';

  $: activeJobs = $jobs.filter(
    (j) => !['done', 'failed', 'cancelled', 'interrupted'].includes(j.state),
  );
  $: activeCount = $drives.filter((d) => d.state !== 'idle').length;
  $: pendingDisc = $pendingDiscID ? $discs[$pendingDiscID] : null;

  function onDriveClick(drive: Drive): void {
    if (drive.state === 'identifying' && drive.current_disc_id) {
      pendingDiscID.set(drive.current_disc_id);
      return;
    }
    if (drive.state === 'ripping') {
      const j = activeJobs.find((j) => j.drive_id === drive.id);
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

  <!-- Hero band — placeholder until M2 ships history metrics -->
  <div class="mb-4 px-5">
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-baseline justify-between">
        <div>
          <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
            Currently ripping
          </div>
          <div class="mt-1 text-[28px] font-bold leading-none text-text">
            {activeJobs.length}
            <span class="ml-2 text-[14px] font-medium text-text-3">jobs</span>
          </div>
        </div>
        <div class="text-right">
          <div class="text-[11px] text-text-3">today</div>
          <div class="font-mono text-[15px] font-semibold text-text">—</div>
        </div>
      </div>
    </div>
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
        job={activeJobs.find((j) => j.drive_id === d.id)}
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
    {#if activeJobs.length === 0}
      <div class="rounded-2xl border border-dashed border-border p-4 text-center text-text-3">
        No active jobs.
      </div>
    {:else}
      <div class="overflow-hidden rounded-2xl border border-border bg-surface-1">
        {#each activeJobs as j (j.id)}
          <JobRow job={j} on:click={() => goto(`/jobs/${j.id}`)} />
        {/each}
      </div>
    {/if}
  </div>
</div>

{#if pendingDisc}
  <DiscIdSheet disc={pendingDisc} />
{/if}

<TabBar active="dashboard" />
