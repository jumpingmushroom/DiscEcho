<script lang="ts">
  import type { Stats } from '$lib/wire';
  import Sparkline from '$lib/components/Sparkline.svelte';
  import { formatBytes } from '$lib/format';

  export let stats: Stats | undefined = undefined;

  function deltaClass(value: number, lowerIsBetter = false): string {
    if (value === 0) return 'text-text-3';
    const better = lowerIsBetter ? value < 0 : value > 0;
    return better ? 'text-accent' : 'text-error';
  }

  function signed(n: number): string {
    if (n > 0) return `+${n}`;
    return String(n);
  }
</script>

{#if stats}
  <div class="mb-6 grid gap-4" style="grid-template-columns: repeat(4, 1fr)">
    <!-- ACTIVE JOBS -->
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-center justify-between">
        <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3"
          >ACTIVE JOBS</span
        >
        <Sparkline data={stats.active_jobs.spark_24h} />
      </div>
      <div class="mt-2 font-mono text-[28px] font-bold leading-none text-text">
        {stats.active_jobs.value}
      </div>
      <div class="mt-1 font-mono text-[11px] {deltaClass(stats.active_jobs.delta_1h)}">
        {signed(stats.active_jobs.delta_1h)}
      </div>
    </div>

    <!-- TODAY RIPPED -->
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-center justify-between">
        <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3"
          >TODAY RIPPED</span
        >
        <Sparkline data={stats.today_ripped.spark_7d_bytes.map(Number)} />
      </div>
      <div class="mt-2 font-mono text-[28px] font-bold leading-none text-text">
        {formatBytes(stats.today_ripped.bytes)}
      </div>
      <div class="mt-1 font-mono text-[11px] text-text-3">
        +{stats.today_ripped.titles} titles
      </div>
    </div>

    <!-- LIBRARY SIZE -->
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-center justify-between">
        <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3"
          >LIBRARY SIZE</span
        >
        <Sparkline data={stats.library.spark_30d_used.map(Number)} />
      </div>
      <div class="mt-2 font-mono text-[28px] font-bold leading-none text-text">
        {formatBytes(stats.library.used_bytes)}
      </div>
      <div class="mt-1 font-mono text-[11px] text-text-3">
        {stats.library.total_bytes > 0 ? `of ${formatBytes(stats.library.total_bytes)}` : ''}
      </div>
    </div>

    <!-- FAILURES (7D) -->
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-center justify-between">
        <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3"
          >FAILURES (7D)</span
        >
        <Sparkline data={stats.failures_7d.spark_30d} />
      </div>
      <div class="mt-2 font-mono text-[28px] font-bold leading-none text-text">
        {stats.failures_7d.value}
      </div>
      <div
        class="mt-1 font-mono text-[11px] {deltaClass(
          stats.failures_7d.value - stats.failures_7d.previous,
          true,
        )}"
      >
        {signed(stats.failures_7d.value - stats.failures_7d.previous)} vs prev
      </div>
    </div>
  </div>
{/if}
