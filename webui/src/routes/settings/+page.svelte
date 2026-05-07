<script lang="ts">
  import AppBar from '$lib/components/AppBar.svelte';
  import TabBar from '$lib/components/TabBar.svelte';
</script>

<div class="min-h-screen pb-24">
  <AppBar title="Settings" />
  <div class="mt-12 px-5 text-center">
    <div class="mb-2 text-[16px] font-semibold text-text-2">Settings land in M6</div>
    <div class="mx-auto max-w-[300px] text-[13px] text-text-3">
      Library path, notifications, and the accent / mood / density tweaks ship in M6.
    </div>
  </div>
</div>
<TabBar active="settings" />
