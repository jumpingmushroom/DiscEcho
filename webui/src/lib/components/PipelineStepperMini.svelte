<script lang="ts">
  import type { Job, StepID } from '$lib/wire';
  export let job: Job;

  const STEPS: StepID[] = [
    'detect',
    'identify',
    'rip',
    'transcode',
    'compress',
    'move',
    'notify',
    'eject',
  ];

  const TERMINAL_STATES = new Set(['done', 'failed', 'cancelled', 'interrupted']);

  function statusFor(step: StepID): 'done' | 'active' | 'pending' | 'skipped' {
    const stp = job.steps?.find((s) => s.step === step);
    if (stp?.state === 'skipped') return 'skipped';
    if (stp?.state === 'done') return 'done';
    // Stale 'running' on a terminal job means the daemon was killed
    // mid-step. Render as done so the mini dot row doesn't keep
    // showing an animated active indicator forever.
    if (stp?.state === 'running' && TERMINAL_STATES.has(job.state)) return 'done';
    if (job.active_step === step) return 'active';
    if (stp?.state === 'failed') return 'done';
    return 'pending';
  }

  // Hide steps the pipeline marked as skipped (audio CD has no
  // separate Transcode/Compress — whipper encodes FLAC inside Rip).
  $: visibleSteps = STEPS.filter((s) => statusFor(s) !== 'skipped');
</script>

<div class="flex items-center gap-[3px]">
  {#each visibleSteps as step}
    {@const status = statusFor(step)}
    <span
      class="rounded-full transition-colors"
      style="
        width: {status === 'active' ? '14px' : '6px'};
        height: 6px;
        background: {status === 'done' || status === 'active' ? 'var(--accent)' : '#2a2a30'};
        opacity: {status === 'pending' || status === 'skipped' ? 0.5 : 1};
        box-shadow: {status === 'active' ? '0 0 0 3px var(--accent-soft)' : 'none'};
      "
    ></span>
  {/each}
</div>
