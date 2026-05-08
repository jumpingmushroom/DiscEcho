<script lang="ts">
  import MobileSettings from '$lib/components/MobileSettings.svelte';
  import DesktopSettings from '$lib/components/desktop/DesktopSettings.svelte';
  import AppBar from '$lib/components/AppBar.svelte';
  import TabBar from '$lib/components/TabBar.svelte';
</script>

<div class="lg:hidden">
  <div class="min-h-screen pb-24">
    <AppBar title="Settings" />
    <MobileSettings />
  </div>
  <TabBar active="settings" />
</div>
<div class="hidden lg:block">
  <DesktopSettings />
</div>
