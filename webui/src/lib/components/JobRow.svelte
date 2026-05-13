<script lang="ts">
  import type { Job } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import PipelineStepperMini from './PipelineStepperMini.svelte';
  import { createEventDispatcher } from 'svelte';
  import { discs } from '$lib/store';
  import { formatDuration } from '$lib/time';

  export let job: Job;

  const dispatch = createEventDispatcher<{ click: void }>();

  $: disc = $discs[job.disc_id];
  $: isQueued = job.state === 'queued';
</script>

<button
  class="flex w-full min-h-[44px] items-center gap-3 border-b border-border px-4 py-3 text-left
         last:border-0 hover:bg-surface-2"
  on:click={() => dispatch('click')}
>
  {#if disc}<DiscTypeBadge type={disc.type} />{/if}
  <div class="min-w-0 flex-1">
    <div class="flex items-center gap-2">
      <span class="truncate text-[13px] font-medium text-text">{disc?.title || 'Unknown'}</span>
      {#if job.drive_id}
        <span
          class="shrink-0 rounded px-1 py-0.5 font-mono text-[10px] tracking-[0.14em]"
          style="background: var(--surface-2); color: var(--text-3)"
        >
          {job.drive_id}
        </span>
      {/if}
    </div>
    <PipelineStepperMini {job} />
  </div>
  {#if isQueued}
    <span
      class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
      style="background: var(--surface-2); color: var(--text-3)"
    >
      QUEUED
    </span>
  {:else}
    <div class="text-right">
      <div class="font-mono text-[13px] font-semibold text-accent">
        {Math.round(job.progress)}%
      </div>
      {#if job.eta_seconds}
        <div class="font-mono text-[10px] text-text-3">{formatDuration(job.eta_seconds)}</div>
      {/if}
    </div>
  {/if}
</button>
