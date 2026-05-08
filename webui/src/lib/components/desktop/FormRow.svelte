<script lang="ts">
  export let label: string;
  export let error: string | null = null;
</script>

<div class="grid items-center gap-4 px-4 py-3" style="grid-template-columns: 200px 1fr">
  <span class="text-[12px] text-text-2">{label}</span>
  <div class="min-w-0">
    <slot />
    {#if error}
      <div class="mt-1 text-[11px] text-error">{error}</div>
    {/if}
  </div>
</div>
