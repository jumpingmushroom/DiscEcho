<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { clearHistory } from '$lib/store';

  // total gates visibility (the button is pointless with nothing to
  // clear) and is shown in the confirm label.
  export let total: number;

  const dispatch = createEventDispatcher<{ cleared: number }>();

  // Two-step confirm, matching the ProfileEditor delete pattern: the
  // first click arms `confirming`, the second performs the clear.
  let confirming = false;
  let clearing = false;
  let error: string | null = null;

  async function onClick(): Promise<void> {
    if (!confirming) {
      confirming = true;
      return;
    }
    clearing = true;
    error = null;
    try {
      const deleted = await clearHistory();
      confirming = false;
      dispatch('cleared', deleted);
    } catch (e) {
      error = (e as Error).message;
      confirming = false;
    } finally {
      clearing = false;
    }
  }
</script>

{#if total > 0}
  <div class="flex flex-col items-end gap-1">
    <button
      type="button"
      class="min-h-[36px] rounded-md border px-3 text-[12px] disabled:opacity-50"
      style="border-color: {confirming ? 'var(--error)' : 'var(--border)'};
             color: {confirming ? 'var(--error)' : 'var(--text-2)'}"
      on:click={onClick}
      disabled={clearing}
    >
      {confirming ? `Confirm — clear ${total} rip${total === 1 ? '' : 's'}?` : 'Clear history'}
    </button>
    {#if error}
      <span class="text-[11px]" style="color: var(--error)">{error}</span>
    {/if}
  </div>
{/if}
