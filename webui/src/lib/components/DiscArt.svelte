<script lang="ts">
  import type { Disc } from '$lib/wire';
  import ArtPlaceholder from './ArtPlaceholder.svelte';

  export let disc: Disc | undefined = undefined;
  export let size: number = 56;
  export let ratio: 'square' | 'portrait' = 'portrait';

  // Parse metadata_json lazily; the JSON string is what we send on the
  // wire so the client absorbs the parse cost only when actually
  // rendering art. Malformed JSON falls back to an empty object so the
  // component never throws.
  function parseMetadata(s: string | undefined): Record<string, unknown> {
    if (!s) return {};
    try {
      return JSON.parse(s) as Record<string, unknown>;
    } catch {
      return {};
    }
  }

  $: meta = parseMetadata(disc?.metadata_json);

  // Cover Art Archive serves the MusicBrainz-curated release art via a
  // stable per-release URL. For audio CDs we have the release MBID in
  // disc.metadata_id, so we don't need an explicit cover_url in
  // metadata_json — fall back to the CAA endpoint when no other URL
  // is set. CAA returns 404 for releases without art; the <img> onerror
  // handler flips back to the placeholder. The {size}-pixel variant
  // matches the largest preview the design uses today.
  function caaURL(d: Disc | undefined): string | undefined {
    if (!d) return undefined;
    if (d.type !== 'AUDIO_CD') return undefined;
    if (d.metadata_provider !== 'MusicBrainz') return undefined;
    if (!d.metadata_id) return undefined;
    return `https://coverartarchive.org/release/${d.metadata_id}/front-250`;
  }

  $: posterUrl =
    (meta.poster_url as string | undefined) ??
    (meta.cover_url as string | undefined) ??
    caaURL(disc);
  $: imgHeight = ratio === 'portrait' ? Math.round(size * 1.5) : size;

  // Track which URLs have 404'd in this mount so a missing CAA image
  // collapses to the placeholder instead of looping forever. A fresh
  // mount retries (e.g. art uploaded after detection).
  let failedURL: string | undefined = undefined;
  $: showImage = posterUrl && posterUrl !== failedURL;
</script>

{#if showImage}
  <img
    src={posterUrl}
    loading="lazy"
    alt={disc?.title ?? ''}
    width={size}
    height={imgHeight}
    class="shrink-0 rounded-md"
    style="object-fit: cover; width: {size}px; height: {imgHeight}px"
    on:error={() => (failedURL = posterUrl)}
  />
{:else}
  <ArtPlaceholder label={disc?.type === 'AUDIO_CD' ? 'cd' : 'cover'} {size} {ratio} />
{/if}
