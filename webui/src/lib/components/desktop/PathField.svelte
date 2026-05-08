<script lang="ts">
  import Icon from '$lib/icons/Icon.svelte';
  import { createEventDispatcher } from 'svelte';

  export let value: string;
  export let editable: boolean = true;
  export let placeholder: string = '';

  const dispatch = createEventDispatcher<{ change: string }>();

  let editing = false;
  let draft = value;
  let inputEl: HTMLInputElement | null = null;

  $: if (!editing) draft = value;
  $: if (editing && inputEl) inputEl.focus();

  function start(): void {
    if (!editable) return;
    draft = value;
    editing = true;
  }

  function commit(): void {
    const trimmed = draft.trim();
    if (trimmed && trimmed !== value) {
      dispatch('change', trimmed);
    }
    editing = false;
  }

  function cancel(): void {
    draft = value;
    editing = false;
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Enter') commit();
    if (e.key === 'Escape') cancel();
  }
</script>

{#if editing}
  <input
    bind:this={inputEl}
    type="text"
    bind:value={draft}
    on:blur={commit}
    on:keydown={onKey}
    {placeholder}
    class="flex h-9 w-full items-center gap-2 rounded-md border border-border-strong
           bg-surface-0 px-3 font-mono text-[12px] text-text outline-none
           focus:border-accent"
  />
{:else}
  <button
    type="button"
    on:click={start}
    disabled={!editable}
    class="flex h-9 w-full items-center gap-2 rounded-md border border-border
           bg-surface-0 px-3 text-left font-mono text-[12px] text-text
           transition-colors hover:border-border-strong
           disabled:cursor-default disabled:hover:border-border"
  >
    <Icon name="folder" size={12} />
    <span class="truncate">{value || placeholder}</span>
  </button>
{/if}
