<script lang="ts">
  import type { Drive, Disc, Job, Profile, StepID } from '$lib/wire';
  import { logs } from '$lib/store';
  import ArtPlaceholder from './ArtPlaceholder.svelte';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import PipelineStepperHorizontal from './PipelineStepperHorizontal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import SpeedEtaChip from './SpeedEtaChip.svelte';

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job;
  export let profile: Profile | undefined = undefined;

  const LOG_TAIL_LINES = 12;

  $: tail = ($logs[job.id] ?? []).slice(-LOG_TAIL_LINES);

  // State pill — derived from active_step where possible so the user sees
  // "TRANSCODING" instead of a generic "RIPPING" once the laser is done.
  // Falls back to drive.state for the brief window before OnStepStart
  // arrives.
  function stateLabel(j: Job, drv: Drive): string {
    switch (j.active_step as StepID | undefined) {
      case 'detect':
      case 'identify':
        return 'IDENTIFYING';
      case 'rip':
        return 'RIPPING';
      case 'transcode':
        return 'TRANSCODING';
      case 'compress':
        return 'COMPRESSING';
      case 'move':
        return 'MOVING';
      case 'notify':
        return 'NOTIFYING';
      case 'eject':
        return 'EJECTING';
      default:
        return drv.state.toUpperCase();
    }
  }

  function discTitle(d: Disc | undefined): string {
    if (!d) return '—';
    if (d.title) return d.title;
    const top = d.candidates?.[0];
    if (top?.title) return top.title;
    return d.id.slice(0, 8);
  }

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
  <!-- Header: drive bus + model + state pill -->
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0 flex-1">
      <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
        {drive.bus}
      </div>
      <div class="mt-1 truncate text-[15px] font-semibold text-text">{drive.model}</div>
    </div>
    <span
      class="shrink-0 rounded px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
      style="background: var(--accent-soft); color: var(--accent)"
    >
      {stateLabel(job, drive)}
    </span>
  </div>

  <!-- Disc identity row -->
  <div class="mt-4 flex gap-4">
    <ArtPlaceholder
      label={disc?.type === 'AUDIO_CD' ? 'cd' : 'cover'}
      size={64}
      ratio={disc?.type === 'AUDIO_CD' ? 'square' : 'portrait'}
    />
    <div class="min-w-0 flex-1">
      {#if disc}<DiscTypeBadge type={disc.type} />{/if}
      <div class="mt-1 truncate text-[16px] font-semibold text-text">
        {discTitle(disc)}
      </div>
      <div class="mt-1 text-[12px] text-text-3">
        {disc?.year ?? ''}
      </div>
      <div class="mt-2 flex items-center gap-2 font-mono text-[11px] text-text-3">
        <span>{drive.bus.toLowerCase()}</span>
        {#if profile}<span>·</span><span>{profile.name}</span>{/if}
      </div>
    </div>
  </div>

  <!-- Pipeline stepper -->
  <div class="mt-5">
    <PipelineStepperHorizontal {job} />
  </div>

  <!-- Progress bar -->
  <div class="mt-5 space-y-2">
    <ProgressBar value={job.progress} height={4} animated />
    <div class="flex items-center justify-between">
      <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
      <span class="font-mono text-[14px] font-semibold text-accent">
        {Math.round(job.progress)}%
      </span>
    </div>
  </div>

  <!-- Log tail -->
  <div class="mt-5">
    <div class="mb-2 text-[10px] font-medium uppercase tracking-[0.14em] text-text-3">Log tail</div>
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
</div>
