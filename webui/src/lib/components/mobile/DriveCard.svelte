<script lang="ts">
  import type { Drive, Disc, Job, Profile, StepID } from '$lib/wire';
  import DiscArt from '$lib/components/DiscArt.svelte';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import ProgressBar from '$lib/components/ProgressBar.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import { onMount, onDestroy } from 'svelte';
  import {
    logs,
    ensureLogBackfill,
    cancelJob,
    ejectDrive,
    reidentify,
    jobs,
    startDisc,
  } from '$lib/store';
  import { lastDoneJobForDisc } from '$lib/components/lastDoneJobForDisc';
  import { formatDuration } from '$lib/time';
  import { trackSummary } from '$lib/format';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job | undefined = undefined;
  export let profile: Profile | undefined = undefined;
  export let queuedCount: number = 0;
  export let href: string | undefined = undefined;

  $: hasActiveJob = !!job && (drive.state === 'ripping' || drive.state === 'identifying');
  $: canStop = hasActiveJob && !!job;
  $: canEject = !hasActiveJob && drive.state !== 'ejecting';
  $: canReidentify = !!disc && !hasActiveJob && drive.state === 'idle';

  // Most recent done job for the currently-inserted disc. Drives the
  // "already ripped, re-rip?" affordance below.
  $: lastDoneJob = lastDoneJobForDisc($jobs, disc?.id);
  $: canRerip = drive.state === 'idle' && !!disc && !!lastDoneJob && !hasActiveJob;
  $: hasActions = canStop || canEject || canReidentify || canRerip;

  $: rerippedCaption = lastDoneJob
    ? `Already ripped${lastDoneJob.finished_at ? ' ' + lastDoneJob.finished_at.slice(0, 10) : ''} — re-rip?`
    : '';

  let actionBusy: 'cancel' | 'eject' | 'reid' | 'rerip' | null = null;
  let actionErr = '';

  async function onStop(): Promise<void> {
    if (!job) return;
    if (!confirm('Stop the running rip? Partial output may be left behind.')) return;
    actionBusy = 'cancel';
    actionErr = '';
    try {
      await cancelJob(job.id);
    } catch (e) {
      actionErr = (e as Error).message;
    } finally {
      actionBusy = null;
    }
  }

  async function onEject(): Promise<void> {
    actionBusy = 'eject';
    actionErr = '';
    try {
      await ejectDrive(drive.id);
    } catch (e) {
      actionErr = (e as Error).message;
    } finally {
      actionBusy = null;
    }
  }

  async function onReidentify(): Promise<void> {
    if (!disc) return;
    actionBusy = 'reid';
    actionErr = '';
    try {
      await reidentify(disc.id);
    } catch (e) {
      actionErr = (e as Error).message;
    } finally {
      actionBusy = null;
    }
  }

  async function onRerip(): Promise<void> {
    if (!disc || !lastDoneJob) return;
    actionBusy = 'rerip';
    actionErr = '';
    try {
      await startDisc(disc.id, lastDoneJob.profile_id, 0);
    } catch (e) {
      actionErr = (e as Error).message;
    } finally {
      actionBusy = null;
    }
  }

  // Last few log lines while ripping — gives the dashboard real
  // signal during whipper's startup phase when progress sits at 0%
  // for minutes. Limited to 3 lines so the card stays compact.
  const LOG_TAIL_LINES = 3;
  $: tail = job ? ($logs[job.id] ?? []).slice(-LOG_TAIL_LINES) : [];

  // Same rationale as the desktop RipCard — if the page mounts after
  // the rip is already running, the tail panel would otherwise sit at
  // empty until the next SSE line.
  onMount(() => {
    if (job && job.state === 'running') {
      void ensureLogBackfill(job.id);
    }
  });

  // Elapsed ticker for the same reason it lives on desktop RipCard:
  // audio rips sit at 0% for 1-3 min during whipper warmup and the
  // user needs *some* signal that work is happening.
  let now = Date.now();
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;
  $: jobRunning = !!job && (job.state === 'running' || job.state === 'queued');
  $: if (jobRunning && elapsedTimer === null) {
    elapsedTimer = setInterval(() => {
      now = Date.now();
    }, 1000);
  } else if (!jobRunning && elapsedTimer !== null) {
    clearInterval(elapsedTimer);
    elapsedTimer = null;
  }
  onDestroy(() => {
    if (elapsedTimer !== null) clearInterval(elapsedTimer);
  });

  $: elapsedSeconds = (() => {
    if (!job?.started_at) return 0;
    const startMs = new Date(job.started_at).getTime();
    if (!Number.isFinite(startMs)) return 0;
    return Math.max(0, Math.floor((now - startMs) / 1000));
  })();

  $: tracks = trackSummary(disc?.metadata_json);

  $: busy = job !== undefined && (drive.state === 'ripping' || drive.state === 'identifying');

  // State pill — derived from active_step for the busy case so the user sees
  // "TRANSCODING" instead of generic "RIPPING" once the laser is done.
  function stateLabel(j: Job | undefined, drv: Drive): string {
    switch (j?.active_step as StepID | undefined) {
      case 'detect':
      case 'identify':
        return 'IDENTIFYING';
      case 'rip':
        return 'RIPPING';
      case 'transcode':
        return 'TRANSCODING';
      case 'compress':
        return 'COMPRESSING';
      case 'move':
        return 'MOVING';
      case 'notify':
        return 'NOTIFYING';
      case 'eject':
        return 'EJECTING';
      default:
        return drv.state.toUpperCase();
    }
  }

  function activeStepLabel(j: Job | undefined): string {
    switch (j?.active_step as StepID | undefined) {
      case 'detect':
        return 'Detect — Drive sees disc';
      case 'identify':
        return 'Identify — Match metadata';
      case 'rip':
        return 'Rip — Read raw data';
      case 'transcode':
        return 'Transcode — Re-encode A/V';
      case 'compress':
        return 'Compress — Pack & verify';
      case 'move':
        return 'Move — Move to library';
      case 'notify':
        return 'Notify — Send notifications';
      case 'eject':
        return 'Eject — Tray release';
      default:
        return '';
    }
  }

  function discTitle(d: Disc | undefined): string {
    if (!d) return '—';
    if (d.title) return d.title;
    return d.id.slice(0, 8);
  }
