import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import AwaitingDecisionList from './AwaitingDecisionList.svelte';
import { discs, jobs, profiles } from '$lib/store';
import type { Disc, Job, Profile } from '$lib/wire';

const baseProfile: Profile = {
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

function disc(id: string, opts: Partial<Disc> = {}): Disc {
  return {
    id,
    drive_id: 'd1',
    type: 'DVD',
    candidates: [
      { source: 'TMDB', title: `Disc ${id}`, year: 2020, confidence: 0, media_type: 'movie' },
    ],
    created_at: `2026-05-12T10:00:0${id}Z`,
    ...opts,
  };
}

function job(discID: string, state: Job['state']): Job {
  return {
    id: `job-${discID}`,
    disc_id: discID,
    drive_id: 'd1',
    profile_id: 'p-dvd',
    state,
    progress: 0,
    created_at: '2026-05-12T10:00:00Z',
  };
}

describe('AwaitingDecisionList', () => {
  beforeEach(() => {
    profiles.set([baseProfile]);
    discs.set({});
    jobs.set([]);
  });

  it('renders nothing when no discs need decision', async () => {
    const { container } = render(AwaitingDecisionList);
    await tick();
    expect(container.querySelector('[data-testid="awaiting-decision-list"]')).toBeNull();
  });

  it('shows discs with candidates and no job', async () => {
    discs.set({ '1': disc('1'), '2': disc('2') });
    const { getAllByText } = render(AwaitingDecisionList);
    await tick();
    expect(getAllByText(/Awaiting decision/i).length).toBe(2);
  });

  it('hides discs whose job already ran (any state, including terminal)', async () => {
    discs.set({
      '1': disc('1'), // queued → hidden
      '2': disc('2'), // running → hidden
      '3': disc('3'), // done → hidden (the bug fix)
      '4': disc('4'), // failed → hidden
      '5': disc('5'), // no job → shown
    });
    jobs.set([job('1', 'queued'), job('2', 'running'), job('3', 'done'), job('4', 'failed')]);
    const { getAllByText } = render(AwaitingDecisionList);
    await tick();
    expect(getAllByText(/Awaiting decision/i).length).toBe(1);
    expect(getAllByText('Disc 5').length).toBe(1);
  });

  it('caps the list at 3 cards', async () => {
    discs.set({
      '1': disc('1'),
      '2': disc('2'),
      '3': disc('3'),
      '4': disc('4'),
      '5': disc('5'),
    });
    const { getAllByText } = render(AwaitingDecisionList);
    await tick();
    expect(getAllByText(/Awaiting decision/i).length).toBe(3);
  });

  it('surfaces audio CDs even when MusicBrainz returned no candidates', async () => {
    // Pre-fix this disc would have been filtered out and the dashboard
    // would have silently snapped back to idle after the insert.
    discs.set({
      '1': disc('1', { type: 'AUDIO_CD', candidates: [], title: '' }),
    });
    const { getAllByText } = render(AwaitingDecisionList);
    await tick();
    expect(getAllByText(/Awaiting decision/i).length).toBe(1);
  });
});
