<script lang="ts">
  import { toasts, dismissToast } from '$lib/toasts';
  import Icon from '$lib/icons/Icon.svelte';
</script>

{#if $toasts.length > 0}
  <div class="fixed bottom-4 right-4 z-50 flex flex-col-reverse gap-2">
    {#each $toasts as toast (toast.id)}
      <div
        class="flex items-center gap-2 rounded-lg border bg-surface-1 px-4 py-3 text-[13px] text-text shadow-lg"
        class:border-accent={toast.kind === 'success'}
        class:border-error={toast.kind === 'error'}
        role={toast.kind === 'error' ? 'alert' : 'status'}
      >
        <span
          class:text-accent={toast.kind === 'success'}
          class:text-error={toast.kind === 'error'}
        >
          <Icon name={toast.kind === 'success' ? 'check' : 'alert'} size={15} />
        </span>
        <span>{toast.message}</span>
        <button
          class="ml-1 text-text-3 transition-colors hover:text-text"
          on:click={() => dismissToast(toast.id)}
          aria-label="Dismiss"
        >
          <Icon name="x" size={14} />
        </button>
      </div>
    {/each}
  </div>
{/if}
