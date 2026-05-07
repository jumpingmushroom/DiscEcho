<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { DiscType } from '$lib/wire';

  export let active: DiscType | '' = '';

  const opts: Array<{ id: DiscType | ''; label: string }> = [
    { id: '', label: 'All' },
    { id: 'AUDIO_CD', label: 'Audio CD' },
    { id: 'DVD', label: 'DVD' },
  ];

  const dispatch = createEventDispatcher<{ change: DiscType | '' }>();
</script>

<div class="overflow-x-auto px-5 pb-3">
  <div class="flex gap-2" style="width: max-content">
    {#each opts as o (o.id)}
      <button
        type="button"
        class="chip min-h-[44px] whitespace-nowrap rounded-full border px-3 text-[12px] font-medium"
        class:active={o.id === active}
        on:click={() => dispatch('change', o.id)}
      >
        {o.label}
      </button>
    {/each}
  </div>
</div>

<style>
  .chip {
    background: var(--surface-1);
    color: var(--text-2);
    border-color: var(--border);
  }
  .chip.active {
    background: var(--accent-soft);
    color: var(--accent);
    border-color: rgba(0, 214, 143, 0.3);
  }
</style>
