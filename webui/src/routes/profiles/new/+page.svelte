<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { isDesktop } from '$lib/viewport';
  import DesktopProfiles from '$lib/components/desktop/DesktopProfiles.svelte';
  import ProfileEditShell from '$lib/components/mobile/ProfileEditShell.svelte';

  // Desktop has no concept of a separate /profiles/new route — its
  // DesktopProfiles flow uses a "+ New profile" button inline. Bounce
  // desktop visitors back to /profiles where that button lives.
  onMount(() => {
    if ($isDesktop) {
      goto('/profiles');
    }
  });
</script>

{#if $isDesktop}
  <DesktopProfiles />
{:else}
  <ProfileEditShell profile={null} creating={true} />
{/if}
