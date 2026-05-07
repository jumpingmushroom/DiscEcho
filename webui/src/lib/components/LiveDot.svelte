<script lang="ts">
  export let label: string | undefined = undefined;
</script>

<span class="inline-flex items-center gap-1.5">
  <span class="live-dot h-1.5 w-1.5 rounded-full bg-accent" aria-hidden="true"></span>
  {#if label}
    <span class="text-[10px] font-medium uppercase tracking-[0.14em] text-text-3">{label}</span>
  {/if}
</span>
