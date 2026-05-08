<script lang="ts">
  import { onMount } from 'svelte';
  import { settings, drives } from '$lib/store';
  import { apiGet, apiPut } from '$lib/api';

  type VersionInfo = { version?: string; commit?: string; build_date?: string };

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

  type IntegrationsInfo = {
    tmdb: { configured: boolean; language: string };
    musicbrainz: { base_url: string; user_agent: string };
    apprise: { bin: string; version?: string };
  };

  let version: VersionInfo | null = null;
  let host: HostInfo | null = null;
  let integrations: IntegrationsInfo | null = null;

  let libraryPathInput = '';
  let libraryEdited = false;
  let savingLibrary = false;
  let librarySavedAt: number | null = null;
  let libraryError: string | null = null;

  $: storedLibraryPath = ($settings['library.path'] as string) ?? '';
  $: if (!libraryEdited) libraryPathInput = storedLibraryPath;

  onMount(async () => {
    const [v, h, i] = await Promise.allSettled([
      apiGet<VersionInfo>('/api/version'),
      apiGet<HostInfo>('/api/system/host'),
      apiGet<IntegrationsInfo>('/api/system/integrations'),
    ]);
    version = v.status === 'fulfilled' ? v.value : null;
    host = h.status === 'fulfilled' ? h.value : null;
    integrations = i.status === 'fulfilled' ? i.value : null;
  });

  async function saveLibrary(): Promise<void> {
    libraryError = null;
    savingLibrary = true;
    try {
      await apiPut('/api/settings', { 'library.path': libraryPathInput.trim() });
      libraryEdited = false;
      librarySavedAt = Date.now();
    } catch (e) {
      libraryError = (e as Error).message;
    } finally {
      savingLibrary = false;
    }
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

  function formatUptime(s: number): string {
    if (!Number.isFinite(s) || s <= 0) return '—';
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  function diskPercent(d: DiskInfo): number {
    if (!d.total_bytes) return 0;
    return Math.min(100, Math.round((d.used_bytes / d.total_bytes) * 100));
  }
</script>

<section class="rounded-2xl border border-border bg-surface-1 p-5">
  <h2 class="text-[14px] font-semibold text-text">System</h2>

  <!-- Library -->
  <div class="mt-4 space-y-2">
    <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Library</div>
    <div class="flex items-center gap-2">
      <input
        type="text"
        bind:value={libraryPathInput}
        on:input={() => (libraryEdited = true)}
        placeholder="/library"
        class="flex-1 rounded-md border border-border bg-surface-2 px-2 py-1 font-mono text-[13px] text-text"
      />
      <button
        on:click={saveLibrary}
        disabled={savingLibrary || libraryPathInput.trim() === storedLibraryPath}
        class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-semibold text-black disabled:opacity-50"
      >
        Save
      </button>
    </div>
    {#if libraryError}
      <div class="text-[11px] text-error">{libraryError}</div>
    {:else if librarySavedAt}
      <div class="text-[11px] text-text-3">
        Saved. Restart the container for running pipelines to pick up the new path.
      </div>
    {:else}
      <div class="text-[11px] text-text-3">
        Where ripped media is written. Takes effect on next container restart.
      </div>
    {/if}
  </div>

  <!-- Drives -->
  <div class="mt-6 space-y-2">
    <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Drives</div>
    {#if $drives.length === 0}
      <div class="rounded-md border border-border bg-surface-2 p-3 text-[12px] text-text-3">
        No drives detected.
      </div>
    {:else}
      <div class="overflow-hidden rounded-md border border-border">
        <table class="w-full text-[12px]">
          <thead class="bg-surface-2 text-left text-text-3">
            <tr>
              <th class="px-3 py-2 font-medium">Model</th>
              <th class="px-3 py-2 font-medium">Bus</th>
              <th class="px-3 py-2 font-medium">Device</th>
              <th class="px-3 py-2 font-medium">State</th>
            </tr>
          </thead>
          <tbody>
            {#each $drives as d (d.id)}
              <tr class="border-t border-border">
                <td class="px-3 py-2 text-text">{d.model || '—'}</td>
                <td class="px-3 py-2 text-text-2">{d.bus || '—'}</td>
                <td class="px-3 py-2 font-mono text-text-2">{d.dev_path}</td>
                <td class="px-3 py-2 text-text-2">{d.state}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <!-- Connections -->
  <div class="mt-6 space-y-2">
    <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Connections</div>
    {#if integrations}
      <div class="grid gap-2 text-[12px]" style="grid-template-columns: 120px 1fr">
        <div class="text-text-2">TMDB</div>
        <div class="text-text">
          {#if integrations.tmdb.configured}
            <span class="text-success">configured</span>
            {#if integrations.tmdb.language}
              <span class="text-text-3"> · {integrations.tmdb.language}</span>
            {/if}
          {:else}
            <span class="text-text-3">not configured — set DISCECHO_TMDB_KEY in .env</span>
          {/if}
        </div>
        <div class="text-text-2">MusicBrainz</div>
        <div class="font-mono text-text">{integrations.musicbrainz.base_url}</div>
        <div class="text-text-2">Apprise</div>
        <div class="font-mono text-text">
          {integrations.apprise.bin}{#if integrations.apprise.version}
            <span class="text-text-3"> · {integrations.apprise.version}</span>
          {/if}
        </div>
      </div>
    {:else}
      <div class="text-[12px] text-text-3">Loading…</div>
    {/if}
  </div>

  <!-- Host -->
  <div class="mt-6 space-y-2">
    <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">Host</div>
    {#if host}
      <div class="grid gap-2 text-[12px]" style="grid-template-columns: 120px 1fr">
        <div class="text-text-2">Hostname</div>
        <div class="font-mono text-text">{host.hostname || '—'}</div>
        <div class="text-text-2">Kernel</div>
        <div class="font-mono text-text">{host.kernel || '—'}</div>
        <div class="text-text-2">CPUs</div>
        <div class="text-text">{host.cpu_count}</div>
        <div class="text-text-2">Uptime</div>
        <div class="text-text">{formatUptime(host.uptime_seconds)}</div>
        <div class="text-text-2">Build</div>
        <div class="font-mono text-text">{version?.version ?? '—'}</div>
      </div>
      {#if host.disks.length > 0}
        <div class="mt-3 space-y-2">
          {#each host.disks as d (d.path)}
            <div>
              <div class="flex justify-between text-[11px] text-text-3">
                <span class="font-mono">{d.path}</span>
                <span>
                  {formatBytes(d.used_bytes)} / {formatBytes(d.total_bytes)}
                  · {formatBytes(d.available_bytes)} free
                </span>
              </div>
              <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-surface-2">
                <div class="h-full bg-accent" style="width: {diskPercent(d)}%"></div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="text-[12px] text-text-3">Loading…</div>
    {/if}
  </div>
</section>
