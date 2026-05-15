import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import AwaitingDecisionCard from './AwaitingDecisionCard.svelte';
import { profiles, settings } from '$lib/store';
import type { Disc, Profile } from '$lib/wire';

const dvdProfile: Profile = {
  id: 'p-dvd',
  disc_type: 'DVD',
  name: 'DVD-Movie',
  engine: 'HandBrake',
  format: 'MP4',
  preset: '',
  container: 'MP4',
  video_codec: 'x264',
  quality_preset: '',
  hdr_pipeline: '',
  drive_policy: 'any',
  options: {},
  output_path_template: '{{.Title}}.mp4',
  enabled: true,
  step_count: 7,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

const highConfDisc: Disc = {
  id: 'disc-1',
  drive_id: 'd1',
  type: 'DVD',
  candidates: [
    { source: 'TMDB', title: 'Blade Runner 2049', year: 2017, confidence: 92, media_type: 'movie' },
  ],
  created_at: '2026-05-12T08:00:00Z',
};

const lowConfDisc: Disc = {
  ...highConfDisc,
  id: 'disc-low',
  candidates: [
    { source: 'TMDB', title: 'Jackass: The Movie', year: 2002, confidence: 0, media_type: 'movie' },
    { source: 'TMDB', title: 'Jackass Number Two', year: 2006, confidence: 0, media_type: 'movie' },
  ],
};

describe('AwaitingDecisionCard', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    profiles.set([dvdProfile]);
    settings.set({ 'operation.mode': 'batch' });
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
    settings.set({});
  });

  it('renders the candidate list and counts the matches', () => {
    const { getByText } = render(AwaitingDecisionCard, { disc: lowConfDisc });
    expect(getByText(/2 matches/)).toBeInTheDocument();
    expect(getByText('Jackass: The Movie')).toBeInTheDocument();
    expect(getByText('Jackass Number Two')).toBeInTheDocument();
  });

  it('auto-rips when top confidence ≥ 50', async () => {
    render(AwaitingDecisionCard, { disc: highConfDisc });
    await tick();
    await vi.advanceTimersByTimeAsync(8000);
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/disc-1/start',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('does not auto-rip when top confidence is below 50', async () => {
    const { getByText, queryByText } = render(AwaitingDecisionCard, { disc: lowConfDisc });
    await tick();
    expect(getByText(/No confident match · pick a title or search/)).toBeInTheDocument();
    expect(queryByText(/Auto-rip in/)).toBeNull();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('Use top match · Start rip button posts to /start', async () => {
    const { getByText } = render(AwaitingDecisionCard, { disc: lowConfDisc });
    await fireEvent.click(getByText('Use top match · Start rip'));
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/disc-low/start',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('a second rapid click on Use top match does not fire a second /start', async () => {
    const { getByText } = render(AwaitingDecisionCard, { disc: lowConfDisc });
    const btn = getByText('Use top match · Start rip');
    await fireEvent.click(btn);
    await fireEvent.click(btn);
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it('auto-confirm and a manual click cannot both fire /start', async () => {
    const { getByText } = render(AwaitingDecisionCard, { disc: highConfDisc });
    await tick();
    // Halfway through the 8-second auto-confirm countdown the user
    // clicks Use top match. Both code paths must coalesce to one POST.
    await vi.advanceTimersByTimeAsync(4000);
    await fireEvent.click(getByText('Use top match · Start rip'));
    await vi.advanceTimersByTimeAsync(8000);
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it('renders inline (not wrapped in a bottom sheet dialog)', () => {
    const { container } = render(AwaitingDecisionCard, { disc: lowConfDisc });
    // BottomSheet renders a [role=dialog] backdrop; this component must not.
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it('manual mode suppresses the countdown even when confidence is high', async () => {
    settings.set({ 'operation.mode': 'manual' });
    const { getByText, queryByText } = render(AwaitingDecisionCard, { disc: highConfDisc });
    await tick();
    expect(queryByText(/Auto-rip in/)).toBeNull();
    expect(getByText(/Manual mode · pick a title to rip/)).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
