<script lang="ts">
  import MobileProfileList from '$lib/components/mobile/ProfileList.svelte';
  import DesktopProfiles from '$lib/components/desktop/DesktopProfiles.svelte';
  import { isDesktop } from '$lib/viewport';
</script>

{#if $isDesktop}
  <DesktopProfiles />
{:else}
  <MobileProfileList />
{/if}
