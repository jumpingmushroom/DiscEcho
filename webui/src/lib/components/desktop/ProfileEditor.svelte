<script lang="ts">
  import type { Profile } from '$lib/wire';
  import { createProfile, updateProfile, deleteProfile } from '$lib/store';
  import { parseValidationErrors, type ValidationErrors } from '$lib/api';
  import { ENGINES, DISC_TYPES, engineNames, specFor } from '$lib/profile_schema';
  import { createEventDispatcher } from 'svelte';
  import Field from './Field.svelte';

  export let profile: Profile | null = null;
  export let creating: boolean = false;

  const dispatch = createEventDispatcher<{ saved: void }>();

  let working: Profile = blank();
  let fieldErrors: ValidationErrors = {};
  let genericError: string | null = null;
  let saving = false;
  let confirmingDelete = false;

  // Re-seed working copy whenever the parent swaps profile or toggles
  // creating mode. Keyed on profile?.id + creating so mid-edit
  // re-renders of the same profile don't blow away the working copy.
  let lastKey = '';
  $: {
    const key = (profile?.id ?? '') + ':' + (creating ? '1' : '0');
    if (key !== lastKey) {
      lastKey = key;
      if (profile) {
        working = clone(profile);
      } else if (creating) {
        working = blank();
      }
      fieldErrors = {};
      genericError = null;
      confirmingDelete = false;
    }
  }

  $: spec = specFor(working.engine);
  $: validFormats = spec?.formats ?? [working.format];
  $: optionKeys = spec ? Object.keys(spec.options) : [];

  function blank(): Profile {
    return {
      id: '',
      disc_type: 'AUDIO_CD',
      name: '',
      engine: 'whipper',
      format: 'FLAC',
      preset: '',
      options: {},
      output_path_template: '',
      enabled: true,
      step_count: ENGINES.whipper.stepCount,
      created_at: '',
      updated_at: '',
    };
  }

  function clone(p: Profile): Profile {
    return JSON.parse(JSON.stringify(p));
  }

  function onEngineChange(): void {
    const s = specFor(working.engine);
    if (s) {
      if (!s.formats.includes(working.format)) working.format = s.formats[0];
      working.step_count = s.stepCount;
      // Drop options not in the new schema.
      const filtered: Record<string, unknown> = {};
      for (const k of Object.keys(working.options)) {
        if (s.options[k]) filtered[k] = working.options[k];
      }
      working.options = filtered;
    }
  }

  async function onSave(): Promise<void> {
    saving = true;
    fieldErrors = {};
    genericError = null;
    try {
      if (creating) {
        const rest: Omit<Profile, 'id' | 'created_at' | 'updated_at'> = {
          disc_type: working.disc_type,
          name: working.name,
          engine: working.engine,
          format: working.format,
          preset: working.preset,
          options: working.options,
          output_path_template: working.output_path_template,
          enabled: working.enabled,
          step_count: working.step_count,
        };
        await createProfile(rest);
      } else {
        await updateProfile(working.id, working);
      }
      dispatch('saved');
    } catch (e) {
      const fe = parseValidationErrors(e);
      if (fe) {
        fieldErrors = fe;
      } else {
        genericError = (e as Error).message;
      }
    } finally {
      saving = false;
    }
  }

  async function onDelete(): Promise<void> {
    if (!confirmingDelete) {
      confirmingDelete = true;
      return;
    }
    saving = true;
    genericError = null;
    try {
      await deleteProfile(working.id);
      confirmingDelete = false;
      dispatch('saved');
    } catch (e) {
      genericError = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  function setOption(key: string, value: unknown): void {
    working.options = { ...working.options, [key]: value };
  }

  function onOptionBoolChange(key: string, e: Event): void {
    setOption(key, (e.currentTarget as HTMLInputElement).checked);
  }

  function onOptionIntInput(key: string, e: Event): void {
    setOption(key, Number((e.currentTarget as HTMLInputElement).value));
  }

  function onOptionStringInput(key: string, e: Event): void {
    setOption(key, (e.currentTarget as HTMLInputElement).value);
  }
</script>

{#if !profile && !creating}
  <div class="rounded-2xl border border-border bg-surface-1 p-12 text-center">
    <div class="text-[14px] font-medium text-text-2">Select a profile to edit</div>
    <div class="mt-2 text-[12px] text-text-3">or click "+ New profile" to create one.</div>
  </div>
{:else}
  <form class="rounded-2xl border border-border bg-surface-1 p-5" on:submit|preventDefault={onSave}>
    {#if genericError}
      <div
        class="mb-4 rounded-md border border-error/30 bg-error/10 px-3 py-2 text-[12px] text-error"
      >
        {genericError}
      </div>
    {/if}

    <div class="space-y-4">
      <Field label="Name" error={fieldErrors.name}>
        <input
          name="name"
          type="text"
          bind:value={working.name}
          class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
        />
      </Field>

      <Field label="Disc type" error={fieldErrors.disc_type}>
        <select
          name="disc_type"
          bind:value={working.disc_type}
          disabled={!creating}
          class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text disabled:opacity-60"
        >
          {#each DISC_TYPES as dt}
            <option value={dt}>{dt}</option>
          {/each}
        </select>
      </Field>

      <Field label="Engine" error={fieldErrors.engine}>
        <select
          name="engine"
          bind:value={working.engine}
          on:change={onEngineChange}
          disabled={!creating}
          class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text disabled:opacity-60"
        >
          {#each engineNames() as e}
            <option value={e}>{e}</option>
          {/each}
        </select>
      </Field>

      <Field label="Format" error={fieldErrors.format}>
        <select
          name="format"
          bind:value={working.format}
          class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
        >
          {#each validFormats as f}
            <option value={f}>{f}</option>
          {/each}
        </select>
      </Field>

      <Field label="Preset" error={fieldErrors.preset}>
        <input
          name="preset"
          type="text"
          bind:value={working.preset}
          class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
        />
      </Field>

      <Field label="Output path template" error={fieldErrors.output_path_template}>
        <textarea
          name="output_path_template"
          bind:value={working.output_path_template}
          rows="2"
          class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 font-mono text-[12px] text-text"
        ></textarea>
      </Field>

      <label class="flex items-center gap-2">
        <input type="checkbox" bind:checked={working.enabled} />
        <span class="text-[13px] text-text-2">Enabled</span>
      </label>

      {#if optionKeys.length > 0}
        <div>
          <div class="mb-2 text-[10px] font-medium uppercase tracking-[0.14em] text-text-3">
            Options
          </div>
          {#each optionKeys as k}
            {@const opt = spec?.options[k]}
            <div class="mb-2">
              <label class="block">
                <span class="mb-1 block text-[12px] font-medium text-text-2">{k}</span>
                {#if opt?.type === 'bool'}
                  <input
                    type="checkbox"
                    checked={Boolean(working.options[k])}
                    on:change={(e) => onOptionBoolChange(k, e)}
                  />
                {:else if opt?.type === 'int'}
                  <input
                    type="number"
                    value={Number(working.options[k] ?? 0)}
                    on:input={(e) => onOptionIntInput(k, e)}
                    class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
                  />
                {:else}
                  <input
                    type="text"
                    value={String(working.options[k] ?? '')}
                    on:input={(e) => onOptionStringInput(k, e)}
                    class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
                  />
                {/if}
              </label>
              {#if fieldErrors[`options.${k}`]}
                <div class="mt-1 text-[11px] text-error">{fieldErrors[`options.${k}`]}</div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}

      <div class="text-[11px] text-text-3">Step count: {working.step_count}</div>
    </div>

    <div class="mt-6 flex items-center gap-3">
      <button
        type="submit"
        disabled={saving}
        class="rounded-md bg-accent px-4 py-2 text-[13px] font-semibold text-black disabled:opacity-50"
      >
        {creating ? 'Create' : 'Save changes'}
      </button>
      {#if !creating}
        <button
          type="button"
          on:click={onDelete}
          disabled={saving}
          class="rounded-md border border-border px-4 py-2 text-[13px] text-text-2 hover:border-error hover:text-error disabled:opacity-50"
        >
          {confirmingDelete ? 'Confirm delete' : 'Delete'}
        </button>
      {/if}
    </div>
  </form>
{/if}
