<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { DiscType } from '$lib/wire';
  import { DISC_TYPE_META } from './DiscTypeBadge.svelte';

  export let active: DiscType | '' = '';

  // Chip order: All, then audio, video, game discs (in console-launch order),
  // then DATA. Labels come from DISC_TYPE_META so they stay in sync with
  // DiscTypeBadge — keep the long-form "Audio CD" for the chip though, since
  // the badge's short "CD" reads ambiguously in a filter row.
  const opts: Array<{ id: DiscType | ''; label: string }> = [
    { id: '', label: 'All' },
    { id: 'AUDIO_CD', label: 'Audio CD' },
    { id: 'DVD', label: DISC_TYPE_META.DVD.label },
    { id: 'BDMV', label: DISC_TYPE_META.BDMV.label },
    { id: 'UHD', label: DISC_TYPE_META.UHD.label },
    { id: 'PSX', label: DISC_TYPE_META.PSX.label },
    { id: 'PS2', label: DISC_TYPE_META.PS2.label },
    { id: 'SAT', label: DISC_TYPE_META.SAT.label },
    { id: 'DC', label: DISC_TYPE_META.DC.label },
    { id: 'XBOX', label: DISC_TYPE_META.XBOX.label },
    { id: 'DATA', label: DISC_TYPE_META.DATA.label },
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
