<script lang="ts">
  import type { Drive, Disc, Job, Profile, StepID } from '$lib/wire';
  import DiscArt from '$lib/components/DiscArt.svelte';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import ProgressBar from '$lib/components/ProgressBar.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import { logs } from '$lib/store';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job | undefined = undefined;
  export let profile: Profile | undefined = undefined;
  export let queuedCount: number = 0;
  export let href: string | undefined = undefined;

  // Last few log lines while ripping — gives the dashboard real
  // signal during whipper's startup phase when progress sits at 0%
  // for minutes. Limited to 3 lines so the card stays compact.
  const LOG_TAIL_LINES = 3;
  $: tail = job ? ($logs[job.id] ?? []).slice(-LOG_TAIL_LINES) : [];

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

<svelte:element
  this={href ? 'a' : 'div'}
  {href}
  data-sveltekit-preload-data={href ? 'hover' : undefined}
  class="block min-h-[44px] rounded-2xl border border-border bg-surface-1 p-3 transition-colors hover:border-border-strong"
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
    <span
      class="shrink-0 rounded px-2 py-0.5 font-bold uppercase tracking-[0.14em]"
      style="font-size: 10px; background: {busy
        ? 'var(--accent-soft)'
        : 'var(--surface-2)'}; color: {busy ? 'var(--accent)' : 'var(--text-3)'}"
    >
      {stateLabel(job, drive)}
    </span>
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
        {disc ? discTitle(disc) : 'No disc inserted'}
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
