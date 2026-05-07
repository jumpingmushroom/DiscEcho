<script lang="ts">
  import type { Drive, Disc, Job } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import SpeedEtaChip from './SpeedEtaChip.svelte';
  import { createEventDispatcher } from 'svelte';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job | undefined = undefined;
  export let queuedCount: number = 0;

  const dispatch = createEventDispatcher<{ click: void }>();
</script>

<button
  class="w-full min-h-[44px] rounded-2xl border border-border bg-surface-1 p-4 text-left
         transition-colors hover:border-border-strong"
  on:click={() => dispatch('click')}
>
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0 flex-1">
      <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">{drive.bus}</div>
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
          {Math.round(job.progress)}%
        </span>
      </div>
    </div>
  {/if}
</button>
