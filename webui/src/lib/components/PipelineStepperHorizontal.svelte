<script lang="ts">
  import type { Job, StepID } from '$lib/wire';

  export let job: Job;

  const STEPS: Array<{ id: StepID; label: string }> = [
    { id: 'detect', label: 'Detect' },
    { id: 'identify', label: 'Identify' },
    { id: 'rip', label: 'Rip' },
    { id: 'transcode', label: 'Transcode' },
    { id: 'compress', label: 'Compress' },
    { id: 'move', label: 'Move' },
    { id: 'notify', label: 'Notify' },
    { id: 'eject', label: 'Eject' },
  ];

  function statusFor(step: StepID): 'done' | 'active' | 'pending' | 'skipped' | 'failed' {
    const stp = job.steps?.find((s) => s.step === step);
    if (stp?.state === 'skipped') return 'skipped';
    if (stp?.state === 'done') return 'done';
    if (stp?.state === 'failed') return 'failed';
    if (job.active_step === step) return 'active';
    return 'pending';
  }
</script>

<div class="flex items-start justify-between">
  {#each STEPS as s, i}
    {@const st = statusFor(s.id)}
    <div class="flex flex-1 flex-col items-center gap-2" data-step={s.id} data-step-state={st}>
      <span
        class="text-[10px] font-medium uppercase tracking-[0.14em]"
        style="color: {st === 'active'
          ? 'var(--accent)'
          : st === 'done'
            ? 'var(--text-2)'
            : 'var(--text-3)'}"
      >
        {s.label}
      </span>
      <div class="relative flex w-full items-center justify-center">
        {#if i > 0}
          <div class="absolute left-0 right-1/2 h-px" style="background: var(--border)"></div>
        {/if}
        {#if i < STEPS.length - 1}
          <div class="absolute left-1/2 right-0 h-px" style="background: var(--border)"></div>
        {/if}
        <span
          class="relative h-3 w-3 rounded-full"
          style="
            background: {st === 'done' || st === 'active'
            ? 'var(--accent)'
            : st === 'failed'
              ? 'var(--error)'
              : 'transparent'};
            border: 1px solid {st === 'done' || st === 'active'
            ? 'var(--accent)'
            : st === 'failed'
              ? 'var(--error)'
              : '#2a2a30'};
            box-shadow: {st === 'active' ? '0 0 0 4px var(--accent-soft)' : 'none'};
            opacity: {st === 'pending' || st === 'skipped' ? 0.7 : 1};
          "
        ></span>
      </div>
    </div>
  {/each}
</div>
