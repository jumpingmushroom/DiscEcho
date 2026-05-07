<script lang="ts">
  import { formatDuration } from '$lib/time';
  export let speed: string | undefined = undefined;
  export let etaSeconds: number | undefined = undefined;
</script>

{#if speed || etaSeconds !== undefined}
  <span
    class="inline-flex items-center gap-2 rounded-md border border-border bg-surface-2
           px-2 py-1 font-mono text-[11px] text-text-2"
  >
    {#if speed}<span>{speed}</span>{/if}
    {#if etaSeconds !== undefined}
      <span class="text-text-3">·</span>
      <span>ETA {formatDuration(etaSeconds)}</span>
    {/if}
  </span>
{/if}
