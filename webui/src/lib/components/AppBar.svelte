<script lang="ts">
  export let title: string;
  export let subtitle: string | undefined = undefined;
</script>

<div class="sticky top-0 z-20 px-5 pb-3 pt-4 backdrop-blur" style="background: rgba(10,10,10,0.86)">
  <div class="flex items-center justify-between">
    <div class="min-w-0">
      {#if subtitle}
        <div class="mb-1 text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
          {subtitle}
        </div>
      {/if}
      <h1 class="text-[24px] font-bold leading-none tracking-tight text-text">{title}</h1>
    </div>
    <slot name="right" />
  </div>
</div>
