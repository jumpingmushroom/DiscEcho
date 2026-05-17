<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import type { Drive, Disc, Job, Profile, StepID, AccurateRipSummary } from '$lib/wire';
  import { logs, ensureLogBackfill } from '$lib/store';
  import { formatDuration } from '$lib/time';
  import { trackSummary } from '$lib/format';
  import { formatProgress } from '$lib/formatProgress';
  import DiscArt from './DiscArt.svelte';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import PipelineStepperHorizontal from './PipelineStepperHorizontal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import SpeedEtaChip from './SpeedEtaChip.svelte';
  import AccurateRipBadge from './AccurateRipBadge.svelte';

  // accurateRipFor pulls the AccurateRipSummary out of a Job's rip-step
  // notes, if any. Used by RipCard to render a verified/unverified/
  // uncalibrated badge during and after audio-CD rips. Returns undefined
  // when the rip step has no notes or no `accuraterip` key (most
  // pre-v0.20 jobs and all non-audio jobs).
  function accurateRipFor(j: Job): AccurateRipSummary | undefined {
    const ripStep = j.steps?.find((s) => s.step === 'rip');
    const raw = ripStep?.notes?.accuraterip as Record<string, unknown> | undefined;
    if (!raw || typeof raw !== 'object') return undefined;
    const status = raw.status;
    if (status !== 'verified' && status !== 'unverified' && status !== 'uncalibrated') {
      return undefined;
    }
    return {
      status,
      verified_tracks: Number(raw.verified_tracks ?? 0),
      total_tracks: Number(raw.total_tracks ?? 0),
      min_confidence:
        typeof raw.min_confidence === 'number' ? (raw.min_confidence as number) : undefined,
      max_confidence:
        typeof raw.max_confidence === 'number' ? (raw.max_confidence as number) : undefined,
    };
  }

  export let drive: Drive;
  export let disc: Disc | undefined = undefined;
  export let job: Job;
  export let profile: Profile | undefined = undefined;

  const LOG_TAIL_LINES = 12;

  $: tail = ($logs[job.id] ?? []).slice(-LOG_TAIL_LINES);

  // Page-load-mid-rip: SSE only delivers new lines, so the log tail
  // sits empty until something fresh happens. Backfill from the
  // /api/jobs/:id/logs endpoint once on mount when the ring is empty.
  onMount(() => {
    if (job.state === 'running') {
      void ensureLogBackfill(job.id);
    }
  });

  // Elapsed wall-clock ticker. Audio rips can sit at 0% progress for
  // 1-3 min during whipper's pre-track warmup; without an elapsed
  // counter there's no signal of liveness besides the spinning step
  // icon. Ticks every second only while the job is non-terminal.
  let now = Date.now();
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;
  $: isRunning = job.state === 'running' || job.state === 'queued';
  $: if (isRunning && elapsedTimer === null) {
    elapsedTimer = setInterval(() => {
      now = Date.now();
    }, 1000);
  } else if (!isRunning && elapsedTimer !== null) {
    clearInterval(elapsedTimer);
    elapsedTimer = null;
  }
  onDestroy(() => {
    if (elapsedTimer !== null) clearInterval(elapsedTimer);
  });

  $: elapsedSeconds = (() => {
    if (!job.started_at) return 0;
    const startMs = new Date(job.started_at).getTime();
    if (!Number.isFinite(startMs)) return 0;
    return Math.max(0, Math.floor((now - startMs) / 1000));
  })();

  $: tracks = trackSummary(disc?.metadata_json);
  $: accurateRip = accurateRipFor(job);

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
    <div class="flex shrink-0 flex-col items-end gap-1">
      <span
        class="rounded px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
        style="background: var(--accent-soft); color: var(--accent)"
      >
        {stateLabel(job, drive)}
      </span>
      {#if isRunning && elapsedSeconds > 0}
        <span class="font-mono text-[11px] text-text-3" data-testid="elapsed">
          {formatDuration(elapsedSeconds)}
        </span>
      {/if}
    </div>
  </div>

  <!-- Disc identity row -->
  <div class="mt-4 flex gap-4">
    <DiscArt {disc} size={64} ratio={disc?.type === 'AUDIO_CD' ? 'square' : 'portrait'} />
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
        {#if tracks}<span>·</span><span>{tracks}</span>{/if}
      </div>
    </div>
  </div>

  <!-- Pipeline stepper -->
  <div class="mt-5">
    <PipelineStepperHorizontal {job} />
  </div>

  {#if accurateRip}
    <div class="mt-3" data-testid="accuraterip-badge">
      <AccurateRipBadge summary={accurateRip} />
    </div>
  {/if}

  <!-- Progress bar -->
  <div class="mt-5 space-y-2">
    <ProgressBar value={job.progress} height={4} animated />
    <div class="flex items-center justify-between">
      <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
      <span class="font-mono text-[14px] font-semibold text-accent">
        {formatProgress(job.progress)}
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
