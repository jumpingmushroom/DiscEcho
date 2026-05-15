<script lang="ts">
  import type { Profile, DiscType } from '$lib/wire';
  import { createProfile, updateProfile, deleteProfile } from '$lib/store';
  import { parseValidationErrors, type ValidationErrors } from '$lib/api';
  import { pushToast } from '$lib/toasts';
  import {
    ENGINES,
    DISC_TYPES,
    HDR_PIPELINES,
    HDR_PIPELINE_LABELS,
    DRIVE_POLICIES,
    DRIVE_POLICY_LABELS,
    engineNames,
    specFor,
  } from '$lib/profile_schema';
  import { createEventDispatcher } from 'svelte';
  import DiscTypeBadge, { DISC_TYPE_META } from '$lib/components/DiscTypeBadge.svelte';
  import FormSection from './FormSection.svelte';
  import FormRow from './FormRow.svelte';
  import PathField from './PathField.svelte';

  export let profile: Profile | null = null;
  export let creating: boolean = false;
  // `chromeless` strips the in-form header (name input + duplicate / save /
  // enabled checkbox) and the in-form Delete button. The mobile drill-down
  // route (`/profiles/[id]`) renders its own AppBar and sticky action bar
  // and drives onSave / onDelete via component bindings instead.
  export let chromeless: boolean = false;

  const dispatch = createEventDispatcher<{ saved: void; duplicate: Profile }>();

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
  $: validContainers = spec?.containers ?? [working.container || working.format || ''];
  $: validVideoCodecs = spec?.videoCodecs ?? [];
  $: optionKeys = spec ? Object.keys(spec.options) : [];
  $: discType = working.disc_type as DiscType;
  $: discMeta = DISC_TYPE_META[discType] ?? null;

  function blank(): Profile {
    return {
      id: '',
      disc_type: 'AUDIO_CD',
      name: '',
      engine: 'whipper',
      format: 'FLAC',
      preset: '',
      container: 'FLAC',
      video_codec: '',
      quality_preset: '',
      hdr_pipeline: '',
      drive_policy: 'any',
      auto_eject: true,
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
      if (!s.containers.includes(working.container)) {
        working.container = s.containers[0] ?? '';
      }
      working.format = working.container;
      if (s.videoCodecs.length === 0) {
        working.video_codec = '';
      } else if (!s.videoCodecs.includes(working.video_codec)) {
        working.video_codec = s.videoCodecs[0];
      }
      working.step_count = s.stepCount;
      // Drop options not in the new schema.
      const filtered: Record<string, unknown> = {};
      for (const k of Object.keys(working.options)) {
        if (s.options[k]) filtered[k] = working.options[k];
      }
      working.options = filtered;
    }
  }

  function onContainerChange(): void {
    // Keep the deprecated `format` mirror in sync for the one-release
    // window where the daemon still falls back to it.
    working.format = working.container;
  }

  export async function onSave(): Promise<void> {
    saving = true;
    fieldErrors = {};
    genericError = null;
    try {
      const payload: Omit<Profile, 'id' | 'created_at' | 'updated_at'> = {
        disc_type: working.disc_type,
        name: working.name,
        engine: working.engine,
        format: working.container,
        preset: working.quality_preset,
        container: working.container,
        video_codec: working.video_codec,
        quality_preset: working.quality_preset,
        hdr_pipeline: working.hdr_pipeline,
        drive_policy: working.drive_policy,
        auto_eject: working.auto_eject,
        options: working.options,
        output_path_template: working.output_path_template,
        enabled: working.enabled,
        step_count: working.step_count,
      };
      if (creating) {
        await createProfile(payload);
      } else {
        await updateProfile(working.id, { ...payload, id: working.id } as Profile);
      }
      pushToast('success', creating ? 'Profile created' : 'Profile saved');
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

  export async function onDelete(): Promise<void> {
    if (!confirmingDelete) {
      confirmingDelete = true;
      return;
    }
    saving = true;
    genericError = null;
    try {
      await deleteProfile(working.id);
      confirmingDelete = false;
      pushToast('success', 'Profile deleted');
      dispatch('saved');
    } catch (e) {
      genericError = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  function onDuplicate(): void {
    const copy = clone(working);
    copy.id = '';
    copy.name = working.name ? `${working.name} (copy)` : '';
    copy.created_at = '';
    copy.updated_at = '';
    dispatch('duplicate', copy);
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

  function onOutputPathChange(e: CustomEvent<string>): void {
    working.output_path_template = e.detail;
  }

  const inputClass =
    'w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text';
  const inputDisabledClass = inputClass + ' disabled:opacity-60';
</script>

{#if !profile && !creating}
  <div class="rounded-2xl border border-border bg-surface-1 p-12 text-center">
    <div class="text-[14px] font-medium text-text-2">Select a profile to edit</div>
    <div class="mt-2 text-[12px] text-text-3">or click "+ New profile" to create one.</div>
  </div>
{:else}
  <form on:submit|preventDefault={onSave}>
    {#if genericError}
      <div
        class="mb-4 rounded-md border border-error/30 bg-error/10 px-3 py-2 text-[12px] text-error"
      >
        {genericError}
      </div>
    {/if}

    {#if !chromeless}
      <header class="mb-7 flex items-start justify-between gap-4">
        <div class="min-w-0">
          <div class="mb-1 flex items-center gap-2">
            {#if discMeta}
              <DiscTypeBadge type={discType} />
            {/if}
            <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
              profile
            </span>
          </div>
          {#if creating}
            <label class="block">
              <span class="sr-only">Name</span>
              <input
                name="name"
                type="text"
                bind:value={working.name}
                placeholder="New profile"
                class="w-full bg-transparent text-[24px] font-bold tracking-tight text-text outline-none placeholder:text-text-3"
              />
            </label>
          {:else}
            <label class="block">
              <span class="sr-only">Name</span>
              <input
                name="name"
                type="text"
                bind:value={working.name}
                class="w-full bg-transparent text-[24px] font-bold tracking-tight text-text outline-none"
              />
            </label>
          {/if}
          <div class="mt-1 font-mono text-[12px] text-text-3">
            applies to all {discMeta?.label ?? working.disc_type} discs
          </div>
          {#if fieldErrors.name}
            <div class="mt-1 text-[11px] text-error">{fieldErrors.name}</div>
          {/if}
        </div>

        <div class="flex flex-shrink-0 items-center gap-3">
          <label class="flex items-center gap-2">
            <input type="checkbox" bind:checked={working.enabled} />
            <span class="text-[12px] text-text-2">Enabled</span>
          </label>
          {#if !creating}
            <button
              type="button"
              on:click={onDuplicate}
              disabled={saving}
              class="rounded-md border border-border px-3 py-1.5 text-[13px] text-text-2 transition-colors hover:border-border-strong disabled:opacity-50"
            >
              Duplicate
            </button>
          {/if}
          <button
            type="submit"
            disabled={saving}
            class="rounded-md bg-accent px-4 py-1.5 text-[13px] font-semibold text-black disabled:opacity-50"
          >
            {creating ? 'Create' : 'Save changes'}
          </button>
        </div>
      </header>
    {/if}

    <div class="space-y-7">
      {#if creating}
        <FormSection title="Identity" sub="Disc type and engine lock once the profile is created.">
          <FormRow label="Disc type" error={fieldErrors.disc_type}>
            <select name="disc_type" bind:value={working.disc_type} class={inputClass}>
              {#each DISC_TYPES as dt}
                <option value={dt}>{dt}</option>
              {/each}
            </select>
          </FormRow>
        </FormSection>
      {/if}

      <FormSection title="Engine" sub="Which tool reads the disc and produces the source.">
        <FormRow label="Reader" error={fieldErrors.engine}>
          <select
            name="engine"
            bind:value={working.engine}
            on:change={onEngineChange}
            disabled={!creating}
            class={inputDisabledClass}
          >
            {#each engineNames() as e}
              <option value={e}>{e}</option>
            {/each}
          </select>
        </FormRow>
        <FormRow label="Drive policy" error={fieldErrors.drive_policy}>
          <select name="drive_policy" bind:value={working.drive_policy} class={inputClass}>
            {#each DRIVE_POLICIES as dp}
              <option value={dp}>{DRIVE_POLICY_LABELS[dp] ?? dp}</option>
            {/each}
          </select>
        </FormRow>
        <FormRow label="Auto-eject on done">
          <label class="flex items-center gap-2">
            <input type="checkbox" bind:checked={working.auto_eject} />
            <span class="text-[12px] text-text-3">Eject the disc when the job finishes.</span>
          </label>
        </FormRow>
      </FormSection>

      <FormSection title="Encoding" sub="Format and codec applied during transcode/compress.">
        <FormRow label="Container" error={fieldErrors.container ?? fieldErrors.format}>
          <select
            name="container"
            bind:value={working.container}
            on:change={onContainerChange}
            class={inputClass}
          >
            {#each validContainers as c}
              <option value={c}>{c}</option>
            {/each}
          </select>
        </FormRow>
        {#if validVideoCodecs.length > 0}
          <FormRow label="Video codec" error={fieldErrors.video_codec}>
            <select name="video_codec" bind:value={working.video_codec} class={inputClass}>
              {#each validVideoCodecs as v}
                <option value={v}>{v}</option>
              {/each}
            </select>
          </FormRow>
        {/if}
        <FormRow label="Quality preset" error={fieldErrors.quality_preset ?? fieldErrors.preset}>
          <input
            name="quality_preset"
            type="text"
            bind:value={working.quality_preset}
            class="w-full rounded-md border border-border bg-surface-2 px-2 py-1.5 font-mono text-[12px] text-text"
          />
        </FormRow>
        {#if validVideoCodecs.length > 0}
          <FormRow label="HDR pipeline" error={fieldErrors.hdr_pipeline}>
            <select name="hdr_pipeline" bind:value={working.hdr_pipeline} class={inputClass}>
              {#each HDR_PIPELINES as h}
                <option value={h}>{HDR_PIPELINE_LABELS[h] ?? h}</option>
              {/each}
            </select>
          </FormRow>
        {/if}
      </FormSection>

      <FormSection title="Post-processing" sub="Chain runs after encode finishes, in order.">
        <div class="px-4 py-6 text-center text-[12px] text-text-3">
          Coming next — verification, cuesheet, metadata tagging.
        </div>
      </FormSection>

      <FormSection title="Library">
        <FormRow label="Output path" error={fieldErrors.output_path_template}>
          <PathField value={working.output_path_template} on:change={onOutputPathChange} />
        </FormRow>
      </FormSection>

      {#if optionKeys.length > 0}
        <FormSection title="Engine options" sub="Engine-specific knobs not covered above.">
          {#each optionKeys as k}
            {@const opt = spec?.options[k]}
            <FormRow label={k} error={fieldErrors[`options.${k}`]}>
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
                  class={inputClass}
                />
              {:else}
                <input
                  type="text"
                  value={String(working.options[k] ?? '')}
                  on:input={(e) => onOptionStringInput(k, e)}
                  class={inputClass}
                />
              {/if}
            </FormRow>
          {/each}
        </FormSection>
      {/if}

      <div class="text-[11px] text-text-3">Step count: {working.step_count}</div>

      {#if !creating && !chromeless}
        <div class="flex justify-end">
          <button
            type="button"
            on:click={onDelete}
            disabled={saving}
            class="rounded-md border border-border px-4 py-2 text-[13px] text-text-2 transition-colors hover:border-error hover:text-error disabled:opacity-50"
          >
            {confirmingDelete ? 'Confirm delete' : 'Delete profile'}
          </button>
        </div>
      {/if}
    </div>
  </form>
{/if}
