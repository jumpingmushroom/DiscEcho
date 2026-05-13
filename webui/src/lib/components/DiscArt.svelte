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
  $: posterUrl = (meta.poster_url as string | undefined) ?? (meta.cover_url as string | undefined);
  $: imgHeight = ratio === 'portrait' ? Math.round(size * 1.5) : size;
</script>

{#if posterUrl}
  <img
    src={posterUrl}
    loading="lazy"
    alt={disc?.title ?? ''}
    width={size}
    height={imgHeight}
    class="shrink-0 rounded-md"
    style="object-fit: cover; width: {size}px; height: {imgHeight}px"
  />
{:else}
  <ArtPlaceholder label={disc?.type === 'AUDIO_CD' ? 'cd' : 'cover'} {size} {ratio} />
{/if}
