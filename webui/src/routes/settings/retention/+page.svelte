<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { isDesktop } from '$lib/viewport';
  import SubPageShell from '$lib/components/mobile/SubPageShell.svelte';
  import RetentionSection from '$lib/components/desktop/RetentionSection.svelte';

  onMount(() => {
    if ($isDesktop) goto('/settings');
  });
</script>

{#if !$isDesktop}
  <SubPageShell title="History retention">
    <RetentionSection />
  </SubPageShell>
{/if}
