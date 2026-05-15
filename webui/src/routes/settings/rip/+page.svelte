<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { isDesktop } from '$lib/viewport';
  import SubPageShell from '$lib/components/mobile/SubPageShell.svelte';
  import RipBehaviourSection from '$lib/components/desktop/RipBehaviourSection.svelte';

  onMount(() => {
    if ($isDesktop) goto('/settings');
  });
</script>

{#if !$isDesktop}
  <SubPageShell title="Rip behaviour">
    <RipBehaviourSection />
  </SubPageShell>
{/if}
