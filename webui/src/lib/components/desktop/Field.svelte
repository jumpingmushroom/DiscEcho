<script lang="ts">
  export let label: string;
  export let error: string | undefined = undefined;
</script>

<label class="block">
  <span class="mb-1 block text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
    {label}
  </span>
  <slot />
  {#if error}
    <div class="mt-1 text-[11px] text-error">{error}</div>
  {/if}
</label>
