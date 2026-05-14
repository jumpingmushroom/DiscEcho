<script lang="ts">
  import '../app.css';
  import { onMount, onDestroy } from 'svelte';
  import { bootstrap, connect } from '$lib/store';
  import { initPWA } from '$lib/pwa';
  import TopNav from '$lib/components/desktop/TopNav.svelte';
  import UpdateToast from '$lib/components/UpdateToast.svelte';
  import ToastHost from '$lib/components/ToastHost.svelte';
  import IOSInstallHint from '$lib/components/IOSInstallHint.svelte';

  let disconnect: (() => void) | null = null;

  onMount(async () => {
    try {
      await bootstrap();
    } catch (e) {
      console.error('bootstrap failed', e);
    }
    disconnect = connect();
    initPWA();
  });

  onDestroy(() => {
    if (disconnect) disconnect();
  });
</script>

<div class="aurora-ribbon" aria-hidden="true"></div>
<TopNav />
<slot />
<UpdateToast />
<ToastHost />
<IOSInstallHint />

<style>
  :global(body) {
    background: var(--bg);
    color: var(--text);
    overscroll-behavior-y: none;
  }
</style>
