import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import DiscIdSheet from './DiscIdSheet.svelte';
import { profiles, pendingDiscID, discs } from '$lib/store';
import type { Disc, Profile } from '$lib/wire';

const cdProfile: Profile = {
  id: 'p-cd',
  disc_type: 'AUDIO_CD',
  name: 'CD-FLAC',
  engine: 'whipper',
  format: 'FLAC',
  preset: 'AccurateRip',
  container: 'FLAC',
  video_codec: '',
  quality_preset: 'AccurateRip',
  hdr_pipeline: '',
  drive_policy: 'any',
  options: {},
  output_path_template: '{{.Title}}',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

const disc: Disc = {
  id: 'disc-1',
  drive_id: 'd1',
  type: 'AUDIO_CD',
  candidates: [
    {
      source: 'MusicBrainz',
      title: 'Kind of Blue',
      artist: 'Miles Davis',
      confidence: 94,
      mbid: 'mb-1',
    },
    {
      source: 'MusicBrainz',
      title: 'Kind of Blue (Remaster)',
      confidence: 81,
      mbid: 'mb-2',
    },
  ],
  created_at: '2026-05-07T12:00:00Z',
};

describe('DiscIdSheet', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    profiles.set([cdProfile]);
    pendingDiscID.set('disc-1');
    fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 'job-new' }),
    });
    vi.stubGlobal('fetch', fetchSpy);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('renders disc title and candidates', () => {
    const { getByText } = render(DiscIdSheet, { disc });
    expect(getByText(/Audio CD · 2 matches/)).toBeInTheDocument();
    expect(getByText('Kind of Blue')).toBeInTheDocument();
  });

  it('counts down from 8 seconds', async () => {
    const { getByText } = render(DiscIdSheet, { disc });
    await tick();
    expect(getByText(/Auto-rip in 8s/)).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(1000);
    await tick();
    expect(getByText(/Auto-rip in 7s/)).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(2000);
    await tick();
    expect(getByText(/Auto-rip in 5s/)).toBeInTheDocument();
  });

  it('user click cancels the timer', async () => {
    const { getByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Use top match · Start rip'));
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/disc-1/start',
      expect.objectContaining({ method: 'POST' }),
    );
    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it('autoConfirms after 8 seconds with candidate_index=0', async () => {
    render(DiscIdSheet, { disc });
    await tick();
    await vi.advanceTimersByTimeAsync(8000);
    expect(fetchSpy).toHaveBeenCalled();
    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(body).toEqual({ profile_id: 'p-cd', candidate_index: 0 });
  });

  it('does not auto-rip when top confidence is below 50', async () => {
    const lowConfDisc: Disc = {
      ...disc,
      candidates: [
        { source: 'TMDB', title: 'Maybe Match', confidence: 12 },
        { source: 'TMDB', title: 'Other Maybe', confidence: 5 },
      ],
    };
    const { getByText, queryByText } = render(DiscIdSheet, { disc: lowConfDisc });
    await tick();
    expect(getByText(/No confident match · pick a title or search/)).toBeInTheDocument();
    expect(queryByText(/Auto-rip in/)).toBeNull();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('shows "No match found" when candidate list is empty', async () => {
    const emptyDisc: Disc = { ...disc, candidates: [] };
    const { getByText, queryByText } = render(DiscIdSheet, { disc: emptyDisc });
    await tick();
    expect(getByText(/No match found · search manually/)).toBeInTheDocument();
    expect(queryByText(/Auto-rip in/)).toBeNull();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('skip clears pendingDiscID without firing API', async () => {
    const { getByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Skip identification'));
    expect(get(pendingDiscID)).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});

describe('DiscIdSheet — candidate-driven profile binding (M2.2)', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    profiles.set([
      cdProfile,
      {
        id: 'p-dvd-movie',
        disc_type: 'DVD',
        name: 'DVD-Movie',
        engine: 'HandBrake',
        format: 'MP4',
        preset: 'x264 RF 20',
        container: 'MP4',
        video_codec: 'x264',
        quality_preset: 'x264 RF 20',
        hdr_pipeline: '',
        drive_policy: 'any',
        options: {},
        output_path_template: '{{.Title}}.mp4',
        enabled: true,
        step_count: 7,
        created_at: '2026-05-07T12:00:00Z',
        updated_at: '2026-05-07T12:00:00Z',
      },
      {
        id: 'p-dvd-series',
        disc_type: 'DVD',
        name: 'DVD-Series',
        engine: 'HandBrake',
        format: 'MKV',
        preset: 'x264 RF 20 per-title',
        container: 'MKV',
        video_codec: 'x264',
        quality_preset: 'x264 RF 20 per-title',
        hdr_pipeline: '',
        drive_policy: 'any',
        options: {},
        output_path_template: '{{.Show}}/{{.EpisodeNumber}}.mkv',
        enabled: true,
        step_count: 7,
        created_at: '2026-05-07T12:00:00Z',
        updated_at: '2026-05-07T12:00:00Z',
      },
    ]);
    pendingDiscID.set(null);
    fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 'job-new' }),
    });
    vi.stubGlobal('fetch', fetchSpy);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('shows FILM badge for movie candidates', () => {
    const dvdDisc = {
      id: 'disc-d1',
      drive_id: 'd1',
      type: 'DVD' as const,
      candidates: [
        {
          source: 'TMDB',
          title: 'Arrival',
          year: 2016,
          confidence: 80,
          tmdb_id: 1,
          media_type: 'movie' as const,
        },
      ],
      created_at: '2026-05-07T12:00:00Z',
    };
    discs.set({ 'disc-d1': dvdDisc });
    const { getByText } = render(DiscIdSheet, { disc: dvdDisc });
    expect(getByText('FILM')).toBeInTheDocument();
  });

  it('shows TV badge for tv candidates', () => {
    const dvdDisc = {
      id: 'disc-d2',
      drive_id: 'd1',
      type: 'DVD' as const,
      candidates: [
        {
          source: 'TMDB',
          title: 'Friends',
          year: 1994,
          confidence: 90,
          tmdb_id: 1668,
          media_type: 'tv' as const,
        },
      ],
      created_at: '2026-05-07T12:00:00Z',
    };
    discs.set({ 'disc-d2': dvdDisc });
    const { getByText } = render(DiscIdSheet, { disc: dvdDisc });
    expect(getByText('TV')).toBeInTheDocument();
  });

  it('clicking tv candidate starts job with DVD-Series profile', async () => {
    const dvdDisc = {
      id: 'disc-d3',
      drive_id: 'd1',
      type: 'DVD' as const,
      candidates: [
        {
          source: 'TMDB',
          title: 'Friends',
          year: 1994,
          confidence: 90,
          tmdb_id: 1668,
          media_type: 'tv' as const,
        },
      ],
      created_at: '2026-05-07T12:00:00Z',
    };
    discs.set({ 'disc-d3': dvdDisc });
    const { getByText } = render(DiscIdSheet, { disc: dvdDisc });

    await fireEvent.click(getByText('Use top match · Start rip'));

    expect(fetchSpy).toHaveBeenCalled();
    const url = fetchSpy.mock.calls[0][0] as string;
    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(url).toBe('/api/discs/disc-d3/start');
    expect(body.profile_id).toBe('p-dvd-series');
  });

  it('clicking movie candidate starts job with DVD-Movie profile', async () => {
    const dvdDisc = {
      id: 'disc-d4',
      drive_id: 'd1',
      type: 'DVD' as const,
      candidates: [
        {
          source: 'TMDB',
          title: 'Arrival',
          year: 2016,
          confidence: 80,
          tmdb_id: 329865,
          media_type: 'movie' as const,
        },
      ],
      created_at: '2026-05-07T12:00:00Z',
    };
    discs.set({ 'disc-d4': dvdDisc });
    const { getByText } = render(DiscIdSheet, { disc: dvdDisc });

    await fireEvent.click(getByText('Use top match · Start rip'));

    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(body.profile_id).toBe('p-dvd-movie');
  });

  it('AUDIO_CD candidate (no media_type) starts CD-FLAC profile', async () => {
    discs.set({ 'disc-1': disc });
    const { getByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Use top match · Start rip'));
    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(body.profile_id).toBe('p-cd');
  });
});

describe('DiscIdSheet — manual search (M2.2)', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    profiles.set([cdProfile]);
    pendingDiscID.set('disc-1');
    discs.set({ 'disc-1': disc });
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('Search manually replaces candidate list with input', async () => {
    const { getByText, getByPlaceholderText, queryByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Search manually'));
    expect(getByPlaceholderText(/album or artist/i)).toBeInTheDocument();
    expect(queryByText('Use top match · Start rip')).toBeNull();
  });

  it('disables Search button when query is empty', async () => {
    const { getByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Search manually'));
    const searchBtn = getByText(/Search MusicBrainz/);
    expect((searchBtn as HTMLButtonElement).disabled).toBe(true);
  });

  it('submitting non-empty query calls manualIdentify and closes panel on success', async () => {
    const newCands = [
      {
        source: 'TMDB',
        title: 'Found',
        year: 2020,
        confidence: 80,
        tmdb_id: 99,
        media_type: 'movie' as const,
      },
    ];
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        disc: { ...disc, candidates: newCands },
        candidates: newCands,
      }),
    });

    const { getByText, getByPlaceholderText, queryByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Search manually'));
    await fireEvent.input(getByPlaceholderText(/album or artist/i), { target: { value: 'Found' } });
    await fireEvent.click(getByText(/Search MusicBrainz/));

    await vi.runAllTimersAsync();
    await Promise.resolve();
    await Promise.resolve();
    expect(queryByText(/Search MusicBrainz/)).toBeNull();

    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/disc-1/identify',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('shows "No matches found" when search returns empty list', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        disc: { ...disc, candidates: [] },
        candidates: [],
      }),
    });

    const { getByText, getByPlaceholderText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Search manually'));
    await fireEvent.input(getByPlaceholderText(/album or artist/i), {
      target: { value: 'Obscure' },
    });
    await fireEvent.click(getByText(/Search MusicBrainz/));

    await vi.runAllTimersAsync();
    await Promise.resolve();
    await Promise.resolve();
    expect(getByText(/No matches found/i)).toBeInTheDocument();
  });

  it('shows error message when search fails', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 502,
      text: async () => 'tmdb down',
    });

    const { getByText, getByPlaceholderText, container } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Search manually'));
    await fireEvent.input(getByPlaceholderText(/album or artist/i), { target: { value: 'X' } });
    await fireEvent.click(getByText(/Search MusicBrainz/));

    await vi.runAllTimersAsync();
    await Promise.resolve();
    await Promise.resolve();
    expect(container.textContent).toMatch(/502/);
  });
});
