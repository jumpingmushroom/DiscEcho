<script lang="ts">
  import MobileSettings from '$lib/components/mobile/Settings.svelte';
  import DesktopSettings from '$lib/components/desktop/DesktopSettings.svelte';
  import { isDesktop } from '$lib/viewport';
</script>

{#if $isDesktop}
  <DesktopSettings />
{:else}
  <MobileSettings />
{/if}
