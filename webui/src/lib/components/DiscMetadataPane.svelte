<script lang="ts">
  import type { Disc } from '$lib/wire';
  import DiscArt from './DiscArt.svelte';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import { formatDuration } from '$lib/time';

  export let disc: Disc | undefined = undefined;

  type Movie = {
    plot?: string;
    director?: string;
    cast?: string[];
    studio?: string;
    genres?: string[];
    rating?: number;
    poster_url?: string;
    dvd_titles?: Array<{ Number: number; DurationSeconds: number }>;
  };
  type Audio = {
    label?: string;
    catalog_number?: string;
    tracks?: Array<{ number: number; title: string; duration_seconds?: number }>;
    cover_url?: string;
  };
  type Game = {
    system?: string;
    serial?: string;
    redump_md5?: string;
  };

  function parseJSON<T>(s: string | undefined): T {
    if (!s) return {} as T;
    try {
      return JSON.parse(s) as T;
    } catch {
      return {} as T;
    }
  }

  type Kind = 'movie' | 'audio' | 'game' | 'data' | 'unknown';

  function computeKind(d: Disc | undefined): Kind {
    if (!d) return 'unknown';
    if (d.type === 'AUDIO_CD') return 'audio';
    if (
      d.type === 'PSX' ||
      d.type === 'PS2' ||
      d.type === 'XBOX' ||
      d.type === 'SAT' ||
      d.type === 'DC'
    ) {
      return 'game';
    }
    if (d.type === 'DATA') return 'data';
    return 'movie';
  }

  function tabsFor(k: Kind): string[] {
    switch (k) {
      case 'movie':
        return ['Overview', 'Cast', 'Files'];
      case 'audio':
        return ['Overview', 'Tracks'];
      case 'game':
        return ['Overview'];
      case 'data':
        return ['Overview', 'Files'];
      default:
        return ['Overview', 'Files'];
    }
  }

  function topCandidateArtist(d: Disc | undefined): string {
    return d?.candidates?.[0]?.artist ?? '';
  }

  $: hasMetadata = !!disc?.metadata_json && disc.metadata_json !== '{}';
  $: kind = computeKind(disc);
  $: tabs = tabsFor(kind);
  let activeTab = 'Overview';
  $: if (tabs && !tabs.includes(activeTab)) activeTab = tabs[0];

  $: movieMeta = parseJSON<Movie>(disc?.metadata_json);
  $: audioMeta = parseJSON<Audio>(disc?.metadata_json);
  $: gameMeta = parseJSON<Game>(disc?.metadata_json);
</script>

