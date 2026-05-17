<script lang="ts">
  import type { Drive, Disc, Job } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import SpeedEtaChip from './SpeedEtaChip.svelte';
  import { formatProgress } from '$lib/formatProgress';
  import { createEventDispatcher } from 'svelte';
  import { cancelJob, ejectDrive, reidentify } from '$lib/store';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job | undefined = undefined;
  export let queuedCount: number = 0;

  const dispatch = createEventDispatcher<{ click: void }>();

  let busy: 'cancel' | 'eject' | 'reid' | null = null;
  let errMsg = '';

  $: hasActiveJob = !!job && (drive.state === 'ripping' || drive.state === 'identifying');
  $: canStop = hasActiveJob && !!job;
  $: canEject = !hasActiveJob && drive.state !== 'ejecting';
  // Re-identify only makes sense when a disc is present, the drive is
  // idle, and the disc carries some identification we might disagree
  // with. Skip discs (where disc.id is gone) won't match.
  $: canReidentify = !!disc && !hasActiveJob && drive.state === 'idle';

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
</script>

<div class="rounded-2xl border border-border bg-surface-1 p-4">
  <button class="w-full text-left transition-colors" on:click={() => dispatch('click')}>
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
          {drive.bus}
        </div>
        <div class="mt-1 truncate text-[15px] font-semibold text-text">{drive.model}</div>
        {#if disc}
          <div class="mt-2 flex flex-wrap items-center gap-2">
            <DiscTypeBadge type={disc.type} />
            <span class="truncate text-[12px] text-text-2">{disc.title || 'Unknown disc'}</span>
          </div>
        {:else}
          <div class="mt-2 text-[12px] text-text-3">Idle</div>
        {/if}
      </div>
      <div class="flex flex-col items-end gap-1">
        <div
          class="text-[10px] font-medium uppercase tracking-[0.14em]"
          style="color: {drive.state === 'idle' ? 'var(--text-3)' : 'var(--accent)'}"
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

    {#if job && (drive.state === 'ripping' || drive.state === 'identifying')}
      <div class="mt-3 space-y-2">
        <ProgressBar value={job.progress} height={4} animated />
        <div class="flex items-center justify-between">
          <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
          <span class="font-mono text-[12px] font-semibold text-accent">
            {formatProgress(job.progress)}
          </span>
        </div>
      </div>
    {/if}
  </button>

  {#if canStop || canEject || canReidentify}
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
