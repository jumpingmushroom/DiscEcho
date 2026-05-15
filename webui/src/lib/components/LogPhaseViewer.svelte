<script lang="ts">
  import { onMount } from 'svelte';
  import { logs, fetchJobLogs, type LogLine } from '$lib/store';
  import type { StepID } from '$lib/wire';

  // jobID identifies the job being viewed.
  export let jobID: string;
  // live=true reads from the in-memory SSE ring ($logs); false fetches
  // persisted lines from the daemon. Set false for terminal jobs and
  // for the "I just hit reload" case on a running job (the ring starts
  // empty after a reload).
  export let live: boolean = true;
  // activeStep is the running job's current step. When provided and
  // the user hasn't manually picked a filter, the viewer auto-selects
  // it so the chip the user is watching matches the active phase.
  export let activeStep: StepID | undefined = undefined;
  // defaultStep is the chip selected on first render. '' means "All".
  // Used by the terminal-state branch on /jobs/[id] to default to All
  // for a finished job, since there is no live phase to follow.
  export let defaultStep: StepID | '' = '';

  // Per-step ordering matches the dashboard's PipelineStepperVertical.
  const STEP_ORDER: StepID[] = [
    'detect',
    'identify',
    'rip',
    'transcode',
    'compress',
    'move',
    'notify',
    'eject',
  ];

  const STEP_LABEL: Record<StepID, string> = {
    detect: 'Detect',
    identify: 'Identify',
    rip: 'Rip',
    transcode: 'Transcode',
    compress: 'Compress',
    move: 'Move',
    notify: 'Notify',
    eject: 'Eject',
  };

  let selected: StepID | '' = defaultStep;
  // userPicked stays false until the user clicks a chip. While false,
  // live-mode auto-tracks activeStep; once the user clicks, we stop
  // hijacking their selection.
  let userPicked: boolean = false;
  // terminalLines holds the fetched payload for non-live mode. Replaced
  // wholesale on each fetch — pagination is room to grow later, but
  // even 2000 lines (the daemon cap) is fine for the first paint.
  let terminalLines: LogLine[] = [];
  let terminalLoading: boolean = false;
  let terminalError: string | null = null;

  // For live mode auto-track: as soon as activeStep moves, follow it
  // unless the user has manually picked a chip.
  $: if (live && !userPicked && activeStep && activeStep !== selected) {
    selected = activeStep;
  }

  // Source feed: live → in-memory ring; terminal → fetched lines.
  $: feed = live ? ($logs[jobID] ?? []) : terminalLines;

  $: counts = computeCounts(feed);
  $: visible = filterLines(feed, selected);

  function computeCounts(lines: LogLine[]): Record<StepID | '', number> {
    const out: Record<StepID | '', number> = {
      '': lines.length,
      detect: 0,
      identify: 0,
      rip: 0,
      transcode: 0,
      compress: 0,
      move: 0,
      notify: 0,
      eject: 0,
    };
    for (const l of lines) {
      const s = (l.step ?? '') as StepID | '';
      if (s && s in out) out[s] += 1;
    }
    return out;
  }

  function filterLines(lines: LogLine[], step: StepID | ''): LogLine[] {
    if (!step) return lines;
    return lines.filter((l) => (l.step ?? '') === step);
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

  function pickStep(step: StepID | ''): void {
    selected = step;
    userPicked = true;
  }

  async function loadTerminal(): Promise<void> {
    terminalLoading = true;
    terminalError = null;
    try {
      const res = await fetchJobLogs(jobID, { limit: 2000 });
      terminalLines = res.lines ?? [];
    } catch (e) {
      terminalError = (e as Error).message;
    } finally {
      terminalLoading = false;
    }
  }

  onMount(() => {
    if (!live) void loadTerminal();
  });
</script>

<div class="rounded-2xl border border-border bg-surface-1">
  <div class="flex items-center gap-2 overflow-x-auto px-3 py-2">
    <button
      type="button"
      class="chip min-h-[36px] whitespace-nowrap rounded-full border px-3 text-[12px] font-medium"
      class:active={selected === ''}
      on:click={() => pickStep('')}
    >
      All <span class="ml-1 text-[10px] text-text-3">{counts['']}</span>
    </button>
    {#each STEP_ORDER as step (step)}
      {#if counts[step] > 0}
        <button
          type="button"
          class="chip min-h-[36px] whitespace-nowrap rounded-full border px-3 text-[12px] font-medium"
          class:active={selected === step}
          on:click={() => pickStep(step)}
        >
          {STEP_LABEL[step]} <span class="ml-1 text-[10px] text-text-3">{counts[step]}</span>
        </button>
      {/if}
    {/each}
  </div>

  <div
    class="max-h-[280px] overflow-auto border-t border-border p-3 font-mono text-[11px] leading-[1.7]"
  >
    {#if !live && terminalLoading}
      <div class="text-text-3">Loading log…</div>
    {:else if !live && terminalError}
      <div style="color: var(--error)">Couldn't load log: {terminalError}</div>
    {:else if visible.length === 0}
      <div class="text-text-3">No log lines{selected ? ` for ${STEP_LABEL[selected]}` : ''}.</div>
    {:else}
      {#each visible as line, i (i)}
        <div class="flex gap-2">
          <span class="shrink-0 text-text-3">{line.t.slice(11, 19)}</span>
          <span
            class="w-9 shrink-0 uppercase"
            style="color: {line.level === 'warn'
              ? 'var(--warn)'
              : line.level === 'error'
                ? 'var(--error)'
                : 'var(--info)'}"
          >
            {line.level}
          </span>
          <span class="flex-1 break-words text-text-2" style="color: {levelColour(line.level)}">
            {line.message}
          </span>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .chip {
    background: var(--surface-1);
    color: var(--text-2);
    border-color: var(--border);
  }
  .chip.active {
    background: var(--accent-soft);
    color: var(--accent);
    border-color: rgba(0, 214, 143, 0.3);
  }
</style>
