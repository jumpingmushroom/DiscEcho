<script lang="ts">
  import { goto } from '$app/navigation';
  import Icon from '$lib/icons/Icon.svelte';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import ProfileEditor from '$lib/components/desktop/ProfileEditor.svelte';
  import type { DiscType, Profile } from '$lib/wire';

  export let profile: Profile | null = null;
  export let creating: boolean = false;

  // Save/Delete are triggered from the sticky action bar via the editor's
  // exported methods. The editor still owns validation, error display, and
  // toast pushes; this shell just renders chrome around it.
  let editor: ProfileEditor | undefined;

  $: title = profile?.name || (creating ? 'New profile' : '');
  $: discType = (profile?.disc_type ?? 'AUDIO_CD') as DiscType;

  function back(): void {
    goto('/profiles');
  }

  function onSavedOrDeleted(): void {
    goto('/profiles');
  }

  function onDuplicate(): void {
    // Desktop only — bottom-of-list duplicate flow doesn't apply on mobile.
    // No-op: we don't currently surface duplicate on mobile.
  }

  function save(): void {
    void editor?.onSave();
  }

  function del(): void {
    void editor?.onDelete();
  }
</script>

<div class="min-h-screen pb-28">
  <!-- Sticky AppBar with ← back -->
  <div
    class="sticky top-0 z-20 flex items-center justify-between border-b border-border px-3 backdrop-blur"
    style="background: rgba(10,10,10,0.92); padding-top: calc(env(safe-area-inset-top, 0px) + 12px); padding-bottom: 12px"
  >
    <button
      class="flex min-h-[44px] min-w-[44px] items-center justify-center text-text-2"
      aria-label="Back"
      on:click={back}
    >
      <Icon name="chevron-left" size={20} />
    </button>
    <div class="min-w-0 flex-1 px-2 text-center">
      <div class="truncate font-medium text-text" style="font-size: var(--ts-body)">
        {title}
      </div>
      <div class="mt-0.5 flex items-center justify-center gap-1.5">
        {#if !creating}<DiscTypeBadge type={discType} />{/if}
        <span class="uppercase tracking-[0.14em] text-text-3" style="font-size: 10px">
          {creating ? 'new profile' : 'profile'}
        </span>
      </div>
    </div>
    <div class="w-11"></div>
  </div>

  <div class="px-4 pt-4">
    <ProfileEditor
      bind:this={editor}
      {profile}
      {creating}
      chromeless={true}
      on:saved={onSavedOrDeleted}
      on:duplicate={onDuplicate}
    />
  </div>

  <!-- Sticky bottom action bar -->
  <div
    class="fixed bottom-0 left-0 right-0 z-30 flex items-center gap-2 border-t border-border bg-bg px-3 py-3"
    style="padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 12px)"
  >
    <button
      type="button"
      class="min-h-[44px] flex-1 rounded-xl bg-accent font-semibold text-black"
      style="font-size: var(--ts-body)"
      on:click={save}
    >
      {creating ? 'Create' : 'Save'}
    </button>
    <button
      type="button"
      class="min-h-[44px] rounded-xl border border-border px-4 text-text-2"
      style="font-size: var(--ts-body)"
      on:click={back}
    >
      Cancel
    </button>
    {#if !creating}
      <button
        type="button"
        class="min-h-[44px] rounded-xl border px-4"
        style="border-color: var(--error); color: var(--error); background: rgba(255,91,91,0.08); font-size: var(--ts-body)"
        on:click={del}
      >
        Delete
      </button>
    {/if}
  </div>
</div>
