import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import DiscMetadataPane from './DiscMetadataPane.svelte';
import type { Disc } from '$lib/wire';

function dvdDisc(metadata: object): Disc {
  return {
    id: 'd1',
    type: 'DVD',
    title: 'Jackass: The Movie',
    year: 2002,
    candidates: [],
    created_at: '2026-05-13T12:00:00Z',
    metadata_json: JSON.stringify(metadata),
  };
}

function audioDisc(metadata: object): Disc {
  return {
    id: 'd2',
    type: 'AUDIO_CD',
    title: 'Kind of Blue',
    candidates: [
      {
        source: 'MusicBrainz',
        title: 'Kind of Blue',
        artist: 'Miles Davis',
        year: 1959,
        confidence: 100,
      },
    ],
    created_at: '2026-05-13T12:00:00Z',
    metadata_json: JSON.stringify(metadata),
  };
}

describe('DiscMetadataPane', () => {
  it('renders movie Overview tab by default with plot + director', () => {
    const disc = dvdDisc({
      plot: 'Lorem ipsum plot.',
      director: 'Jeff Tremaine',
      genres: ['Comedy'],
      rating: 6.2,
    });
    const { getByText } = render(DiscMetadataPane, { disc });
    expect(getByText('Jackass: The Movie')).toBeInTheDocument();
    expect(getByText(/Lorem ipsum plot/)).toBeInTheDocument();
    expect(getByText('Jeff Tremaine')).toBeInTheDocument();
  });

  it('switches to Cast tab on click', async () => {
    const disc = dvdDisc({ cast: ['Knoxville', 'Bam', 'Steve-O'] });
    const { getByText } = render(DiscMetadataPane, { disc });
    await fireEvent.click(getByText('Cast'));
    expect(getByText('Knoxville')).toBeInTheDocument();
    expect(getByText('Bam')).toBeInTheDocument();
    expect(getByText('Steve-O')).toBeInTheDocument();
  });

  it('renders audio CD Tracks tab with track list', async () => {
    const disc = audioDisc({
      tracks: [
        { number: 1, title: 'So What', duration_seconds: 562 },
        { number: 2, title: 'Freddie Freeloader', duration_seconds: 586 },
      ],
    });
    const { getByText } = render(DiscMetadataPane, { disc });
    await fireEvent.click(getByText('Tracks'));
    expect(getByText('So What')).toBeInTheDocument();
    expect(getByText('9m 22s')).toBeInTheDocument(); // 562s formatted
  });

  it('renders game pane with system and serial', () => {
    const disc: Disc = {
      id: 'g1',
      type: 'PSX',
      title: 'Final Fantasy VII',
      candidates: [],
      created_at: '2026-05-13T12:00:00Z',
      metadata_json: JSON.stringify({
        system: 'Sony PlayStation',
        serial: 'SCUS-94163',
      }),
    };
    const { getAllByText, getByText } = render(DiscMetadataPane, { disc });
    // "Sony PlayStation" appears in both the sub-line and the Overview grid
    expect(getAllByText('Sony PlayStation').length).toBeGreaterThanOrEqual(1);
    expect(getByText('SCUS-94163')).toBeInTheDocument();
    expect(getByText('Final Fantasy VII')).toBeInTheDocument();
  });

  it('renders unidentified placeholder when metadata is empty', () => {
    const disc: Disc = {
      id: 'd3',
      type: 'DVD',
      candidates: [],
      created_at: '2026-05-13T12:00:00Z',
    };
    const { getByText } = render(DiscMetadataPane, { disc });
    expect(getByText(/Unknown disc/i)).toBeInTheDocument();
  });

  it('shows the disc title when known even without a rich metadata blob', () => {
    const disc: Disc = {
      id: 'd4',
      type: 'DVD',
      title: 'March of the Penguins',
      year: 2005,
      candidates: [],
      created_at: '2026-05-13T12:00:00Z',
      metadata_json: '{}',
    };
    const { getByText, queryByText } = render(DiscMetadataPane, { disc });
    expect(getByText('March of the Penguins')).toBeInTheDocument();
    expect(queryByText(/Unknown disc/i)).not.toBeInTheDocument();
  });

  it('falls back to the top candidate title when the disc has no title', () => {
    const disc: Disc = {
      id: 'd5',
      type: 'DVD',
      candidates: [{ source: 'TMDB', title: 'Arrival', year: 2016, confidence: 80 }],
      created_at: '2026-05-13T12:00:00Z',
    };
    const { getByText } = render(DiscMetadataPane, { disc });
    expect(getByText('Arrival')).toBeInTheDocument();
  });

  it('shows empty-state placeholder when no disc', () => {
    const { getByText } = render(DiscMetadataPane, { disc: undefined });
    expect(getByText(/Click a queue row/i)).toBeInTheDocument();
  });
});
