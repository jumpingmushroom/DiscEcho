<script lang="ts">
  import { onMount } from 'svelte';
  import { drives, bootCodeCounts } from '$lib/store';
  import { apiGet, apiPut, apiPatch } from '$lib/api';
  import { pushToast } from '$lib/toasts';
  import type { Drive } from '$lib/wire';
  import FormSection from './FormSection.svelte';
  import FormRow from './FormRow.svelte';
  import PathField from './PathField.svelte';
  import ApiRow from './ApiRow.svelte';

  type VersionInfo = { version?: string; commit?: string; build_date?: string };

  type DiskInfo = {
    path: string;
    paths?: string[];
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

  type SubItem = {
    label: string;
    status: string;
    detail?: string;
  };

  type IntegrationStatus = {
    name: string;
    hint?: string;
    status: string;
    detail?: string;
    editable?: string;
    sub_items?: SubItem[];
  };

  type IntegrationsInfo = {
    tmdb: { configured: boolean; language: string };
    musicbrainz: { base_url: string; user_agent: string };
    apprise: { bin: string; version?: string };
    library_roots?: Record<string, string>;
    items?: IntegrationStatus[];
    boot_code_counts?: Record<string, number>;
  };

  type MediaRoot = 'movies' | 'tv' | 'music' | 'games' | 'data';
  type LibraryRoots = Record<MediaRoot, string>;

  const ROOT_LABELS: Record<MediaRoot, string> = {
    movies: 'Movies',
    tv: 'TV',
    music: 'Music',
    games: 'Games',
    data: 'Data archive',
  };
  const ROOT_ORDER: MediaRoot[] = ['movies', 'tv', 'music', 'games', 'data'];

  let version: VersionInfo | null = null;
  let host: HostInfo | null = null;
  let integrations: IntegrationsInfo | null = null;

  let roots: LibraryRoots = { movies: '', tv: '', music: '', games: '', data: '' };
  let dirty: Partial<LibraryRoots> = {};
  let savingRoots = false;
  let rootsError: string | null = null;

  $: hasDirty = Object.keys(dirty).length > 0;

  onMount(async () => {
    const [v, h, i] = await Promise.allSettled([
      apiGet<VersionInfo>('/api/version'),
      apiGet<HostInfo>('/api/system/host'),
      apiGet<IntegrationsInfo>('/api/system/integrations'),
    ]);
    version = v.status === 'fulfilled' ? v.value : null;
    host = h.status === 'fulfilled' ? h.value : null;
    integrations = i.status === 'fulfilled' ? i.value : null;
    if (integrations?.library_roots) {
      for (const m of ROOT_ORDER) {
        roots[m] = integrations.library_roots[m] ?? '';
      }
    }
    if (integrations?.boot_code_counts) {
      bootCodeCounts.set(integrations.boot_code_counts);
    }
  });

  function onRootChange(media: MediaRoot, e: CustomEvent<string>): void {
    dirty = { ...dirty, [media]: e.detail };
  }

  async function saveRoots(): Promise<void> {
    rootsError = null;
    savingRoots = true;
    try {
      const body: Record<string, string> = {};
      for (const [media, path] of Object.entries(dirty)) {
        body[`library.${media}`] = path as string;
      }
      await apiPut('/api/settings', body);
      roots = { ...roots, ...dirty };
      dirty = {};
      pushToast('success', 'Library paths saved');
    } catch (e) {
      rootsError = (e as Error).message;
    } finally {
      savingRoots = false;
    }
  }

  function discardRoots(): void {
    dirty = {};
    rootsError = null;
  }

  function onIntegrationEdit(item: IntegrationStatus): void {
    if (item.name === 'Apprise') {
      // Scroll the user to the notifications section, which is where
      // Apprise URLs are managed.
      const target = document.querySelector('[data-section="notifications"]');
      if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      return;
    }
    if (item.editable) {
      window.alert(
        `Configure ${item.name} by setting ${item.editable} in .env, then restart the container.`,
      );
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

  // Per-drive offset editor state. Keyed by drive ID so multiple drives
  // can be edited independently. The draft uses `number | null` because
  // `<input type="number" bind:value>` writes a JavaScript number (or
  // null when the field is empty), not a string. Saving guards against
  // concurrent PATCH bursts.
  let offsetEditing: Record<string, boolean> = {};
  let offsetDraft: Record<string, number | null> = {};
  let offsetSaving: Record<string, boolean> = {};
  let offsetError: Record<string, string> = {};

  function offsetSourceLabel(d: Drive): string {
    if (d.read_offset_source === 'manual') return 'manual';
    if (d.read_offset_source === 'auto') return 'auto';
    return 'uncalibrated';
  }

  function beginOffsetEdit(d: Drive): void {
    offsetEditing = { ...offsetEditing, [d.id]: true };
    offsetDraft = { ...offsetDraft, [d.id]: d.read_offset ?? 0 };
    offsetError = { ...offsetError, [d.id]: '' };
  }

  function cancelOffsetEdit(d: Drive): void {
    offsetEditing = { ...offsetEditing, [d.id]: false };
    offsetError = { ...offsetError, [d.id]: '' };
  }

  async function saveOffset(d: Drive): Promise<void> {
    const draft = offsetDraft[d.id];
    if (draft === null || draft === undefined || Number.isNaN(draft)) {
      offsetError = { ...offsetError, [d.id]: 'Offset must be a whole number.' };
      return;
    }
    if (!Number.isInteger(draft)) {
      offsetError = { ...offsetError, [d.id]: 'Offset must be a whole number.' };
      return;
    }
    if (draft < -3000 || draft > 3000) {
      offsetError = { ...offsetError, [d.id]: 'Offset must be within ±3000 samples.' };
      return;
    }
    const parsed = draft;
    offsetSaving = { ...offsetSaving, [d.id]: true };
    offsetError = { ...offsetError, [d.id]: '' };
    try {
      await apiPatch(`/api/drives/${encodeURIComponent(d.id)}/offset`, {
        read_offset: parsed,
      });
      drives.update((list) =>
        list.map((row) =>
          row.id === d.id ? { ...row, read_offset: parsed, read_offset_source: 'manual' } : row,
        ),
      );
      offsetEditing = { ...offsetEditing, [d.id]: false };
      pushToast('success', `Saved read-offset ${parsed} for ${d.model || d.dev_path}`);
    } catch (e) {
      offsetError = { ...offsetError, [d.id]: (e as Error).message };
    } finally {
      offsetSaving = { ...offsetSaving, [d.id]: false };
    }
  }
</script>

<div class="space-y-7">
  <FormSection title="Drives" sub="Connected optical drives. DiscEcho watches /dev/sr* by default.">
    {#if $drives.length === 0}
      <div class="px-4 py-3 text-[12px] text-text-3">No drives detected.</div>
    {:else}
      {#each $drives as d (d.id)}
        <div class="px-4 py-3" data-testid="drive-row" data-drive-id={d.id}>
          <div class="grid items-center gap-4" style="grid-template-columns: 1fr auto auto">
            <div>
              <div class="text-[13px] font-medium text-text">{d.model || '—'}</div>
              <div class="font-mono text-[11px] text-text-3">
                {d.dev_path}
              </div>
            </div>
            <span
              class="inline-flex items-center rounded-md border border-border bg-surface-2
                     px-2 py-[3px] text-[11px] font-medium uppercase tracking-wide text-text-2"
            >
              {d.state}
            </span>
          </div>

          <!--
            AccurateRip read-offset row. Inline edit affordance — number
            entry only; auto-detect against a calibration disc is a
            follow-up (needs whipper offset find stdout parsing
            captured against the homelab drive first). Disabled while
            drive is busy: changing offset mid-rip would silently mean
            the in-flight checksums don't reflect the configured offset.
          -->
          <div
            class="mt-2 flex items-center gap-3 text-[11px] text-text-3"
            data-testid="drive-offset-row"
          >
            <span class="font-mono">offset:</span>
            {#if offsetEditing[d.id]}
              <input
                type="number"
                bind:value={offsetDraft[d.id]}
                min={-3000}
                max={3000}
                step="1"
                class="w-24 rounded border border-border bg-surface-2 px-2 py-1 font-mono text-text"
                data-testid="drive-offset-input"
              />
              <button
                type="button"
                on:click={() => saveOffset(d)}
                disabled={offsetSaving[d.id]}
                class="rounded-md bg-accent px-2 py-1 text-[11px] font-semibold text-black disabled:opacity-50"
                data-testid="drive-offset-save"
              >
                Save
              </button>
              <button
                type="button"
                on:click={() => cancelOffsetEdit(d)}
                disabled={offsetSaving[d.id]}
                class="rounded-md border border-border px-2 py-1 text-[11px] text-text-2 disabled:opacity-50"
              >
                Cancel
              </button>
              {#if offsetError[d.id]}
                <span class="text-error" data-testid="drive-offset-error">{offsetError[d.id]}</span>
              {/if}
            {:else}
              <span class="font-mono text-text" data-testid="drive-offset-value">
                {d.read_offset ?? 0}
              </span>
              <span
                class="inline-flex items-center rounded-md border border-border bg-surface-2 px-2 py-[2px] text-[10px] uppercase tracking-wide"
                class:text-text-3={offsetSourceLabel(d) === 'uncalibrated'}
                data-testid="drive-offset-source"
              >
                {offsetSourceLabel(d)}
              </span>
              <button
                type="button"
                on:click={() => beginOffsetEdit(d)}
                disabled={d.state !== 'idle'}
                title={d.state !== 'idle'
                  ? 'Drive is busy — wait for it to return to idle before changing the offset.'
                  : 'Set the per-drive read-offset to enable AccurateRip verification.'}
                class="rounded-md border border-border px-2 py-1 text-[11px] text-text-2 disabled:opacity-50"
                data-testid="drive-offset-edit"
              >
                Edit
              </button>
            {/if}
          </div>
        </div>
      {/each}
    {/if}
  </FormSection>

  <FormSection title="Library paths" sub="Where ripped media lands.">
    {#each ROOT_ORDER as m (m)}
      <FormRow label={ROOT_LABELS[m]}>
        <PathField value={dirty[m] ?? roots[m]} on:change={(e) => onRootChange(m, e)} />
      </FormRow>
    {/each}
    {#if hasDirty}
      <div class="flex items-center justify-between gap-3 px-4 py-3">
        <div class="text-[11px] text-text-3">
          {#if rootsError}
            <span class="text-error">{rootsError}</span>
          {:else}
            Unsaved changes. Container restart required for in-flight rips to pick up new paths.
          {/if}
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            on:click={discardRoots}
            disabled={savingRoots}
            class="rounded-md border border-border px-3 py-1.5 text-[12px] text-text-2 disabled:opacity-50"
          >
            Discard
          </button>
          <button
            type="button"
            on:click={saveRoots}
            disabled={savingRoots}
            class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-semibold text-black disabled:opacity-50"
          >
            Save changes
          </button>
        </div>
      </div>
    {/if}
  </FormSection>

  <FormSection title="API keys & connections">
    {#if integrations?.items && integrations.items.length > 0}
      {#each integrations.items as item (item.name)}
        <ApiRow
          name={item.name}
          hint={item.hint ?? ''}
          status={item.status}
          detail={item.detail ?? ''}
          editable={item.editable ?? ''}
          on:edit={() => onIntegrationEdit(item)}
        />
      {/each}
    {:else if integrations}
      <div class="px-4 py-3 text-[12px] text-text-3">No integrations configured.</div>
    {:else}
      <div class="px-4 py-3 text-[12px] text-text-3">Loading…</div>
    {/if}
  </FormSection>

  <FormSection title="Host">
    {#if host}
      <div class="px-4 py-3 grid gap-2 text-[12px]" style="grid-template-columns: 120px 1fr">
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
        <div class="px-4 py-3 space-y-2">
          {#each host.disks as d (d.path)}
            <div>
              <div class="flex justify-between text-[11px] text-text-3">
                <span class="font-mono">{d.path}</span>
                <span>
                  {formatBytes(d.used_bytes)} / {formatBytes(d.total_bytes)}
                  · {formatBytes(d.available_bytes)} free
                </span>
              </div>
              {#if d.paths && d.paths.length > 0}
                <div class="font-mono text-[10px] text-text-3">
                  shared with {d.paths.join(', ')}
                </div>
              {/if}
              <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-surface-2">
                <div class="h-full bg-accent" style="width: {diskPercent(d)}%"></div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="px-4 py-3 text-[12px] text-text-3">Loading…</div>
    {/if}
  </FormSection>
</div>
