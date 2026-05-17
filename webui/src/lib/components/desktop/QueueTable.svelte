<script lang="ts">
  import type { Job, Disc, Drive } from '$lib/wire';
  import { discs, drives } from '$lib/store';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import PipelineStepperMini from '$lib/components/PipelineStepperMini.svelte';
  import { createEventDispatcher } from 'svelte';
  import { formatDuration } from '$lib/time';
  import { formatProgress } from '$lib/formatProgress';

  export let jobs: Job[];
  export let selectedJobID: string | null = null;

  const dispatch = createEventDispatcher<{ select: string }>();

  function onRowClick(j: Job): void {
    dispatch('select', j.id);
  }

  function jobTitle(disc: Disc | undefined): string {
    if (!disc) return '—';
    if (disc.title) return disc.title;
    const top = disc.candidates?.[0];
    if (top?.title) return top.title;
    return disc.id.slice(0, 8);
  }

  function driveLabel(driveID: string, drs: Drive[]): string {
    const d = drs.find((x) => x.id === driveID);
    return d?.bus || driveID.slice(0, 8) || '—';
  }
</script>

<div class="overflow-hidden rounded-2xl border border-border bg-surface-1">
  {#if jobs.length === 0}
    <div class="p-6 text-center text-[13px] text-text-3">No active jobs</div>
  {:else}
    <table class="w-full table-auto text-left">
      <thead class="border-b border-border">
        <tr class="text-[10px] font-medium uppercase tracking-[0.14em] text-text-3">
          <th class="px-4 py-2">Type</th>
          <th class="px-4 py-2">Title</th>
          <th class="px-4 py-2">Drv</th>
          <th class="px-4 py-2">Step</th>
          <th class="px-4 py-2 text-right">Pct</th>
          <th class="px-4 py-2 text-right">ETA</th>
        </tr>
      </thead>
      <tbody>
        {#each jobs as j (j.id)}
          {@const disc = $discs[j.disc_id]}
          {@const isSelected = j.id === selectedJobID}
          <tr
            data-job-id={j.id}
            data-selected={isSelected ? 'true' : 'false'}
            class="cursor-pointer border-b border-border last:border-0 hover:bg-surface-2"
            class:selected={isSelected}
            on:click={() => onRowClick(j)}
          >
            <td class="px-4 py-2">
              {#if disc}<DiscTypeBadge type={disc.type} />{/if}
            </td>
            <td class="truncate px-4 py-2 text-[13px] font-medium text-text">
              {jobTitle(disc)}
            </td>
            <td class="px-4 py-2 font-mono text-[12px] text-text-3">
              {driveLabel(j.drive_id ?? '', $drives)}
            </td>
            <td class="px-4 py-2"><PipelineStepperMini job={j} /></td>
            <td class="px-4 py-2 text-right">
              {#if j.state === 'queued'}
                <span
                  class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
                  style="background: var(--surface-2); color: var(--text-3)"
                >
                  QUEUED
                </span>
              {:else}
                <span class="font-mono text-[13px] font-semibold text-accent">
                  {formatProgress(j.progress)}
                </span>
              {/if}
            </td>
            <td class="px-4 py-2 text-right font-mono text-[12px] text-text-3">
              {j.eta_seconds ? formatDuration(j.eta_seconds) : '—'}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  tr.selected {
    background: var(--surface-2);
  }
</style>
