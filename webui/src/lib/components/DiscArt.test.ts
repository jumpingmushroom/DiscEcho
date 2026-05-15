import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import DiscArt from './DiscArt.svelte';
import type { Disc } from '$lib/wire';

const baseDisc: Disc = {
  id: 'd1',
  type: 'DVD',
  candidates: [],
  created_at: '2026-05-13T12:00:00Z',
};

describe('DiscArt', () => {
  it('renders the placeholder when disc has no metadata_json', () => {
    const { container } = render(DiscArt, { disc: baseDisc, size: 56 });
    expect(container.querySelector('img')).toBeNull();
  });

  it('renders an <img> with poster_url when metadata_json has one', () => {
    const disc: Disc = {
      ...baseDisc,
      metadata_json: JSON.stringify({ poster_url: 'https://image.tmdb.org/t/p/w342/abc.jpg' }),
    };
    const { container } = render(DiscArt, { disc, size: 56 });
    const img = container.querySelector('img');
    expect(img).not.toBeNull();
    expect(img?.getAttribute('src')).toBe('https://image.tmdb.org/t/p/w342/abc.jpg');
    expect(img?.getAttribute('loading')).toBe('lazy');
  });

  it('uses cover_url when poster_url is absent', () => {
    const audio: Disc = {
      ...baseDisc,
      type: 'AUDIO_CD',
      metadata_json: JSON.stringify({ cover_url: 'https://coverartarchive.org/release/x/y.jpg' }),
    };
    const { container } = render(DiscArt, { disc: audio, size: 56 });
    const img = container.querySelector('img');
    expect(img?.getAttribute('src')).toContain('coverartarchive.org');
  });

  it('falls back to placeholder when metadata_json is malformed JSON', () => {
    const disc: Disc = { ...baseDisc, metadata_json: '{not json' };
    const { container } = render(DiscArt, { disc, size: 56 });
    expect(container.querySelector('img')).toBeNull();
  });

  it('handles undefined disc without throwing', () => {
    const { container } = render(DiscArt, { disc: undefined, size: 56 });
    expect(container.querySelector('img')).toBeNull();
  });

  it('falls back to the Cover Art Archive URL for audio CDs with a MusicBrainz id', () => {
    const audio: Disc = {
      ...baseDisc,
      type: 'AUDIO_CD',
      metadata_provider: 'MusicBrainz',
      metadata_id: '7ad65c20-9abd-4e78-afcb-39b1ed445041',
    };
    const { container } = render(DiscArt, { disc: audio, size: 56 });
    const img = container.querySelector('img');
    expect(img?.getAttribute('src')).toBe(
      'https://coverartarchive.org/release/7ad65c20-9abd-4e78-afcb-39b1ed445041/front-250',
    );
  });

  it('prefers an explicit cover_url over the CAA fallback', () => {
    const audio: Disc = {
      ...baseDisc,
      type: 'AUDIO_CD',
      metadata_provider: 'MusicBrainz',
      metadata_id: '7ad65c20-9abd-4e78-afcb-39b1ed445041',
      metadata_json: JSON.stringify({ cover_url: 'https://example.com/custom.jpg' }),
    };
    const { container } = render(DiscArt, { disc: audio, size: 56 });
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'https://example.com/custom.jpg',
    );
  });

  it('does not synthesise a CAA URL for non-audio discs', () => {
    const dvd: Disc = {
      ...baseDisc,
      metadata_provider: 'MusicBrainz',
      metadata_id: 'whatever',
    };
    const { container } = render(DiscArt, { disc: dvd, size: 56 });
    expect(container.querySelector('img')).toBeNull();
  });
});
