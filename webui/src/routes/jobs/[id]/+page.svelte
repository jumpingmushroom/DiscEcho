<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { jobs, discs, profiles, cancelJob, deleteJob, fetchJob } from '$lib/store';
  import LiveDot from '$lib/components/LiveDot.svelte';
  import DiscArt from '$lib/components/DiscArt.svelte';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import PipelineStepperVertical from '$lib/components/PipelineStepperVertical.svelte';
  import KVRow from '$lib/components/KVRow.svelte';
  import LogPhaseViewer from '$lib/components/LogPhaseViewer.svelte';
  import { formatBytes } from '$lib/format';
  import { formatDuration } from '$lib/time';
  import type { Job } from '$lib/wire';

  $: id = $page.params.id;

  // localJob backs the terminal-state load path: when the requested
  // id isn't in the live $jobs snapshot (because it's done and aged
  // out), we fetch it from the daemon and stash it here. The reactive
  // $job below prefers the live entry — that's important during a
  // running job that's also in $jobs, so step transitions update the UI.
  let localJob: Job | undefined = undefined;
  let loading = false;
  let loadError: string | null = null;

  $: liveJob = $jobs.find((j) => j.id === id);
  $: job = liveJob ?? localJob;
  $: disc = job ? $discs[job.disc_id] : undefined;
  $: isTerminal =
    job !== undefined &&
    (['done', 'failed', 'cancelled', 'interrupted'] as const).includes(job.state as never);
  $: profileName =
    job !== undefined ? ($profiles.find((p) => p.id === job.profile_id)?.name ?? '') : '';
  $: outcomeLabel =
    job?.state === 'done'
      ? 'DONE'
      : job?.state === 'failed'
        ? 'FAILED'
        : job?.state === 'cancelled'
          ? 'CANCELLED'
          : 'INTERRUPTED';

  type Tab = 'pipeline' | 'log' | 'summary';
  // Default to Pipeline while a job is alive (most operational view); flip
  // to Summary once it terminates (most useful post-mortem view). Keyed
  // off isTerminal but only on first compute so user navigation between
  // tabs isn't clobbered when SSE flips the job to terminal.
  let tab: Tab | null = null;
  $: if (tab === null && job) tab = isTerminal ? 'summary' : 'pipeline';

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: 'pipeline', label: 'Pipeline' },
    { id: 'log', label: 'Log' },
    { id: 'summary', label: 'Summary' },
  ];

  async function loadFromDaemon(jobID: string): Promise<void> {
    loading = true;
    loadError = null;
    try {
      const res = await fetchJob(jobID);
      localJob = res.job;
    } catch (e) {
      loadError = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  // If the requested id never appears in the live snapshot, fall back
  // to the daemon's GET /api/jobs/:id (which also returns the disc).
  // We only do this once per id and only when the live store doesn't
  // already cover it — a running job streams via SSE and doesn't need
  // a one-shot fetch.
  $: if (id && !liveJob && !localJob && !loading && !loadError) {
    void loadFromDaemon(id);
  }

  async function onCancel(): Promise<void> {
    if (!job) return;
    if (!confirm('Cancel this job?')) return;
    await cancelJob(job.id);
    goto('/');
  }

  async function onDelete(): Promise<void> {
    if (!job) return;
    if (!confirm('Delete this entry from history? Files on disk are not affected.')) return;
    try {
      await deleteJob(job.id);
      goto('/history');
    } catch (e) {
      alert('Delete failed: ' + (e as Error).message);
    }
  }

  // Force-finalize active_step so PipelineStepperVertical paints the
  // terminal-step state instead of leaving the last running step
  // visually "in progress" on a terminal job. We don't mutate the
  // job — just pass a derived copy to the stepper.
  $: terminalJob = job && isTerminal ? { ...job, active_step: undefined } : job;

  onMount(() => {
    // No-op; loading is reactive on id. Kept for parity with sibling routes.
  });
</script>

<div class="min-h-screen pb-32">
  <!-- header -->
  <div
    class="sticky top-0 z-20 flex items-center justify-between border-b border-border px-3 backdrop-blur"
    style="background: rgba(5,5,5,0.92); padding-top: calc(env(safe-area-inset-top, 0px) + 12px); padding-bottom: 12px"
  >
    <button
      class="flex min-h-[44px] min-w-[44px] items-center justify-center text-text-2"
      on:click={() => goto(isTerminal ? '/history' : '/')}
      aria-label="Back"
    >
      ←
    </button>
    <div class="flex items-center gap-2">
      {#if disc}<DiscTypeBadge type={disc.type} />{/if}
      <span class="lg:hidden">
        <LiveDot />
      </span>
    </div>
    <div class="w-11"></div>
  </div>

  {#if loading && !job}
    <div class="px-5 py-12 text-center text-text-3">Loading…</div>
  {:else if !job}
    <div class="px-5 py-12 text-center text-text-3">
      {loadError ? `Couldn't load job: ${loadError}` : 'Job not found.'}
    </div>
  {:else}
    <!-- hero -->
    <div class="flex gap-4 border-b border-border px-5 pb-5 pt-4">
      <DiscArt {disc} size={96} ratio={disc?.type === 'AUDIO_CD' ? 'square' : 'portrait'} />
      <div class="min-w-0 flex-1">
        <div
          class="font-medium uppercase tracking-[0.14em] text-text-3"
          style="font-size: var(--ts-overline)"
        >
          Job {job.id.slice(0, 8)}…
        </div>
        <div class="mt-1 font-bold leading-tight text-text" style="font-size: var(--ts-display)">
          {disc?.title || 'Unknown disc'}
        </div>
        {#if disc?.year || profileName}
          <div class="mt-1 text-text-3" style="font-size: var(--ts-meta)">
            {disc?.year ?? ''}{disc?.year && profileName ? ' · ' : ''}{profileName}
          </div>
        {/if}
        {#if isTerminal}
          <div class="mt-3 flex items-center gap-2">
            <span
              class="rounded px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
              style={job.state === 'failed'
                ? 'background: rgba(255,91,91,0.15); color: var(--error)'
                : job.state === 'cancelled' || job.state === 'interrupted'
                  ? 'background: var(--surface-2); color: var(--text-3)'
                  : 'background: var(--accent-soft); color: var(--accent)'}
            >
              {outcomeLabel}
            </span>
          </div>
        {:else}
          <div class="mt-2">
            <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
          </div>
        {/if}
      </div>
    </div>

    <!-- tabs -->
    <div
      class="sticky z-10 flex border-b border-border px-3 backdrop-blur"
      style="top: calc(env(safe-area-inset-top, 0px) + 56px); background: rgba(5,5,5,0.92)"
    >
      {#each tabs as t (t.id)}
        <button
          type="button"
          class="tab flex-1 border-b-2 px-3 py-3 font-medium"
          class:active={tab === t.id}
          style="font-size: var(--ts-body)"
          on:click={() => (tab = t.id)}
        >
          {t.label}
        </button>
      {/each}
    </div>

    {#if tab === 'pipeline'}
      <div class="px-2 pb-2 pt-3">
        <PipelineStepperVertical job={terminalJob ?? job} />
      </div>
    {:else if tab === 'log'}
      <div class="px-3 pt-3">
        <LogPhaseViewer
          jobID={job.id}
          live={!isTerminal}
          activeStep={job.active_step}
          defaultStep=""
        />
      </div>
    {:else if tab === 'summary'}
      <div class="space-y-4 px-5 pt-3">
        {#if isTerminal}
          <div class="divide-y divide-border rounded-2xl border border-border bg-surface-1">
            {#if job.finished_at}
              <KVRow label="Finished">{new Date(job.finished_at).toLocaleString()}</KVRow>
            {/if}
            {#if job.elapsed_seconds}
              <KVRow label="Elapsed">{formatDuration(job.elapsed_seconds)}</KVRow>
            {/if}
            {#if job.output_bytes}
              <KVRow label="Output size">{formatBytes(job.output_bytes)}</KVRow>
            {/if}
            {#if profileName}
              <KVRow label="Profile">{profileName}</KVRow>
            {/if}
          </div>

          {#if job.error_message}
            <div
              class="rounded-2xl border px-4 py-3"
              style="border-color: rgba(255,91,91,0.3); background: rgba(255,91,91,0.1)"
            >
              <div
                class="font-medium uppercase tracking-[0.14em]"
                style="font-size: var(--ts-meta); color: var(--error)"
              >
                Error
              </div>
              <div class="mt-1 break-words font-mono text-text-2" style="font-size: var(--ts-meta)">
                {job.error_message}
              </div>
            </div>
          {/if}
        {:else}
          <div class="divide-y divide-border rounded-2xl border border-border bg-surface-1">
            <KVRow label="State">{job.state}</KVRow>
            <KVRow label="Progress">{Math.round(job.progress)}%</KVRow>
            {#if job.active_step}
              <KVRow label="Active step">{job.active_step}</KVRow>
            {/if}
            {#if profileName}
              <KVRow label="Profile">{profileName}</KVRow>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    <!-- sticky actions -->
    <div
      class="fixed bottom-0 left-0 right-0 z-30 flex items-center gap-2 border-t border-border bg-bg px-3 py-3"
      style="padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 12px)"
    >
      {#if isTerminal}
        <button
          class="min-h-[44px] flex-1 rounded-xl border px-4"
          style="border-color: var(--error); color: var(--error); background: rgba(255,91,91,0.08); font-size: var(--ts-body)"
          on:click={onDelete}
        >
          Delete from history
        </button>
      {:else}
        <button
          class="min-h-[44px] rounded-xl border px-4"
          style="border-color: var(--error); color: var(--error); background: rgba(255,91,91,0.08); font-size: var(--ts-body)"
          on:click={onCancel}
        >
          Cancel
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .tab {
    color: var(--text-3);
    border-color: transparent;
  }
  .tab.active {
    color: var(--accent);
    border-color: var(--accent);
  }
</style>
