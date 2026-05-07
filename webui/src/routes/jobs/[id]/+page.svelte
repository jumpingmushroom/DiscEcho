<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { jobs, discs, logs, cancelJob } from '$lib/store';
  import LiveDot from '$lib/components/LiveDot.svelte';
  import ArtPlaceholder from '$lib/components/ArtPlaceholder.svelte';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import PipelineStepperVertical from '$lib/components/PipelineStepperVertical.svelte';
  import KVRow from '$lib/components/KVRow.svelte';

  $: id = $page.params.id;
  $: job = $jobs.find((j) => j.id === id);
  $: disc = job ? $discs[job.disc_id] : undefined;
  $: jobLogs = id ? ($logs[id] ?? []) : [];

  async function onCancel(): Promise<void> {
    if (!job) return;
    if (!confirm('Cancel this job?')) return;
    await cancelJob(job.id);
    goto('/');
  }
</script>

<div class="min-h-screen pb-32">
  <!-- header -->
  <div
    class="sticky top-0 z-20 flex items-center justify-between border-b border-border px-3 py-3
           backdrop-blur"
    style="background: rgba(5,5,5,0.92)"
  >
    <button
      class="flex min-h-[44px] min-w-[44px] items-center justify-center text-text-2"
      on:click={() => goto('/')}
      aria-label="Back"
    >
      ←
    </button>
    <div class="flex items-center gap-2">
      {#if disc}<DiscTypeBadge type={disc.type} />{/if}
      <LiveDot />
    </div>
    <div class="w-11"></div>
  </div>

  {#if !job}
    <div class="px-5 py-12 text-center text-text-3">Job not found.</div>
  {:else}
    <!-- hero -->
    <div class="flex gap-4 border-b border-border px-5 pb-5 pt-4">
      <ArtPlaceholder label="cover" size={84} ratio="portrait" />
      <div class="min-w-0 flex-1">
        <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
          Job {job.id.slice(0, 8)}…
        </div>
        <div class="mt-1 text-[20px] font-bold leading-tight text-text">
          {disc?.title || 'Unknown disc'}
        </div>
        <div class="mt-2">
          <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
        </div>
      </div>
    </div>

    <!-- pipeline -->
    <div class="px-2 pb-2 pt-4">
      <div class="px-3 pb-2 text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
        Pipeline
      </div>
      <PipelineStepperVertical {job} />
    </div>

    <!-- log -->
    {#if jobLogs.length > 0}
      <div class="mt-4 px-5">
        <details class="overflow-hidden rounded-2xl border border-border bg-surface-1" open>
          <summary class="cursor-pointer px-4 py-3 text-[13px] font-medium text-text">
            Live log
          </summary>
          <div
            class="max-h-[180px] overflow-auto border-t border-border p-3 font-mono
                   text-[10.5px] leading-[1.7]"
          >
            {#each jobLogs as l, i (i)}
              <div class="flex gap-2">
                <span class="shrink-0 text-text-3">{l.t.slice(11, 19)}</span>
                <span
                  class="w-9 shrink-0 uppercase"
                  style="color: {l.level === 'warn'
                    ? 'var(--warn)'
                    : l.level === 'error'
                      ? 'var(--error)'
                      : 'var(--info)'}"
                >
                  {l.level}
                </span>
                <span class="flex-1 break-words text-text-2">{l.message}</span>
              </div>
            {/each}
          </div>
        </details>
      </div>
    {/if}

    <!-- output -->
    <div class="mt-4 space-y-3 px-5">
      <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Output</div>
      <div class="divide-y divide-border rounded-2xl border border-border bg-surface-1">
        <KVRow label="State">{job.state}</KVRow>
        <KVRow label="Progress">{Math.round(job.progress)}%</KVRow>
        {#if job.error_message}
          <KVRow label="Error">{job.error_message}</KVRow>
        {/if}
      </div>
    </div>

    <!-- sticky actions -->
    <div
      class="fixed bottom-0 left-0 right-0 z-30 flex items-center gap-2 border-t border-border
             bg-bg px-3 py-3"
      style="padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 12px)"
    >
      <button class="min-h-[44px] flex-1 rounded-xl border border-border text-text-3" disabled>
        Pause
      </button>
      <button class="min-h-[44px] flex-1 rounded-xl border border-border text-text-3" disabled>
        Override
      </button>
      <button
        class="min-h-[44px] rounded-xl border px-4"
        style="border-color: var(--error); color: var(--error); background: rgba(255,91,91,0.12)"
        on:click={onCancel}
      >
        Cancel
      </button>
    </div>
  {/if}
</div>
