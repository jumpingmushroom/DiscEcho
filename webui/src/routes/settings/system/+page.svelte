<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { isDesktop } from '$lib/viewport';
  import SubPageShell from '$lib/components/mobile/SubPageShell.svelte';
  import SystemSection from '$lib/components/desktop/SystemSection.svelte';

  // Desktop users see all sections inline on /settings; punt them back.
  onMount(() => {
    if ($isDesktop) goto('/settings');
  });
</script>

{#if !$isDesktop}
  <SubPageShell title="System">
    <SystemSection />
  </SubPageShell>
{/if}
