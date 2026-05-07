<script lang="ts">
  import type { Job } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import PipelineStepperMini from './PipelineStepperMini.svelte';
  import { createEventDispatcher } from 'svelte';
  import { discs } from '$lib/store';

  export let job: Job;

  const dispatch = createEventDispatcher<{ click: void }>();

  $: disc = $discs[job.disc_id];
</script>

<button
  class="flex w-full min-h-[44px] items-center gap-3 border-b border-border px-4 py-3 text-left
         last:border-0 hover:bg-surface-2"
  on:click={() => dispatch('click')}
>
  {#if disc}<DiscTypeBadge type={disc.type} />{/if}
  <div class="min-w-0 flex-1">
    <div class="truncate text-[13px] font-medium text-text">{disc?.title || 'Unknown'}</div>
    <PipelineStepperMini {job} />
  </div>
  <div class="text-right">
    <div class="font-mono text-[13px] font-semibold text-accent">
      {Math.round(job.progress)}%
    </div>
    {#if job.eta_seconds}
      <div class="font-mono text-[10px] text-text-3">{job.eta_seconds}s</div>
    {/if}
  </div>
</button>
