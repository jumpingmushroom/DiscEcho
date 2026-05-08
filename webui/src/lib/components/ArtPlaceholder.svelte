<script lang="ts">
  export let label: string = 'cover';
  export let size: number = 64;
  export let ratio: 'square' | 'portrait' = 'portrait';

  $: width = size;
  $: height = ratio === 'square' ? size : Math.round(size * 1.42);
</script>

<div
  class="relative flex flex-shrink-0 items-center justify-center overflow-hidden rounded-md border border-border"
  style="
    width: {width}px;
    height: {height}px;
    background: repeating-linear-gradient(135deg,
      var(--surface-2) 0px, var(--surface-2) 6px,
      var(--surface-3) 6px, var(--surface-3) 12px);
  "
>
  <span class="font-mono text-[9px] uppercase tracking-[0.15em] text-text-3">{label}</span>
</div>
