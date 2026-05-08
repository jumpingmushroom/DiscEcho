<script lang="ts">
  export let title: string;
  export let sub: string | null = null;
</script>

<section>
  <div class="mb-3">
    <h3 class="text-[14px] font-semibold text-text">{title}</h3>
    {#if sub}
      <p class="mt-0.5 text-[12px] text-text-3">{sub}</p>
    {/if}
  </div>
  <div class="divide-y divide-border rounded-2xl border border-border bg-surface-1">
    <slot />
  </div>
</section>
