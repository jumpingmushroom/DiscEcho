<script lang="ts">
  import type { Job } from '$lib/wire';
  import { discs, profiles, logs } from '$lib/store';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import ArtPlaceholder from '$lib/components/ArtPlaceholder.svelte';
  import ProgressBar from '$lib/components/ProgressBar.svelte';
  import SpeedEtaChip from '$lib/components/SpeedEtaChip.svelte';
  import PipelineStepperHorizontal from '$lib/components/PipelineStepperHorizontal.svelte';

  export let job: Job | undefined = undefined;

  const LOG_TAIL_LINES = 12;

  $: disc = job ? $discs[job.disc_id] : undefined;
  $: profile = job ? $profiles.find((p) => p.id === job.profile_id) : undefined;
  $: tail = job ? ($logs[job.id] ?? []).slice(-LOG_TAIL_LINES) : [];

  function levelColour(level: string): string {
    switch (level) {
      case 'warn':
        return 'var(--warn)';
      case 'error':
        return 'var(--error)';
      default:
        return 'var(--text-3)';
    }
  }
</script>

<div class="rounded-2xl border border-border bg-surface-1 p-5">
  {#if !job}
    <div class="py-12 text-center text-[13px] text-text-3">
      Click a drive or queue row to inspect a job.
    </div>
  {:else}
    <div class="flex gap-4">
      <ArtPlaceholder
        label={disc?.type === 'AUDIO_CD' ? 'cd' : 'cover'}
        size={64}
        ratio={disc?.type === 'AUDIO_CD' ? 'square' : 'portrait'}
      />
      <div class="min-w-0 flex-1">
        {#if disc}<DiscTypeBadge type={disc.type} />{/if}
        <div class="mt-1 truncate text-[16px] font-semibold text-text">
          {disc?.title || 'Unknown'}
        </div>
        <div class="mt-1 text-[12px] text-text-3">
          {disc?.year ? disc.year : ''}
        </div>
        <div class="mt-2 flex items-center gap-2 font-mono text-[11px] text-text-3">
          <span>{job.drive_id || '—'}</span>
          {#if profile}<span>·</span><span>{profile.name}</span>{/if}
        </div>
      </div>
    </div>

    <div class="mt-5">
      <PipelineStepperHorizontal {job} />
    </div>

    <div class="mt-5 space-y-2">
      <ProgressBar value={job.progress} height={4} animated />
      <div class="flex items-center justify-between">
        <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
        <span class="font-mono text-[14px] font-semibold text-accent">
          {Math.round(job.progress)}%
        </span>
      </div>
    </div>

    <div class="mt-5">
      <div class="mb-2 text-[10px] font-medium uppercase tracking-[0.14em] text-text-3">
        Log tail
      </div>
      <div
        class="max-h-48 overflow-y-auto rounded-md border border-border p-2 font-mono text-[11px]
               leading-relaxed"
        style="background: var(--surface-2)"
      >
        {#if tail.length === 0}
          <div class="text-text-3">No log lines yet.</div>
        {:else}
          {#each tail as line}
            <div class="flex gap-2">
              <span class="shrink-0 text-text-3">{line.t.slice(11, 23)}</span>
              <span class="shrink-0 uppercase" style="color: {levelColour(line.level)}">
                {line.level}
              </span>
              <span class="text-text-2">{line.message}</span>
            </div>
          {/each}
        {/if}
      </div>
    </div>
  {/if}
</div>
