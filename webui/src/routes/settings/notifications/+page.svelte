<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { isDesktop } from '$lib/viewport';
  import SubPageShell from '$lib/components/mobile/SubPageShell.svelte';
  import NotificationsSection from '$lib/components/desktop/NotificationsSection.svelte';

  onMount(() => {
    if ($isDesktop) goto('/settings');
  });
</script>

{#if !$isDesktop}
  <SubPageShell title="Notifications">
    <NotificationsSection />
  </SubPageShell>
{/if}
