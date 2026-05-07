<script lang="ts">
  import MobileProfileList from '$lib/components/MobileProfileList.svelte';
  import DesktopProfiles from '$lib/components/desktop/DesktopProfiles.svelte';
</script>

<div class="lg:hidden"><MobileProfileList /></div>
<div class="hidden lg:block"><DesktopProfiles /></div>
