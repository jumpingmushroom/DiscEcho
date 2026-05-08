<script lang="ts">
  import { derived } from 'svelte/store';
  import { onMount } from 'svelte';
  import { notifications, settings, drives } from '$lib/store';
  import { apiGet } from '$lib/api';

  type DiskInfo = {
    path: string;
    total_bytes: number;
    used_bytes: number;
    available_bytes: number;
  };
  type HostInfo = {
    hostname: string;
    kernel: string;
    cpu_count: number;
    uptime_seconds: number;
    disks: DiskInfo[];
  };

  let host: HostInfo | null = null;

  onMount(async () => {
    try {
      host = await apiGet<HostInfo>('/api/system/host');
    } catch {
      host = null;
    }
  });

  function schemeOnly(url: string): string {
    const i = url.indexOf('://');
    return i > 0 ? url.slice(0, i + 3) + '...' : url;
  }

  function formatBytes(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    let u = 0;
    let v = n;
    while (v >= 1024 && u < units.length - 1) {
      v /= 1024;
      u += 1;
    }
    return `${v.toFixed(v < 10 ? 1 : 0)} ${units[u]}`;
  }

  const libraryPath = derived(settings, ($s) => ($s['library.path'] ?? '—') as string);
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
    <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-text-3">Drives</h2>
    {#if $drives.length === 0}
      <div class="mt-2 text-[12px] text-text-3">No drives detected.</div>
    {:else}
      <ul class="mt-2 space-y-1 text-[12px] text-text-2">
        {#each $drives as d (d.id)}
          <li class="flex items-center justify-between rounded-md border border-border px-3 py-2">
            <span>
              {d.model || 'unknown'}
              <span class="font-mono text-text-3"> · {d.dev_path}</span>
            </span>
            <span class="text-[10px] text-text-3">{d.state}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section>
    <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-text-3">Host</h2>
    {#if host}
      <div class="mt-2 text-[12px] text-text-2">
        <div>{host.hostname || '—'} · {host.cpu_count} CPUs</div>
        <div class="font-mono text-text-3">{host.kernel}</div>
      </div>
      {#if host.disks.length > 0}
        <ul class="mt-2 space-y-1 text-[11px] text-text-3">
          {#each host.disks as d (d.path)}
            <li class="font-mono">
              {d.path}: {formatBytes(d.available_bytes)} free / {formatBytes(d.total_bytes)}
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      <div class="mt-2 text-[12px] text-text-3">Loading…</div>
    {/if}
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
