<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { isDesktop } from '$lib/viewport';
  import { profiles } from '$lib/store';
  import { onMount } from 'svelte';
  import DesktopProfiles from '$lib/components/desktop/DesktopProfiles.svelte';
  import ProfileEditShell from '$lib/components/mobile/ProfileEditShell.svelte';
  import { selectedProfileID } from '$lib/store';

  $: id = $page.params.id;
  $: profile = $profiles.find((p) => p.id === id) ?? null;

  // On desktop, this URL is normally /profiles?selected=... — we keep the
  // route alive on desktop too by deep-linking the selection store and
  // rendering the two-pane DesktopProfiles. Mobile users see the
  // full-screen editor shell.
  onMount(() => {
    if ($isDesktop && id) {
      selectedProfileID.set(id);
    }
  });

  $: if ($profiles.length > 0 && !profile) {
    // Profile id from URL doesn't exist in the live snapshot — punt to list.
    goto('/profiles');
  }
</script>

{#if $isDesktop}
  <DesktopProfiles />
{:else if profile}
  <ProfileEditShell {profile} creating={false} />
{:else}
  <div class="min-h-screen px-5 py-12 text-center text-text-3">Loading…</div>
{/if}
