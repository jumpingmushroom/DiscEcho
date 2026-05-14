<script lang="ts">
  import type { Job, StepID } from '$lib/wire';
  import Icon from '$lib/icons/Icon.svelte';

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

  type Status = 'done' | 'active' | 'pending' | 'skipped' | 'failed';

  function statusFor(step: StepID): Status {
    const stp = job.steps?.find((s) => s.step === step);
    if (stp?.state === 'skipped') return 'skipped';
    if (stp?.state === 'done') return 'done';
    if (stp?.state === 'failed') return 'failed';
    if (job.active_step === step) return 'active';
    return 'pending';
  }

  $: activeStep = STEPS.find((s) => s.id === job.active_step);
  // SVG ring math: r=11, circumference for the active dot's arc.
  const RING_R = 11;
  const RING_C = 2 * Math.PI * RING_R;
  $: dashOffset = RING_C - (RING_C * (job.progress ?? 0)) / 100;
</script>

<div class="w-full">
  <!-- dots row -->
  <div class="flex items-center px-1">
    {#each STEPS as s, i}
      {@const st = statusFor(s.id)}
      {@const isLast = i === STEPS.length - 1}
      <div class="flex flex-col items-center" style="min-width: 40px">
        <div class="relative h-6 w-6" data-step={s.id} data-step-state={st}>
          {#if st === 'done'}
            <div
              class="pop-in flex h-6 w-6 items-center justify-center rounded-full"
              style="background: var(--accent-soft); border: 1px solid rgba(var(--accent-rgb),0.4)"
            >
              <Icon name="check" size={13} stroke="var(--accent)" strokeWidth={2.5} />
            </div>
          {:else if st === 'failed'}
            <div
              class="flex h-6 w-6 items-center justify-center rounded-full"
              style="background: rgba(255,91,91,0.12); border: 1px solid rgba(255,91,91,0.4)"
            >
              <Icon name="x" size={13} stroke="var(--error)" strokeWidth={2.5} />
            </div>
          {:else if st === 'active'}
            <svg width="24" height="24" viewBox="0 0 24 24" class="absolute inset-0">
              <circle
                cx="12"
                cy="12"
                r={RING_R}
                fill="var(--accent-soft)"
                stroke="rgba(var(--accent-rgb),0.2)"
                stroke-width="1"
              />
              <circle
                cx="12"
                cy="12"
                r={RING_R}
                fill="none"
                stroke="var(--accent)"
                stroke-width="2"
                stroke-dasharray={RING_C}
                stroke-dashoffset={dashOffset}
                transform="rotate(-90 12 12)"
                stroke-linecap="round"
                style="transition: stroke-dashoffset 600ms cubic-bezier(0.2,0.6,0.4,1)"
              />
            </svg>
            <span class="absolute inset-0 flex items-center justify-center">
              <span class="live-dot"></span>
            </span>
          {:else}
            <div
              class="flex h-6 w-6 items-center justify-center rounded-full"
              style="background: var(--surface-0); border: 1px solid #2a2a30"
            >
              <span class="h-1 w-1 rounded-full" style="background: #3f3f46"></span>
            </div>
          {/if}
        </div>
      </div>
      {#if !isLast}
        {@const nextSt = statusFor(STEPS[i + 1].id)}
        <div class="relative mx-1 flex-1" style="height: 1px">
          <div class="absolute inset-0 rounded-full" style="background: var(--surface-3)"></div>
          <div
            class="absolute inset-y-0 left-0 rounded-full transition-all duration-700"
            style="
              width: {st === 'done' || (st === 'active' && nextSt !== 'pending')
              ? '100%'
              : st === 'active'
                ? '50%'
                : '0%'};
              background: var(--accent);
            "
          ></div>
        </div>
      {/if}
    {/each}
  </div>

  <!-- labels row -->
  <div class="mt-2.5 flex items-start px-1">
    {#each STEPS as s, i}
      {@const st = statusFor(s.id)}
      <div class="flex flex-col items-center" style="min-width: 40px">
        <span
          class="whitespace-nowrap text-[11px] font-medium"
          style="color: {st === 'active'
            ? 'var(--accent)'
            : st === 'done'
              ? 'var(--text-2)'
              : 'var(--text-3)'}"
        >
          {s.label}
        </span>
      </div>
      {#if i < STEPS.length - 1}<div class="flex-1"></div>{/if}
    {/each}
  </div>

  <!-- meta row: names the active step. Speed/ETA for the step is shown
       once, with the progress bar in the parent (RipCard) — it's the
       current step's value, so duplicating it here said nothing new. -->
  {#if activeStep}
    <div class="mt-3 flex items-center gap-2 border-t pt-3" style="border-color: var(--surface-3)">
      <span class="text-[11px] uppercase tracking-[0.12em] text-text-3">Active</span>
      <span class="text-[12px] font-medium text-text-2">
        {activeStep.label} — {activeStep.desc}
      </span>
    </div>
  {/if}
</div>
