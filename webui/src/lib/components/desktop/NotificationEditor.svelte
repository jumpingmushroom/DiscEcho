<script lang="ts">
  import type { Notification } from '$lib/wire';
  import {
    createNotification,
    updateNotification,
    deleteNotification,
    validateNotification,
    testNotification,
  } from '$lib/store';
  import { parseValidationErrors, type ValidationErrors } from '$lib/api';
  import { pushToast } from '$lib/toasts';
  import { createEventDispatcher } from 'svelte';
  import Field from './Field.svelte';

  export let notification: Notification | null = null;
  export let creating: boolean = false;

  const dispatch = createEventDispatcher<{ saved: void }>();

  const TRIGGERS = ['done', 'failed', 'warn'] as const;

  let working: Notification = blank();
  let original: Notification = blank();
  let fieldErrors: ValidationErrors = {};
  let saveError: string | null = null;
  let saving = false;
  let confirmingDelete = false;

  // Re-seed working copy whenever the parent swaps the notification or toggles
  // creating mode. Keyed on id + creating so mid-edit re-renders of the same
  // row don't blow away in-progress edits.
  let lastKey = '';
  $: {
    const key = (notification?.id ?? '') + ':' + (creating ? '1' : '0');
    if (key !== lastKey) {
      lastKey = key;
      working = notification ? clone(notification) : blank();
      original = clone(working);
      fieldErrors = {};
      saveError = null;
      confirmingDelete = false;
    }
  }

  $: dirty = JSON.stringify(working) !== JSON.stringify(original);

  function blank(): Notification {
    return {
      id: '',
      name: '',
      url: '',
      tags: '',
      triggers: 'done,failed',
      enabled: true,
      created_at: '',
      updated_at: '',
    };
  }

  function clone(n: Notification): Notification {
    return JSON.parse(JSON.stringify(n));
  }

  function triggerSet(): Set<string> {
    return new Set(
      working.triggers
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean),
    );
  }

  function toggleTrigger(t: string, on: boolean): void {
    const s = triggerSet();
    if (on) s.add(t);
    else s.delete(t);
    working.triggers = Array.from(s).join(',');
  }

  function onTriggerChange(t: string, e: Event): void {
    toggleTrigger(t, (e.currentTarget as HTMLInputElement).checked);
  }

  async function onSave(): Promise<void> {
    saving = true;
    fieldErrors = {};
    saveError = null;
    try {
      if (creating) {
        const rest: Omit<Notification, 'id' | 'created_at' | 'updated_at'> = {
          name: working.name,
          url: working.url,
          tags: working.tags,
          triggers: working.triggers,
          enabled: working.enabled,
        };
        const fresh = await createNotification(rest);
        working = clone(fresh);
        original = clone(fresh);
      } else {
        const fresh = await updateNotification(working.id, working);
        working = clone(fresh);
        original = clone(fresh);
      }
      pushToast('success', creating ? 'Notification created' : 'Notification saved');
      dispatch('saved');
    } catch (e) {
      const fe = parseValidationErrors(e);
      if (fe) {
        fieldErrors = fe;
      } else {
        saveError = (e as Error).message;
      }
    } finally {
      saving = false;
    }
  }

  async function onValidate(): Promise<void> {
    try {
      const res = await validateNotification(working.id);
      if (res.ok) {
        pushToast('success', 'URL is valid.');
      } else {
        pushToast('error', res.error ?? 'validation failed');
      }
    } catch (e) {
      pushToast('error', (e as Error).message);
    }
  }

  async function onTest(): Promise<void> {
    const res = await testNotification(working.id);
    if (res.sent) {
      pushToast('success', 'Test notification sent.');
    } else {
      pushToast('error', res.error ?? 'send failed');
    }
  }

  async function onDelete(): Promise<void> {
    if (!confirmingDelete) {
      confirmingDelete = true;
      return;
    }
    saving = true;
    saveError = null;
    try {
      await deleteNotification(working.id);
      confirmingDelete = false;
      pushToast('success', 'Notification deleted');
      dispatch('saved');
    } catch (e) {
      saveError = (e as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<div class="rounded-2xl border border-border bg-surface-1 p-4">
  <div class="space-y-3">
    <Field label="Name" error={fieldErrors.name}>
      <input
        type="text"
        bind:value={working.name}
        class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
      />
    </Field>

    <Field label="URL" error={fieldErrors.url}>
      <textarea
        bind:value={working.url}
        rows="2"
        class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 font-mono text-[12px] text-text"
      ></textarea>
    </Field>

    <div>
      <span class="mb-1 block text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
        Triggers
      </span>
      <div class="flex gap-3 text-[12px] text-text-2">
        {#each TRIGGERS as t}
          <label class="flex items-center gap-1">
            <input
              type="checkbox"
              checked={triggerSet().has(t)}
              on:change={(e) => onTriggerChange(t, e)}
            />
            {t}
          </label>
        {/each}
      </div>
      {#if fieldErrors.triggers}
        <div class="mt-1 text-[11px] text-error">{fieldErrors.triggers}</div>
      {/if}
    </div>

    <label class="flex items-center gap-2">
      <input type="checkbox" bind:checked={working.enabled} />
      <span class="text-[13px] text-text-2">Enabled</span>
    </label>

    {#if saveError}
      <div class="rounded-md border border-error px-3 py-2 text-[12px] text-error">
        {saveError}
      </div>
    {/if}

    <div class="flex flex-wrap items-center gap-2">
      <button
        on:click={onSave}
        disabled={saving}
        class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-semibold text-black disabled:opacity-50"
      >
        {creating ? 'Create' : 'Save'}
      </button>
      {#if !creating}
        <button
          on:click={onValidate}
          disabled={saving || dirty}
          class="rounded-md border border-border px-3 py-1.5 text-[12px] text-text-2 disabled:opacity-50"
        >
          Validate
        </button>
        <button
          on:click={onTest}
          disabled={saving || dirty}
          class="rounded-md border border-border px-3 py-1.5 text-[12px] text-text-2 disabled:opacity-50"
        >
          Test
        </button>
        <button
          on:click={onDelete}
          disabled={saving}
          class="rounded-md border border-border px-3 py-1.5 text-[12px] text-text-2 hover:border-error hover:text-error disabled:opacity-50"
        >
          {confirmingDelete ? 'Confirm delete' : 'Delete'}
        </button>
      {/if}
    </div>
  </div>
</div>
