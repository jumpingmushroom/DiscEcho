<script lang="ts">
  import { profiles, selectedProfileID } from '$lib/store';
  import ProfileList from './ProfileList.svelte';
  import ProfileEditor from './ProfileEditor.svelte';

  let creating = false;

  $: selected = $profiles.find((p) => p.id === $selectedProfileID) ?? null;

  function onSelect(e: CustomEvent<string>): void {
    creating = false;
    selectedProfileID.set(e.detail);
  }

  function onNew(): void {
    creating = true;
    selectedProfileID.set(null);
  }

  function onSaved(): void {
    creating = false;
  }
</script>

<div class="mx-auto min-h-screen max-w-screen-2xl p-6">
  <div class="grid gap-6" style="grid-template-columns: 320px 1fr">
    <ProfileList
      profiles={$profiles}
      selectedID={$selectedProfileID}
      on:select={onSelect}
      on:new={onNew}
    />
    <ProfileEditor profile={selected} {creating} on:saved={onSaved} />
  </div>
</div>
