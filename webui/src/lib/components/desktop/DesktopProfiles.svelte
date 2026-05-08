<script lang="ts">
  import type { Profile } from '$lib/wire';
  import { profiles, selectedProfileID } from '$lib/store';
  import ProfileList from './ProfileList.svelte';
  import ProfileEditor from './ProfileEditor.svelte';

  let creating = false;
  let draft: Profile | null = null;

  $: selected = draft ?? $profiles.find((p) => p.id === $selectedProfileID) ?? null;

  function onSelect(e: CustomEvent<string>): void {
    creating = false;
    draft = null;
    selectedProfileID.set(e.detail);
  }

  function onNew(): void {
    creating = true;
    draft = null;
    selectedProfileID.set(null);
  }

  function onDuplicate(e: CustomEvent<Profile>): void {
    creating = true;
    draft = e.detail;
    selectedProfileID.set(null);
  }

  function onSaved(): void {
    creating = false;
    draft = null;
  }
</script>

<div class="grid h-screen" style="grid-template-columns: 300px 1fr">
  <aside class="border-r border-border bg-surface-1 overflow-hidden">
    <ProfileList
      profiles={$profiles}
      selectedID={$selectedProfileID}
      on:select={onSelect}
      on:new={onNew}
    />
  </aside>
  <main class="overflow-auto">
    <div class="max-w-3xl px-7 py-6">
      <ProfileEditor profile={selected} {creating} on:saved={onSaved} on:duplicate={onDuplicate} />
    </div>
  </main>
</div>
