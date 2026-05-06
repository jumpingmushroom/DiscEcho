<script lang="ts">
  import { onMount } from 'svelte';

  type BuildInfo = { version: string; commit: string; build_date: string };

  let info: BuildInfo | null = null;
  let error: string | null = null;

  onMount(async () => {
    try {
      const res = await fetch('/api/version');
      if (!res.ok) throw new Error(`http ${res.status}`);
      info = await res.json();
    } catch (e) {
      error = (e as Error).message;
    }
  });
</script>

<main class="min-h-screen flex items-center justify-center p-8">
  <div class="text-center space-y-3">
    <h1 class="text-4xl font-bold tracking-tight">DiscEcho</h1>
    <p class="text-text-2 text-sm">M0 placeholder &mdash; the real UI lands at M1.</p>
    {#if info}
      <p class="font-mono text-xs text-text-3">
        {info.version} · {info.commit} · {info.build_date}
      </p>
    {:else if error}
      <p class="font-mono text-xs text-error">api unreachable: {error}</p>
    {:else}
      <p class="font-mono text-xs text-text-3">loading…</p>
    {/if}
  </div>
</main>
