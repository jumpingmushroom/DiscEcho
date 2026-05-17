<script lang="ts">
  import type { Job, StepID } from '$lib/wire';
  import ProgressBar from './ProgressBar.svelte';
  import SpeedEtaChip from './SpeedEtaChip.svelte';
  import LiveDot from './LiveDot.svelte';
  import { formatProgress } from '$lib/formatProgress';

  export let job: Job;

  const STEPS: Array<{ id: StepID; label: string; desc: string }> = [
    { id: 'detect', label: 'Detect', desc: 'Drive sees disc' },
    { id: 'identify', label: 'Identify', desc: 'Match metadata' },
    { id: 'rip', label: 'Rip', desc: 'Read raw data' },
    { id: 'transcode', label: 'Transcode', desc: 'Re-encode A/V' },
    { id: 'compress', label: 'Compress', desc: 'Pack & verify' },
    { id: 'move', label: 'Move', desc: 'Move to library' },
    { id: 'notify', label: 'Notify', desc: 'Send notifications' },
    { id: 'eject', label: 'Eject', desc: 'Tray release' },
  ];

  const TERMINAL_STATES = new Set(['done', 'failed', 'cancelled', 'interrupted']);

  function status(step: StepID): 'done' | 'active' | 'pending' | 'skipped' | 'failed' {
    const stp = job.steps?.find((s) => s.step === step);
    if (stp?.state === 'skipped') return 'skipped';
    if (stp?.state === 'done') return 'done';
    if (stp?.state === 'failed') return 'failed';
    // Belt-and-braces: a 'running' step on a terminal job means the
    // daemon was killed mid-step before it could flip the row. Render
    // as failed so the user doesn't see a stale spinner.
    if (stp?.state === 'running' && TERMINAL_STATES.has(job.state)) return 'failed';
    if (job.active_step === step) return 'active';
    return 'pending';
  }

  // Skipped steps (e.g. Transcode/Compress on audio CDs, where whipper
  // does the FLAC encode inside Rip) are an internal accounting detail;
  // they're not informative to the user, so hide them from the list.
  $: visibleSteps = STEPS.filter((s) => status(s.id) !== 'skipped');
</script>

<div class="relative">
  <div class="absolute bottom-1 left-[15px] top-1 w-px bg-border"></div>
  {#each visibleSteps as s}
    {@const st = status(s.id)}
    {#if st === 'done' || st === 'failed' || st === 'skipped'}
      <div class="relative flex min-h-[40px] items-center py-2 pl-12 pr-3">
        <div
          class="absolute left-[8px] top-1/2 flex h-4 w-4 -translate-y-1/2 items-center
                 justify-center rounded-full"
          style="
            background: {st === 'failed' ? 'rgba(255,91,91,0.14)' : 'var(--accent-soft)'};
            border: 1px solid {st === 'failed' ? 'rgba(255,91,91,0.4)' : 'rgba(0,214,143,0.4)'};
          "
        >
          <svg
            width="10"
            height="10"
            viewBox="0 0 24 24"
            fill="none"
            stroke={st === 'failed' ? 'var(--error)' : 'var(--accent)'}
            stroke-width="3"
          >
            {#if st === 'failed'}
              <path d="M18 6L6 18M6 6l12 12" />
            {:else}
              <path d="M5 12l5 5L20 7" />
            {/if}
          </svg>
        </div>
        <div class="flex flex-1 items-center justify-between">
          <span class="text-[14px] text-text-2">{s.label}</span>
          <span class="font-mono text-[11px] text-text-3">{st}</span>
        </div>
      </div>
    {:else if st === 'active'}
      <div class="relative py-3 pl-12 pr-3">
        <div class="absolute left-[6px] top-3 h-5 w-5">
          <svg width="20" height="20" viewBox="0 0 20 20">
            <circle
              cx="10"
              cy="10"
              r="9"
              fill="rgba(0,214,143,0.08)"
              stroke="rgba(0,214,143,0.25)"
              stroke-width="1"
            />
            <circle
              cx="10"
              cy="10"
              r="9"
              fill="none"
              stroke="var(--accent)"
              stroke-width="2"
              stroke-dasharray={2 * Math.PI * 9}
              stroke-dashoffset={2 * Math.PI * 9 * (1 - job.progress / 100)}
              transform="rotate(-90 10 10)"
              stroke-linecap="round"
              style="transition: stroke-dashoffset 600ms cubic-bezier(0.2,0.6,0.4,1)"
            />
          </svg>
          <span class="absolute inset-0 flex items-center justify-center">
            <span class="live-dot h-1.5 w-1.5 rounded-full bg-accent"></span>
          </span>
        </div>
        <div
          class="rounded-xl border p-3"
          style="border-color: rgba(0,214,143,0.2); background: rgba(0,214,143,0.04)"
        >
          <div class="flex items-baseline justify-between">
            <div>
              <div class="text-[15px] font-semibold text-text">{s.label}</div>
              <div class="mt-0.5 text-[12px] text-text-2">{s.desc}</div>
            </div>
            <span class="font-mono text-[13px] font-semibold text-accent">
              {formatProgress(job.progress)}
            </span>
          </div>
          <div class="mt-3">
            <ProgressBar value={job.progress} height={4} animated />
          </div>
          {#if job.speed || job.eta_seconds !== undefined}
            <div class="mt-3 flex items-center justify-between">
              <SpeedEtaChip speed={job.speed} etaSeconds={job.eta_seconds} />
              <LiveDot label="STREAMING" />
            </div>
          {/if}
        </div>
      </div>
    {:else}
      <div class="relative flex min-h-[40px] items-center py-2 pl-12 pr-3">
        <div
          class="absolute left-[10px] top-1/2 h-3 w-3 -translate-y-1/2 rounded-full"
          style="background: var(--surface-1); border: 1px solid #2a2a30"
        ></div>
        <div class="flex flex-1 items-center justify-between">
          <span class="text-[14px] text-text-3">{s.label}</span>
          <span class="font-mono text-[11px] text-text-3 opacity-60">queued</span>
        </div>
      </div>
    {/if}
  {/each}
</div>
