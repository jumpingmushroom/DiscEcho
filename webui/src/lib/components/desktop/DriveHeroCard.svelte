<script lang="ts">
  import type { Drive, Disc, Job } from '$lib/wire';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import PipelineStepperMini from '$lib/components/PipelineStepperMini.svelte';
  import ProgressBar from '$lib/components/ProgressBar.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import { ripSubStepLabel } from '$lib/ripSubStepLabel';
  import { createEventDispatcher } from 'svelte';
  import { cancelJob, ejectDrive, reidentify, jobs, startDisc } from '$lib/store';
  import { lastDoneJobForDisc } from '$lib/components/lastDoneJobForDisc';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job | undefined = undefined;
  export let queuedCount: number = 0;

  const dispatch = createEventDispatcher<{ select: string | null }>();

  $: stateColour = drive.state === 'idle' ? 'var(--text-3)' : 'var(--accent)';

  $: hasActiveJob = !!job && (drive.state === 'ripping' || drive.state === 'identifying');
  $: canStop = hasActiveJob && !!job;
  $: canEject = !hasActiveJob && drive.state !== 'ejecting';
  $: canReidentify = !!disc && !hasActiveJob && drive.state === 'idle';

  // Most recent done job for the currently-inserted disc. Drives the
  // "already ripped, re-rip?" affordance below.
  $: lastDoneJob = lastDoneJobForDisc($jobs, disc?.id);
  $: canRerip = drive.state === 'idle' && !!disc && !!lastDoneJob && !hasActiveJob;

  let busy: 'cancel' | 'eject' | 'reid' | 'rerip' | null = null;
  let errMsg = '';

  // Caption shown below the model name when no disc is bound. The
  // disc-bound branch renders the disc title instead; the "already
  // ripped" caption lives there too, gated on canRerip.
  $: caption = (() => {
    switch (drive.state) {
      case 'ripping':
        return 'Ripping disc…';
      case 'identifying':
        return 'Identifying disc…';
      case 'error':
        return 'Drive error — see logs';
      case 'idle':
      default:
        return 'Idle — insert a disc';
    }
  })();

  $: rerippedCaption = lastDoneJob
    ? `Already ripped${lastDoneJob.finished_at ? ' ' + lastDoneJob.finished_at.slice(0, 10) : ''} — re-rip?`
    : '';

  async function onStop(): Promise<void> {
    if (!job) return;
    if (!confirm('Stop the running rip? Partial output may be left behind.')) return;
    busy = 'cancel';
    errMsg = '';
    try {
      await cancelJob(job.id);
    } catch (e) {
      errMsg = (e as Error).message;
    } finally {
      busy = null;
    }
  }

  async function onEject(): Promise<void> {
    busy = 'eject';
    errMsg = '';
    try {
      await ejectDrive(drive.id);
    } catch (e) {
      errMsg = (e as Error).message;
    } finally {
      busy = null;
    }
  }

  async function onReidentify(): Promise<void> {
    if (!disc) return;
    busy = 'reid';
    errMsg = '';
    try {
      await reidentify(disc.id);
    } catch (e) {
      errMsg = (e as Error).message;
    } finally {
      busy = null;
    }
  }

  async function onRerip(): Promise<void> {
    if (!disc || !lastDoneJob) return;
    busy = 'rerip';
    errMsg = '';
    try {
      await startDisc(disc.id, lastDoneJob.profile_id, 0);
    } catch (e) {
      errMsg = (e as Error).message;
    } finally {
      busy = null;
    }
  }

  $: activeStepLabel = (() => {
    if (job?.active_step === 'rip') {
      return `Rip — ${ripSubStepLabel(job?.active_substep)}`;
    }
    return '';
  })();
</script>

<div
  class="rounded-2xl border border-border bg-surface-1 p-4 transition-colors hover:border-border-strong"
