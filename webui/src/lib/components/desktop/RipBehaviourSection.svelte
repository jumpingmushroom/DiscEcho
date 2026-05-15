<script lang="ts">
  import { settings, updateRipBehaviour } from '$lib/store';
  import { pushToast } from '$lib/toasts';

  $: modeInit = (($settings['operation.mode'] as string) === 'manual' ? 'manual' : 'batch') as
    | 'batch'
    | 'manual';
  $: ejectInit =
    $settings['rip.eject_on_finish'] === undefined
      ? true
      : ($settings['rip.eject_on_finish'] as string) === 'true';

  let mode: 'batch' | 'manual' = modeInit;
  let ejectOnFinish: boolean = ejectInit;
  let saving = false;
  let error: string | null = null;

  // Seed working copy from the loaded settings exactly once. Once the
  // user starts editing we stop tracking external changes so a stale
  // SSE update doesn't overwrite mid-edit.
  let initialized = false;
  $: if (!initialized && $settings['operation.mode'] !== undefined) {
    mode = modeInit;
    ejectOnFinish = ejectInit;
    initialized = true;
  }

  async function onSave(): Promise<void> {
    error = null;
    saving = true;
    try {
      await updateRipBehaviour({ mode, ejectOnFinish });
      pushToast('success', 'Rip behaviour saved');
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<section class="rounded-2xl border border-border bg-surface-1 p-5">
  <h2 class="text-[14px] font-semibold text-text">Rip behaviour</h2>
  <p class="mt-1 text-[12px] text-text-3">
    Batch auto-confirms identified discs after 8 s and (optionally) ejects when the rip finishes.
    Manual leaves both actions to you — pick a candidate, click Start, click Eject.
  </p>
  <div class="mt-4 space-y-3">
    <div class="flex flex-col gap-1">
      <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Mode</span>
      <div class="inline-flex rounded-md border border-border bg-surface-2 p-0.5">
        <label
          class="cursor-pointer rounded-sm px-3 py-1 text-[13px]"
          style="background: {mode === 'batch' ? 'var(--accent)' : 'transparent'}; color: {mode ===
          'batch'
            ? 'black'
            : 'var(--text-2)'}"
        >
          <input type="radio" bind:group={mode} value="batch" class="hidden" />
          Batch
        </label>
        <label
          class="cursor-pointer rounded-sm px-3 py-1 text-[13px]"
          style="background: {mode === 'manual' ? 'var(--accent)' : 'transparent'}; color: {mode ===
          'manual'
            ? 'black'
            : 'var(--text-2)'}"
        >
          <input type="radio" bind:group={mode} value="manual" class="hidden" />
          Manual
        </label>
      </div>
    </div>

    <label
      class="flex items-center gap-2 text-[12px]"
      style="color: {mode === 'manual' ? 'var(--text-3)' : 'var(--text-2)'}"
    >
      <input type="checkbox" bind:checked={ejectOnFinish} disabled={mode === 'manual'} />
      Eject the tray when a rip finishes
    </label>
    {#if mode === 'manual'}
      <div class="text-[11px] text-text-3">
        Disabled — manual mode never auto-ejects. Click Eject on the drive card instead.
      </div>
    {/if}

    {#if error}<div class="text-[12px] text-error">{error}</div>{/if}
    <button
      on:click={onSave}
      disabled={saving}
      class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-semibold text-black disabled:opacity-50"
    >
      Save
    </button>
  </div>
</section>
