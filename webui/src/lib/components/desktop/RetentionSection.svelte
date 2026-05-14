<script lang="ts">
  import { settings, updateRetention } from '$lib/store';
  import { pushToast } from '$lib/toasts';

  $: foreverInit = ($settings['retention.forever'] as string) === 'true';
  $: daysInit = parseInt(($settings['retention.days'] as string) ?? '30', 10) || 30;

  let forever: boolean = foreverInit;
  let days: number = daysInit;
  let saving = false;
  let error: string | null = null;

  // Re-seed working copy when settings load (or change), but only
  // until the user starts editing.
  let initialized = false;
  $: if (!initialized && $settings['retention.forever'] !== undefined) {
    forever = foreverInit;
    days = daysInit;
    initialized = true;
  }

  async function onSave(): Promise<void> {
    if (!forever && (!days || days < 1)) {
      error = 'Days must be at least 1.';
      return;
    }
    error = null;
    saving = true;
    try {
      await updateRetention({ forever, days });
      pushToast('success', 'Retention settings saved');
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<section class="rounded-2xl border border-border bg-surface-1 p-5">
  <h2 class="text-[14px] font-semibold text-text">History retention</h2>
  <div class="mt-4 space-y-3">
    <label class="flex items-center gap-2 text-[12px] text-text-2">
      <input type="checkbox" bind:checked={forever} />
      Keep history forever
    </label>
    {#if !forever}
      <label class="flex items-center gap-2 text-[12px] text-text-2">
        Delete history after
        <input
          type="number"
          min="1"
          bind:value={days}
          class="w-20 rounded-md border border-border bg-surface-2 px-2 py-1 text-[13px] text-text"
        />
        days
      </label>
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
