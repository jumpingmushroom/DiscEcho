<script lang="ts">
  import MobileProfileList from '$lib/components/MobileProfileList.svelte';
  import DesktopProfiles from '$lib/components/desktop/DesktopProfiles.svelte';
  import { isDesktop } from '$lib/viewport';
</script>

{#if $isDesktop}
  <DesktopProfiles />
{:else}
  <MobileProfileList />
{/if}
