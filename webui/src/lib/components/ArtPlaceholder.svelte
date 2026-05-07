<script lang="ts">
  export let label: string = 'cover';
  export let size: number = 64;
  export let ratio: 'square' | 'portrait' = 'portrait';

  $: width = size;
  $: height = ratio === 'square' ? size : Math.round(size * 1.4);
</script>

<div
  class="flex flex-shrink-0 items-center justify-center rounded-lg border border-border
         bg-surface-2 text-[10px] font-medium uppercase tracking-[0.14em] text-text-3"
  style="width: {width}px; height: {height}px"
>
  {label}
</div>
