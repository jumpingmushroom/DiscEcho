<script lang="ts">
  import AppBar from '$lib/components/AppBar.svelte';
  import TabBar from '$lib/components/TabBar.svelte';
</script>

<div class="min-h-screen pb-24">
  <AppBar title="History" />
  <div class="mt-12 px-5 text-center">
    <div class="mb-2 text-[16px] font-semibold text-text-2">History lands in M2</div>
    <div class="mx-auto max-w-[300px] text-[13px] text-text-3">
      Once the daemon's history endpoint ships, this is where your last 30 days of rips live.
    </div>
  </div>
</div>
<TabBar active="history" />
