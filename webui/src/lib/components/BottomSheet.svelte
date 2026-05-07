<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  export let open: boolean = false;
  const dispatch = createEventDispatcher<{ close: void }>();
</script>

{#if open}
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <div
    class="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm"
    on:click={() => dispatch('close')}
    on:keydown={(e) => e.key === 'Escape' && dispatch('close')}
    role="dialog"
    tabindex="-1"
    aria-modal="true"
  >
    <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
    <div
      class="absolute bottom-0 left-0 right-0 rounded-t-3xl border-l border-r border-t
             border-border-strong bg-surface-1 pb-safe pt-2"
      on:click|stopPropagation
      on:keydown|stopPropagation
      role="dialog"
      tabindex="-1"
    >
      <div class="mx-auto mb-3 mt-1 h-1.5 w-12 rounded-full bg-border-strong"></div>
      <slot />
    </div>
  </div>
{/if}
