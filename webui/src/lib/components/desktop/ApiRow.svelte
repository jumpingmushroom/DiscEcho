<script lang="ts">
  import Icon from '$lib/icons/Icon.svelte';
  import { createEventDispatcher } from 'svelte';

  export let name: string;
  export let hint: string = '';
  export let status: string;
  export let detail: string = '';
  export let editable: string = '';

  const dispatch = createEventDispatcher<{ edit: void }>();

  $: tone = statusTone(status);

  function statusTone(s: string): 'accent' | 'soft' | 'error' {
    if (s.startsWith('error:')) return 'error';
    if (s === 'connected') return 'accent';
    return 'soft';
  }
</script>

<div class="grid items-center gap-3 px-4 py-3" style="grid-template-columns: 16px 1fr auto auto">
  <Icon name="key" size={14} stroke="var(--text-2)" />
  <div class="min-w-0">
    <div class="text-[13px] font-medium text-text">{name}</div>
    {#if hint}
      <div class="mt-0.5 text-[11px] text-text-3">{hint}</div>
    {/if}
  </div>
  <span
    class="inline-flex items-center gap-1.5 rounded-md border px-2 py-[3px] text-[11px] font-medium tracking-wide"
    class:border-accent={tone === 'accent'}
    class:text-accent={tone === 'accent'}
    class:bg-accent-soft={tone === 'accent'}
    class:border-error={tone === 'error'}
    class:text-error={tone === 'error'}
    class:border-border-strong={tone === 'soft'}
    class:bg-surface-2={tone === 'soft'}
    class:text-text-2={tone === 'soft'}
  >
    {#if tone === 'accent'}
      <span class="h-1.5 w-1.5 rounded-full" style="background: currentColor"></span>
    {/if}
    {status}
    {#if detail}
      <span class="text-text-3">· {detail}</span>
    {/if}
  </span>
  {#if editable}
    <button
      type="button"
      on:click={() => dispatch('edit')}
      class="rounded-md px-2 py-1 text-[11px] text-text-2 hover:bg-surface-2 hover:text-text"
      title="set {editable} in .env to configure"
    >
      Edit
    </button>
  {:else}
    <button
      type="button"
      on:click={() => dispatch('edit')}
      class="rounded-md px-2 py-1 text-[11px] text-text-2 hover:bg-surface-2 hover:text-text"
    >
      Edit
    </button>
  {/if}
</div>