<div class="rounded-2xl border border-border bg-surface-1 p-5">
  {#if !disc}
    <div class="py-12 text-center text-[13px] text-text-3">
      Click a queue row to inspect a disc.
    </div>
  {:else}
    <!-- Header row: art + title + sub-line -->
    <div class="flex items-center gap-3">
      <DiscArt {disc} size={56} ratio={disc.type === 'AUDIO_CD' ? 'square' : 'portrait'} />
      <div class="min-w-0 flex-1">
        {#if hasMetadata}
          <div class="text-[15px] font-semibold text-text">{disc.title || 'Unknown'}</div>
        {:else}
          <div class="text-[15px] font-semibold text-text-3">Unknown disc</div>
        {/if}
        <div class="mt-0.5 text-[11px] text-text-3">
          {#if kind === 'movie'}
            {disc.year ?? ''}{movieMeta.genres?.length
              ? ` · ${movieMeta.genres[0]}`
              : ''}{typeof movieMeta.rating === 'number'
              ? ` · ★ ${movieMeta.rating.toFixed(1)}`
              : ''}
          {:else if kind === 'audio'}
            {topCandidateArtist(disc)}{disc.year ? ` · ${disc.year}` : ''}
          {:else if kind === 'game'}
            {gameMeta.system ?? disc.type}{disc.candidates[0]?.region
              ? ` · ${disc.candidates[0].region}`
              : ''}
          {:else if kind === 'data'}
            {disc.type}
          {:else}
            {disc.type} · awaiting decision
          {/if}
        </div>
      </div>
      <DiscTypeBadge type={disc.type} />
    </div>

    <!-- Tabs -->
    <div class="mt-4 flex border-b border-border" role="tablist">
      {#each tabs as t}
        <button
          type="button"
          class="flex-1 py-2 text-[11px] font-medium uppercase tracking-[0.12em]"
          style="color: {activeTab === t
            ? 'var(--accent)'
            : 'var(--text-3)'}; border-bottom: 2px solid {activeTab === t
            ? 'var(--accent)'
            : 'transparent'}"
          on:click={() => (activeTab = t)}
        >
          {t}
        </button>
      {/each}
    </div>

    <!-- Tab body -->
    <div class="mt-4 text-[12px] text-text-2">
      {#if activeTab === 'Overview'}
        {#if kind === 'movie'}
          {#if movieMeta.plot}
            <p class="italic text-text-2">{movieMeta.plot}</p>
          {/if}
          <dl class="mt-3 grid gap-x-3 gap-y-1.5" style="grid-template-columns: auto 1fr">
            {#if movieMeta.director}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Director</dt>
              <dd>{movieMeta.director}</dd>
            {/if}
            {#if movieMeta.studio}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Studio</dt>
              <dd>{movieMeta.studio}</dd>
            {/if}
          </dl>
        {:else if kind === 'audio'}
          <dl class="grid gap-x-3 gap-y-1.5" style="grid-template-columns: auto 1fr">
            {#if topCandidateArtist(disc)}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Artist</dt>
              <dd>{topCandidateArtist(disc)}</dd>
            {/if}
            {#if audioMeta.label}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Label</dt>
              <dd>
                {audioMeta.label}{audioMeta.catalog_number ? ` · ${audioMeta.catalog_number}` : ''}
              </dd>
            {/if}
          </dl>
        {:else if kind === 'game'}
          <dl class="grid gap-x-3 gap-y-1.5" style="grid-template-columns: auto 1fr">
            {#if gameMeta.system}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">System</dt>
              <dd>{gameMeta.system}</dd>
            {/if}
            {#if disc.candidates[0]?.region}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Region</dt>
              <dd>{disc.candidates[0].region}</dd>
            {/if}
            {#if gameMeta.serial}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Serial</dt>
              <dd>{gameMeta.serial}</dd>
            {/if}
            {#if gameMeta.redump_md5}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Redump MD5</dt>
              <dd class="truncate font-mono text-[11px]">{gameMeta.redump_md5}</dd>
            {/if}
          </dl>
        {:else}
          <dl class="grid gap-x-3 gap-y-1.5" style="grid-template-columns: auto 1fr">
            <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Type</dt>
            <dd>{disc.type}</dd>
            {#if disc.title}
              <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Volume label</dt>
              <dd>{disc.title}</dd>
            {/if}
            <dt class="text-[10px] uppercase tracking-[0.12em] text-text-3">Disc ID</dt>
            <dd class="font-mono text-[11px]">{disc.id.slice(0, 16)}…</dd>
          </dl>
        {/if}
      {:else if activeTab === 'Cast'}
        {#if movieMeta.cast?.length}
          <ul class="space-y-1.5">
            {#each movieMeta.cast as actor}
              <li>{actor}</li>
            {/each}
          </ul>
        {:else}
          <p class="text-text-3">No cast information.</p>
        {/if}
      {:else if activeTab === 'Tracks'}
        {#if audioMeta.tracks?.length}
          <ol class="space-y-1.5">
            {#each audioMeta.tracks as t}
              <li class="grid gap-3" style="grid-template-columns: 24px 1fr auto">
                <span class="text-right font-mono text-text-3">{t.number}</span>
                <span>{t.title}</span>
                <span class="font-mono text-text-3"
                  >{t.duration_seconds ? formatDuration(t.duration_seconds) : ''}</span
                >
              </li>
            {/each}
          </ol>
        {:else}
          <p class="text-text-3">No track information.</p>
        {/if}
      {:else if activeTab === 'Files'}
        {#if kind === 'movie' && movieMeta.dvd_titles?.length}
          <ul class="space-y-1 font-mono text-[11px]">
            {#each movieMeta.dvd_titles as t}
              <li>title {t.Number} · {formatDuration(t.DurationSeconds)}</li>
            {/each}
          </ul>
        {:else}
          <p class="text-text-3">No file inventory available.</p>
        {/if}
      {/if}
    </div>
  {/if}
</div>
