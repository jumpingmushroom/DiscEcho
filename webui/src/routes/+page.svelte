<script lang="ts">
  import MobileDashboard from '$lib/components/MobileDashboard.svelte';
  import DesktopDashboard from '$lib/components/desktop/DesktopDashboard.svelte';
</script>

<div class="lg:hidden"><MobileDashboard /></div>
<div class="hidden lg:block"><DesktopDashboard /></div>
