<script lang="ts">
  import { derived } from 'svelte/store';
  import { notifications, settings } from '$lib/store';

  function schemeOnly(url: string): string {
    const i = url.indexOf('://');
    return i > 0 ? url.slice(0, i + 3) + '...' : url;
  }

  const libraryPath = derived(settings, ($s) => ($s['library_path'] ?? '—') as string);
  const accent = derived(settings, ($s) => ($s['prefs.accent'] ?? 'aurora') as string);
  const mood = derived(settings, ($s) => ($s['prefs.mood'] ?? 'void') as string);
  const density = derived(settings, ($s) => ($s['prefs.density'] ?? 'standard') as string);
  const retainForever = derived(settings, ($s) => $s['retention.forever'] === 'true');
  const retentionDays = derived(settings, ($s) => ($s['retention.days'] ?? '?') as string);
</script>

<div class="space-y-4 p-4 pb-24">
  <section>
    <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-text-3">System</h2>
    <div class="mt-2 text-[12px] text-text-2">
      <div>Library: <span class="font-mono">{$libraryPath}</span></div>
    </div>
  </section>

  <section>
    <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-text-3">Notifications</h2>
    <ul class="mt-2 space-y-1 text-[12px] text-text-2">
      {#each $notifications as n (n.id)}
        <li class="flex items-center justify-between rounded-md border border-border px-3 py-2">
          <span>
            {n.name} <span class="font-mono text-text-3">{schemeOnly(n.url)}</span>
          </span>
          <span class="text-[10px]" class:text-accent={n.enabled} class:text-text-3={!n.enabled}>
            {n.enabled ? 'ON' : 'OFF'}
          </span>
        </li>
      {/each}
      {#if $notifications.length === 0}
        <li class="text-text-3">No notifications.</li>
      {/if}
    </ul>
  </section>

  <section>
    <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-text-3">Appearance</h2>
    <div class="mt-2 text-[12px] text-text-2">
      <div>Accent: {$accent}</div>
      <div>Mood: {$mood}</div>
      <div>Density: {$density}</div>
    </div>
  </section>

  <section>
    <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-text-3">
      History retention
    </h2>
    <div class="mt-2 text-[12px] text-text-2">
      {#if $retainForever}
        Keep history forever.
      {:else}
        Delete history after {$retentionDays} days.
      {/if}
    </div>
  </section>

  <div class="pt-4 text-center text-[11px] text-text-3">Edit on desktop.</div>
</div>
