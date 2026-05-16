<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { HistoryRow as HRow } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import ArtPlaceholder from './ArtPlaceholder.svelte';
  import { startDisc } from '$lib/store';

  export let row: HRow;
  const dispatch = createEventDispatcher<{ click: void }>();

  $: title = row.disc.title || 'Unknown';
  $: completedAt = row.job.finished_at ?? '';
  $: state = row.job.state;
  $: stateBadge = state === 'failed' ? 'FAILED' : state === 'cancelled' ? 'CANCELLED' : null;
  $: artLabel = row.disc.type === 'AUDIO_CD' ? 'cd' : 'cover';
  $: artRatio = (row.disc.type === 'AUDIO_CD' ? 'square' : 'portrait') as 'square' | 'portrait';

  let busy = false;
  let errMsg = '';

  async function onRerip(): Promise<void> {
    busy = true;
    errMsg = '';
    try {
      await startDisc(row.disc.id, row.job.profile_id, 0);
    } catch (e) {
      errMsg = (e as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<div
  class="flex w-full min-h-[44px] items-center gap-3 border-b border-border px-3 py-3 last:border-0 hover:bg-surface-2"
>
  <button
    type="button"
    class="flex flex-1 items-center gap-3 text-left"
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
  <button
    type="button"
    class="min-h-[36px] shrink-0 rounded-xl border border-border bg-surface-2 px-3 text-[13px] font-medium text-accent disabled:opacity-50"
    on:click={onRerip}
    disabled={busy}
    data-testid="history-rerip"
  >
    {busy ? 'Starting…' : 'Re-rip'}
  </button>
</div>
{#if errMsg}
  <div class="px-3 pb-2 text-[11px] text-error">{errMsg}</div>
{/if}

<style>
  .failure-badge {
    background: rgba(255, 91, 91, 0.15);
    color: var(--error);
  }
</style>