</script>

<div
  class="rounded-2xl border border-border bg-surface-1 transition-colors hover:border-border-strong"
>
  <svelte:element
    this={href ? 'a' : 'div'}
    {href}
    data-sveltekit-preload-data={href ? 'hover' : undefined}
    class="block min-h-[44px] p-3"
  >
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0 flex-1">
        <div class="font-medium uppercase tracking-[0.14em] text-text-3" style="font-size: 10px">
          {drive.bus}
        </div>
        <div class="mt-0.5 truncate font-semibold text-text" style="font-size: var(--ts-body)">
          {drive.model}
        </div>
      </div>
      <div class="flex shrink-0 flex-col items-end gap-0.5">
        <span
          class="rounded px-2 py-0.5 font-bold uppercase tracking-[0.14em]"
          style="font-size: 10px; background: {busy
            ? 'var(--accent-soft)'
            : 'var(--surface-2)'}; color: {busy ? 'var(--accent)' : 'var(--text-3)'}"
        >
          {stateLabel(job, drive)}
        </span>
        {#if jobRunning && elapsedSeconds > 0}
          <span class="font-mono text-text-3" style="font-size: 10px" data-testid="elapsed">
            {formatDuration(elapsedSeconds)}
          </span>
        {/if}
      </div>
    </div>

    {#if busy && job}
      <div class="mt-3 flex gap-3">
        <DiscArt {disc} size={48} ratio={disc?.type === 'AUDIO_CD' ? 'square' : 'portrait'} />
        <div class="min-w-0 flex-1">
          {#if disc}<DiscTypeBadge type={disc.type} />{/if}
          <div class="mt-1 truncate font-semibold text-text" style="font-size: var(--ts-body)">
            {discTitle(disc)}
          </div>
          <div class="mt-0.5 text-text-3" style="font-size: var(--ts-overline)">
            {disc?.year ?? ''}{disc?.year && profile ? ' · ' : ''}{profile?.name ?? ''}
          </div>
          {#if tracks}
            <div class="mt-0.5 text-text-3" style="font-size: var(--ts-overline)">{tracks}</div>
          {/if}
        </div>
      </div>
      {#if activeStepLabel(job)}
        <div class="mt-3 text-text-2" style="font-size: var(--ts-overline)">
          {activeStepLabel(job)}
        </div>
      {/if}
      <div class="mt-2 space-y-1.5">
        <ProgressBar value={job.progress} animated />
        <div class="flex items-center justify-between">
          <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
          <span class="font-mono font-semibold text-accent" style="font-size: var(--ts-meta)">
            {Math.round(job.progress)}%
          </span>
        </div>
      </div>
      {#if tail.length > 0}
        <div
          class="mt-2 overflow-hidden rounded-md border border-border px-2 py-1.5 font-mono"
          style="background: var(--surface-2); font-size: 11px; line-height: 1.4"
        >
          {#each tail as line (line.t + line.message)}
            <div class="truncate text-text-3">
              <span
                class="mr-1 uppercase"
                style="color: {line.level === 'warn'
                  ? 'var(--warn)'
                  : line.level === 'error'
                    ? 'var(--error)'
                    : 'var(--text-3)'}"
              >
                {line.level === 'info' ? '·' : line.level}
              </span>
              <span class="text-text-2">{line.message}</span>
            </div>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="mt-2 flex items-center justify-between gap-2">
        <div class="text-text-3" style="font-size: var(--ts-meta)">
          {#if canRerip && disc && lastDoneJob}
            {rerippedCaption}
          {:else}
            {disc ? discTitle(disc) : 'No disc inserted'}
          {/if}
        </div>
        {#if queuedCount > 0}
          <span
            class="rounded px-1.5 py-0.5 font-mono tracking-[0.14em]"
            style="font-size: 10px; background: var(--surface-2); color: var(--text-3)"
          >
            +{queuedCount} queued
          </span>
        {/if}
      </div>
    {/if}
  </svelte:element>
  {#if drive.last_error}
    <div
      class="mx-3 mb-2 mt-1 rounded-lg border border-error/30 bg-error/10 p-3"
      style="font-size: 12px"
    >
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
  {#if hasActions}
    <div class="flex flex-wrap gap-2 border-t border-border px-3 py-2">
      {#if canStop}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-warn disabled:opacity-50"
          on:click={onStop}
          disabled={actionBusy !== null}
          data-testid="drive-stop"
        >
          {actionBusy === 'cancel' ? 'Stopping…' : 'Stop'}
        </button>
      {/if}
      {#if canReidentify}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-text-2 disabled:opacity-50"
          on:click={onReidentify}
          disabled={actionBusy !== null}
          data-testid="drive-reidentify"
        >
          {actionBusy === 'reid' ? 'Re-identifying…' : 'Re-identify'}
        </button>
      {/if}
      {#if canRerip}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-accent disabled:opacity-50"
          on:click={onRerip}
          disabled={actionBusy !== null}
          data-testid="drive-rerip"
        >
          {actionBusy === 'rerip' ? 'Starting…' : 'Re-rip'}
        </button>
      {/if}
      {#if canEject}
        <button
          class="min-h-[36px] flex-1 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-text-2 disabled:opacity-50"
          on:click={onEject}
          disabled={actionBusy !== null}
          data-testid="drive-eject"
        >
          {actionBusy === 'eject' ? 'Ejecting…' : 'Eject'}
        </button>
      {/if}
    </div>
  {/if}
  {#if actionErr}
    <div class="px-3 pb-2 text-[11px] text-error">{actionErr}</div>
  {/if}
</div>
