<script lang="ts">
  export let data: number[] = [];
  export let width: number = 60;
  export let height: number = 24;
  export let accent: string = 'var(--accent)';
  export let muted: string = 'rgba(0,214,143,0.2)';

  // Bars evenly spaced with a 1px gap; height proportional to series
  // max. Zero-only series renders all bars at height 1 so the chart
  // isn't blank.
  $: max = data.length > 0 ? Math.max(...data, 1) : 1;
  $: barCount = data.length;
  $: barWidth = barCount > 0 ? Math.max(1, (width - (barCount - 1)) / barCount) : 0;
</script>

{#if barCount > 0}
  <svg
    role="img"
    aria-label="sparkline"
    viewBox="0 0 {width} {height}"
    {width}
    {height}
    style="display: block; flex-shrink: 0"
  >
    {#each data as v, i}
      {@const h = Math.max(1, (v / max) * height)}
      <rect
        x={i * (barWidth + 1)}
        y={height - h}
        width={barWidth}
        height={h}
        fill={i === barCount - 1 ? accent : muted}
      />
    {/each}
  </svg>
{/if}
