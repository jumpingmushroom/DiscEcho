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
  // disc.metadata_id; specific releases (a particular pressing) often
  // have no art on CAA, but the release-group nearly always does — so
  // we build a candidate list and walk it on <img> error. CAA returns
  // 404 for releases/groups without art; the onerror handler advances
  // to the next URL, then to the placeholder. The {size}-pixel variant
  // matches the largest preview the design uses today.
  function caaCandidates(d: Disc | undefined, m: Record<string, unknown>): string[] {
    const urls: string[] = [];
    const explicit = (m.poster_url as string | undefined) ?? (m.cover_url as string | undefined);
    if (explicit) urls.push(explicit);
    if (d?.type === 'AUDIO_CD' && d.metadata_provider === 'MusicBrainz') {
      if (d.metadata_id) {
        urls.push(`https://coverartarchive.org/release/${d.metadata_id}/front-250`);
      }
      const rgID = m.release_group_mbid as string | undefined;
      if (rgID) {
        urls.push(`https://coverartarchive.org/release-group/${rgID}/front-250`);
      }
    }
    return urls;
  }

  $: candidates = caaCandidates(disc, meta);
  $: imgHeight = ratio === 'portrait' ? Math.round(size * 1.5) : size;

  // Walk through the candidate URL list on <img> error. A fresh mount
  // resets to index 0 (e.g. art uploaded after detection).
  let cursor = 0;
  $: if (candidates) cursor = 0;
  $: posterUrl = candidates[cursor];
  $: showImage = posterUrl !== undefined;
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
    on:error={() => (cursor += 1)}
  />
{:else}
  <ArtPlaceholder label={disc?.type === 'AUDIO_CD' ? 'cd' : 'cover'} {size} {ratio} />
{/if}
