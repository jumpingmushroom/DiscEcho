<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    history,
    historyTotal,
    historyLoading,
    historyError,
    historyFilter,
    fetchHistoryPage,
    liveStatus,
  } from '$lib/store';
  import AppBar from '$lib/components/mobile/AppBar.svelte';
  import TabBar from '$lib/components/mobile/TabBar.svelte';
  import LiveDot from '$lib/components/LiveDot.svelte';
  import { isDesktop } from '$lib/viewport';
  import FilterChips from '$lib/components/FilterChips.svelte';
  import HistoryRow from '$lib/components/HistoryRow.svelte';
  import ClearHistoryButton from '$lib/components/ClearHistoryButton.svelte';
  import { dayGroupLabel } from '$lib/time';
  import type { HistoryRow as HRow, DiscType } from '$lib/wire';

  let sentinel: HTMLDivElement | null = null;
  let observer: IntersectionObserver | null = null;
  let offset = 0;

  async function loadFirstPage(filter: DiscType | ''): Promise<void> {
    offset = 0;
    const got = await fetchHistoryPage(filter, 0);
    offset = got;
  }

  async function loadNextPage(): Promise<void> {
    if ($historyLoading) return;
    if ($history.length >= $historyTotal) return;
    const got = await fetchHistoryPage($historyFilter, offset);
    offset += got;
  }

  function onFilterChange(e: CustomEvent<DiscType | ''>): void {
    historyFilter.set(e.detail);
    void loadFirstPage(e.detail);
  }

  function grouped(rows: HRow[]): Array<[string, HRow[]]> {
    const map = new Map<string, HRow[]>();
    for (const r of rows) {
      const key = dayGroupLabel(r.job.finished_at ?? r.job.created_at);
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(r);
    }
    return [...map.entries()];
  }

  function emptyTitle(filter: DiscType | ''): string {
    if (!filter) return 'No rips yet';
    if (filter === 'AUDIO_CD') return 'No audio CDs in history yet';
    return `No ${filter}s in history yet`;
  }

  function retry(): void {
    void loadFirstPage($historyFilter);
  }

  function onCleared(): void {
    void loadFirstPage($historyFilter);
  }

  onMount(() => {
    void loadFirstPage($historyFilter);
  });

  $: if (typeof IntersectionObserver !== 'undefined' && sentinel) {
    observer?.disconnect();
    observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) void loadNextPage();
      },
      { rootMargin: '0px 0px 200px 0px' },
    );
    observer.observe(sentinel);
  }

  onDestroy(() => {
    observer?.disconnect();
  });
</script>

<div class="min-h-screen pb-24">
  <AppBar title="History" subtitle="last 30 days">
    <div slot="right" class="flex items-center gap-2">
      <ClearHistoryButton total={$historyTotal} on:cleared={onCleared} />
      <span class="lg:hidden">
        <LiveDot label={$liveStatus === 'live' ? 'LIVE' : 'WAIT'} />
      </span>
    </div>
  </AppBar>

  <FilterChips active={$historyFilter} on:change={onFilterChange} />

  {#if $historyError}
    <div
      class="mx-5 mb-3 rounded-xl border px-4 py-3"
      style="border-color: rgba(255,91,91,0.3); background: rgba(255,91,91,0.1)"
    >
      <div class="text-[13px] font-medium" style="color: var(--error)">Couldn't load history.</div>
      <div class="mt-1 font-mono text-[11px] text-text-3">{$historyError}</div>
      <button
        class="mt-2 min-h-[36px] rounded-md border border-border px-3 text-[12px] text-text-2"
        on:click={retry}>Retry</button
      >
    </div>
  {/if}

  {#if $historyLoading && $history.length === 0}
    <div class="mt-12 flex justify-center">
      <span class="font-mono text-[12px] text-text-3">Loading…</span>
    </div>
  {:else if $history.length === 0}
    <div class="mt-12 px-5 text-center">
      <div class="mb-2 text-[16px] font-semibold text-text-2">{emptyTitle($historyFilter)}</div>
      <div class="mx-auto max-w-[300px] text-[13px] text-text-3">Insert a disc to get started.</div>
    </div>
  {:else}
    <div class="px-5">
      {#each grouped($history) as [label, rows] (label)}
        <div class="mb-4">
          <div class="mb-2 text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
            {label}
          </div>
          <div class="overflow-hidden rounded-2xl border border-border bg-surface-1">
            {#each rows as row (row.job.id)}
              <HistoryRow {row} on:click={() => goto(`/jobs/${row.job.id}`)} />
            {/each}
          </div>
        </div>
      {/each}
      {#if $history.length < $historyTotal}
        <div bind:this={sentinel} class="h-1"></div>
      {/if}
    </div>
  {/if}
</div>

{#if !$isDesktop}
  <TabBar active="history" />
{/if}
