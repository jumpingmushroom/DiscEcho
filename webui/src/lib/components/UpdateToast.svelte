<script lang="ts">
  import { updateAvailable, applyUpdate } from '$lib/pwa';
</script>

{#if $updateAvailable}
  <div
    class="fixed bottom-4 left-4 z-50 rounded-lg border border-border bg-surface-1 px-4 py-3 shadow-lg"
    role="status"
  >
    <div class="text-[13px] text-text">A new version is available.</div>
    <button
      class="mt-2 rounded-md bg-accent px-3 py-1.5 text-[12px] font-semibold text-black"
      on:click={applyUpdate}
    >
      Reload
    </button>
  </div>
{/if}
