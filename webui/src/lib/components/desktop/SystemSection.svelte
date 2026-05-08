<script lang="ts">
  import { onMount } from 'svelte';
  import { settings } from '$lib/store';
  import { apiGet } from '$lib/api';

  type VersionInfo = { version?: string; commit?: string; build_date?: string };
  let version: VersionInfo | null = null;

  $: libraryPath = ($settings['library_path'] as string) ?? '';

  onMount(async () => {
    try {
      version = await apiGet<VersionInfo>('/api/version');
    } catch {
      version = null;
    }
  });
</script>

<section class="rounded-2xl border border-border bg-surface-1 p-5">
  <h2 class="text-[14px] font-semibold text-text">System</h2>
  <div class="mt-4 grid gap-3 text-[12px] text-text-2" style="grid-template-columns: 140px 1fr">
    <div>Library path</div>
    <div class="font-mono text-text">{libraryPath || 'not configured'}</div>
    <div>Build version</div>
    <div class="font-mono text-text">
      {version?.version ?? '—'}
    </div>
  </div>
</section>
