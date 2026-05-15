<script lang="ts">
  import { derived } from 'svelte/store';
  import { onMount } from 'svelte';
  import { notifications, settings, drives } from '$lib/store';
  import { apiGet } from '$lib/api';
  import AppBar from './AppBar.svelte';
  import TabBar from './TabBar.svelte';
  import Icon from '$lib/icons/Icon.svelte';

  type HostInfo = {
    hostname: string;
    kernel: string;
    cpu_count: number;
    uptime_seconds: number;
  };

  let host: HostInfo | null = null;

  onMount(async () => {
    try {
      host = await apiGet<HostInfo>('/api/system/host');
    } catch {
      host = null;
    }
  });

  const libraryPath = derived(settings, ($s) => ($s['library.path'] ?? '—') as string);
  const retainForever = derived(settings, ($s) => $s['retention.forever'] === 'true');
  const retentionDays = derived(settings, ($s) => ($s['retention.days'] ?? '?') as string);
  const opMode = derived(settings, ($s) =>
    $s['operation.mode'] === 'manual' ? 'manual' : 'batch',
  );
  const ejectAtEnd = derived(settings, ($s) =>
    $s['rip.eject_on_finish'] === undefined ? true : $s['rip.eject_on_finish'] === 'true',
  );

  $: enabledNotifs = $notifications.filter((n) => n.enabled).length;
  $: retentionSummary = $retainForever ? 'Keep forever' : `Delete after ${$retentionDays} days`;
  $: ripSummary =
    $opMode === 'manual'
      ? 'Manual · no auto-rip or eject'
      : $ejectAtEnd
        ? 'Batch · auto-rip + auto-eject'
        : 'Batch · auto-rip, manual eject';
</script>

<div class="min-h-screen pb-24">
  <AppBar title="Settings" />

  <div class="space-y-2 px-4 pt-3">
    <a
      href="/settings/system"
      data-sveltekit-preload-data="hover"
      class="block min-h-[44px] rounded-2xl border border-border bg-surface-1 p-4 transition-colors hover:border-border-strong"
    >
      <div class="flex items-center justify-between gap-2">
        <div class="min-w-0">
          <div class="font-medium text-text" style="font-size: var(--ts-title)">System</div>
          <div class="mt-0.5 truncate font-mono text-text-3" style="font-size: var(--ts-overline)">
            Library: {$libraryPath}
          </div>
        </div>
        <Icon name="chevron-right" size={16} />
      </div>
    </a>

    <a
      href="/settings/rip"
      data-sveltekit-preload-data="hover"
      class="block min-h-[44px] rounded-2xl border border-border bg-surface-1 p-4 transition-colors hover:border-border-strong"
    >
      <div class="flex items-center justify-between gap-2">
        <div class="min-w-0">
          <div class="font-medium text-text" style="font-size: var(--ts-title)">Rip behaviour</div>
          <div class="mt-0.5 text-text-3" style="font-size: var(--ts-overline)">
            {ripSummary}
          </div>
        </div>
        <Icon name="chevron-right" size={16} />
      </div>
    </a>

    <a
      href="/settings/notifications"
      data-sveltekit-preload-data="hover"
      class="block min-h-[44px] rounded-2xl border border-border bg-surface-1 p-4 transition-colors hover:border-border-strong"
    >
      <div class="flex items-center justify-between gap-2">
        <div class="min-w-0">
          <div class="font-medium text-text" style="font-size: var(--ts-title)">Notifications</div>
          <div class="mt-0.5 text-text-3" style="font-size: var(--ts-overline)">
            {$notifications.length} configured · {enabledNotifs} on
          </div>
        </div>
        <Icon name="chevron-right" size={16} />
      </div>
    </a>

    <a
      href="/settings/retention"
      data-sveltekit-preload-data="hover"
      class="block min-h-[44px] rounded-2xl border border-border bg-surface-1 p-4 transition-colors hover:border-border-strong"
    >
      <div class="flex items-center justify-between gap-2">
        <div class="min-w-0">
          <div class="font-medium text-text" style="font-size: var(--ts-title)">
            History retention
          </div>
          <div class="mt-0.5 text-text-3" style="font-size: var(--ts-overline)">
            {retentionSummary}
          </div>
        </div>
        <Icon name="chevron-right" size={16} />
      </div>
    </a>

    <!-- Read-only summary cards (no edit page) -->
    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="flex items-center justify-between gap-2">
        <div class="min-w-0">
          <div class="font-medium text-text" style="font-size: var(--ts-title)">Drives</div>
          <div class="mt-0.5 text-text-3" style="font-size: var(--ts-overline)">
            {$drives.length} detected · read-only
          </div>
        </div>
      </div>
      {#if $drives.length > 0}
        <ul class="mt-3 space-y-1">
          {#each $drives as d (d.id)}
            <li
              class="flex items-center justify-between rounded-md border border-border px-3 py-2 text-text-2"
              style="font-size: var(--ts-meta)"
            >
              <span class="truncate">
                {d.model || 'unknown'}
                <span class="font-mono text-text-3"> · {d.dev_path}</span>
              </span>
              <span
                class="shrink-0 uppercase tracking-[0.14em] text-text-3"
                style="font-size: 10px"
              >
                {d.state}
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="rounded-2xl border border-border bg-surface-1 p-4">
      <div class="font-medium text-text" style="font-size: var(--ts-title)">Host info</div>
      {#if host}
        <div class="mt-1 text-text-3" style="font-size: var(--ts-meta)">
          {host.hostname || '—'} · {host.cpu_count} CPUs
        </div>
        <div class="mt-0.5 truncate font-mono text-text-3" style="font-size: var(--ts-overline)">
          {host.kernel}
        </div>
      {:else}
        <div class="mt-1 text-text-3" style="font-size: var(--ts-meta)">Loading…</div>
      {/if}
    </div>
  </div>
</div>

<TabBar active="settings" />
