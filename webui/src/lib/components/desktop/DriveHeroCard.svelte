<script lang="ts">
  import type { Drive, Disc, Job } from '$lib/wire';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import PipelineStepperMini from '$lib/components/PipelineStepperMini.svelte';
  import ProgressBar from '$lib/components/ProgressBar.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import { createEventDispatcher } from 'svelte';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job | undefined = undefined;
  export let queuedCount: number = 0;

  const dispatch = createEventDispatcher<{ select: string | null }>();

  $: stateColour = drive.state === 'idle' ? 'var(--text-3)' : 'var(--accent)';

  // Caption shown below the model name. The disc-bound path always wins;
  // otherwise we follow drive.state so the card never lies and says
  // "Idle" while the daemon has flipped to ripping/identifying.
  function captionFor(state: Drive['state']): string {
    switch (state) {
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
  }
</script>

<button
  class="w-full rounded-2xl border border-border bg-surface-1 p-4 text-left
         transition-colors hover:border-border-strong"
  on:click={() => dispatch('select', job?.id ?? null)}
>
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0 flex-1">
      <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
        {drive.bus}
      </div>
      <div class="mt-1 truncate text-[14px] font-semibold text-text">{drive.model}</div>
    </div>
    <div class="flex flex-col items-end gap-1">
      <div class="text-[10px] font-medium uppercase tracking-[0.14em]" style="color: {stateColour}">
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
  {:else}
    <div
      class="mt-3 text-[12px]"
      style="color: {drive.state === 'error' ? 'var(--error)' : 'var(--text-3)'}"
    >
      {captionFor(drive.state)}
    </div>
  {/if}

  {#if job && (drive.state === 'ripping' || drive.state === 'identifying')}
    <div class="mt-3 space-y-2">
      <PipelineStepperMini {job} />
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
