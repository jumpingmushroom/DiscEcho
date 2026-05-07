<script lang="ts">
  import '../app.css';
  import { onMount, onDestroy } from 'svelte';
  import { bootstrap, connect } from '$lib/store';
  import TopNav from '$lib/components/desktop/TopNav.svelte';

  let disconnect: (() => void) | null = null;

  onMount(async () => {
    try {
      await bootstrap();
    } catch (e) {
      console.error('bootstrap failed', e);
    }
    disconnect = connect();
  });

  onDestroy(() => {
    if (disconnect) disconnect();
  });
</script>

<TopNav />
<slot />

<style>
  :global(body) {
    background: var(--bg);
    color: var(--text);
    overscroll-behavior-y: none;
  }
</style>
