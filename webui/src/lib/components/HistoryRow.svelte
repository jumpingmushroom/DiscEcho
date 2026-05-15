<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { HistoryRow as HRow } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import ArtPlaceholder from './ArtPlaceholder.svelte';

  export let row: HRow;
  const dispatch = createEventDispatcher<{ click: void }>();

  $: title = row.disc.title || 'Unknown';
  $: completedAt = row.job.finished_at ?? '';
  $: state = row.job.state;
  $: stateBadge = state === 'failed' ? 'FAILED' : state === 'cancelled' ? 'CANCELLED' : null;
  $: artLabel = row.disc.type === 'AUDIO_CD' ? 'cd' : 'cover';
  $: artRatio = (row.disc.type === 'AUDIO_CD' ? 'square' : 'portrait') as 'square' | 'portrait';
</script>

<button
  class="flex w-full min-h-[44px] items-center gap-3 border-b border-border px-3 py-3 text-left
         last:border-0 hover:bg-surface-2"
  on:click={() => dispatch('click')}
>
  <ArtPlaceholder label={artLabel} size={44} ratio={artRatio} />
  <div class="min-w-0 flex-1">
    <div class="mb-0.5 flex items-center gap-2">
      <DiscTypeBadge type={row.disc.type} />
      {#if stateBadge}
        <span
          class="failure-badge rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
        >
          {stateBadge}
        </span>
      {/if}
    </div>
    <div class="truncate font-medium text-text" style="font-size: var(--ts-body)">{title}</div>
    <div class="mt-0.5 truncate font-mono text-text-3" style="font-size: var(--ts-meta)">
      {row.disc.year ? row.disc.year + ' · ' : ''}{completedAt.slice(11, 16)}
    </div>
  </div>
</button>

<style>
  .failure-badge {
    background: rgba(255, 91, 91, 0.15);
    color: var(--error);
  }
</style>