>
  <button class="w-full text-left" on:click={() => dispatch('select', job?.id ?? null)}>
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
          {drive.bus}
        </div>
        <div class="mt-1 truncate text-[14px] font-semibold text-text">{drive.model}</div>
      </div>
      <div class="flex flex-col items-end gap-1">
        <div
          class="text-[10px] font-medium uppercase tracking-[0.14em]"
          style="color: {stateColour}"
        >
          {drive.state}
        </div>
        {#if queuedCount > 0}
          <span
            class="rounded px-1 py-0.5 font-mono text-[10px] tracking-[0.14em]"
            style="background: var(--surface-2); color: var(--text-3)"
          >
            +{queuedCount} queued
          </span>
        {/if}
      </div>
    </div>

    {#if disc}
      <div class="mt-3 flex flex-wrap items-center gap-2">
        <DiscTypeBadge type={disc.type} />
        <span class="truncate text-[13px] text-text-2">
          {disc.title || disc.candidates?.[0]?.title || disc.id.slice(0, 8)}
        </span>
      </div>
      {#if canRerip}
        <div class="mt-2 text-[12px]" style="color: var(--text-3)">
          {rerippedCaption}
        </div>
      {/if}
    {:else}
      <div
        class="mt-3 text-[12px]"
        style="color: {drive.state === 'error' ? 'var(--error)' : 'var(--text-3)'}"
      >
        {caption}
      </div>
    {/if}

    {#if drive.last_error}
      <div class="mt-2 rounded-lg border border-error/30 bg-error/10 p-3 text-[12px]">
        <div class="font-semibold text-error">Drive error</div>
        <div class="mt-1 text-text-2">{drive.last_error}</div>
        {#if drive.last_error_tip}
          <div class="mt-2 text-text-3">
            <span class="font-semibold">Tip:</span>
            {drive.last_error_tip}
          </div>
        {/if}
      </div>
    {/if}

    {#if job && (drive.state === 'ripping' || drive.state === 'identifying')}
      <div class="mt-3 space-y-2">
        <PipelineStepperMini {job} />
        {#if activeStepLabel}
          <div class="text-[11px]" style="color: var(--text-3)" data-testid="active-step-label">
            {activeStepLabel}
          </div>
        {/if}
        <ProgressBar value={job.progress} height={4} animated />
        <div class="flex items-center justify-between">
          <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
          <span class="font-mono text-[12px] font-semibold text-accent">
            {Math.round(job.progress)}%
          </span>
        </div>
      </div>
    {/if}
  </button>

  {#if canStop || canEject || canReidentify || canRerip}
    <div class="mt-3 flex flex-wrap gap-2 border-t border-border pt-3">
      {#if canStop}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-warn disabled:opacity-50"
          on:click|stopPropagation={onStop}
          disabled={busy !== null}
          data-testid="drive-stop"
        >
          {busy === 'cancel' ? 'Stopping…' : 'Stop'}
        </button>
      {/if}
      {#if canReidentify}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-text-2 disabled:opacity-50"
          on:click|stopPropagation={onReidentify}
          disabled={busy !== null}
          data-testid="drive-reidentify"
        >
          {busy === 'reid' ? 'Re-identifying…' : 'Re-identify'}
        </button>
      {/if}
      {#if canRerip}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-accent disabled:opacity-50"
          on:click|stopPropagation={onRerip}
          disabled={busy !== null}
          data-testid="drive-rerip"
        >
          {busy === 'rerip' ? 'Starting…' : 'Re-rip'}
        </button>
      {/if}
      {#if canEject}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-text-2 disabled:opacity-50"
          on:click|stopPropagation={onEject}
          disabled={busy !== null}
          data-testid="drive-eject"
        >
          {busy === 'eject' ? 'Ejecting…' : 'Eject'}
        </button>
      {/if}
    </div>
  {/if}
  {#if errMsg}
    <div class="mt-2 text-[11px] text-error">{errMsg}</div>
  {/if}
</div>
