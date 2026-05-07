import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import DiscIdSheet from './DiscIdSheet.svelte';
import { profiles, pendingDiscID } from '$lib/store';
import type { Disc, Profile } from '$lib/wire';

const cdProfile: Profile = {
  id: 'p-cd',
  disc_type: 'AUDIO_CD',
  name: 'CD-FLAC',
  engine: 'whipper',
  format: 'FLAC',
  preset: 'AccurateRip',
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

  it('skip clears pendingDiscID without firing API', async () => {
    const { getByText } = render(DiscIdSheet, { disc });
    await fireEvent.click(getByText('Skip identification'));
    expect(get(pendingDiscID)).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
