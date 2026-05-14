<script lang="ts">
  import MobileDashboard from '$lib/components/MobileDashboard.svelte';
  import DesktopDashboard from '$lib/components/desktop/DesktopDashboard.svelte';
  import { isDesktop } from '$lib/viewport';
</script>

{#if $isDesktop}
  <DesktopDashboard />
{:else}
  <MobileDashboard />
{/if}
