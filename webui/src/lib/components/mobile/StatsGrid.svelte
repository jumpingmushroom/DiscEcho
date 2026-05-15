<script lang="ts">
  import type { Stats } from '$lib/wire';
  import Sparkline from '$lib/components/Sparkline.svelte';
  import { formatBytes } from '$lib/format';

  export let stats: Stats | undefined = undefined;
</script>

{#if stats}
  <div class="grid grid-cols-2 gap-2 px-4">
    <div class="rounded-xl border border-border bg-surface-1 p-3">
      <div class="flex items-center justify-between">
        <span class="font-medium uppercase tracking-[0.14em] text-text-3" style="font-size: 10px"
          >Active</span
        >
        <Sparkline data={stats.active_jobs.spark_24h} width={40} height={16} />
      </div>
      <div class="mt-1 font-mono text-[20px] font-bold leading-none text-text">
        {stats.active_jobs.value}
      </div>
      <div
        class="mt-0.5 font-mono"
        style="font-size: 10px; color: {stats.active_jobs.delta_1h >= 0
          ? 'var(--accent)'
          : 'var(--text-3)'}"
      >
        {stats.active_jobs.delta_1h >= 0 ? '+' : ''}{stats.active_jobs.delta_1h}
      </div>
    </div>

    <div class="rounded-xl border border-border bg-surface-1 p-3">
      <div class="flex items-center justify-between">
        <span class="font-medium uppercase tracking-[0.14em] text-text-3" style="font-size: 10px"
          >Today</span
        >
        <Sparkline data={stats.today_ripped.spark_7d_bytes.map(Number)} width={40} height={16} />
      </div>
      <div class="mt-1 font-mono text-[20px] font-bold leading-none text-text">
        {formatBytes(stats.today_ripped.bytes)}
      </div>
      <div class="mt-0.5 font-mono text-text-3" style="font-size: 10px">
        +{stats.today_ripped.titles} titles
      </div>
    </div>

    <div class="rounded-xl border border-border bg-surface-1 p-3">
      <div class="flex items-center justify-between">
        <span class="font-medium uppercase tracking-[0.14em] text-text-3" style="font-size: 10px"
          >Library</span
        >
        <Sparkline data={stats.library.spark_30d_used.map(Number)} width={40} height={16} />
      </div>
      <div class="mt-1 font-mono text-[20px] font-bold leading-none text-text">
        {formatBytes(stats.library.used_bytes)}
      </div>
      <div class="mt-0.5 font-mono text-text-3" style="font-size: 10px">
        {stats.library.total_bytes > 0 ? `of ${formatBytes(stats.library.total_bytes)}` : ''}
      </div>
    </div>

    <div class="rounded-xl border border-border bg-surface-1 p-3">
      <div class="flex items-center justify-between">
        <span class="font-medium uppercase tracking-[0.14em] text-text-3" style="font-size: 10px"
          >Failures 7d</span
        >
        <Sparkline data={stats.failures_7d.spark_30d} width={40} height={16} />
      </div>
      <div class="mt-1 font-mono text-[20px] font-bold leading-none text-text">
        {stats.failures_7d.value}
      </div>
      <div
        class="mt-0.5 font-mono"
        style="font-size: 10px; color: {stats.failures_7d.value <= stats.failures_7d.previous
          ? 'var(--accent)'
          : 'var(--error)'}"
      >
        {stats.failures_7d.value - stats.failures_7d.previous >= 0 ? '+' : ''}{stats.failures_7d
          .value - stats.failures_7d.previous} vs prev
      </div>
    </div>
  </div>
{/if}
