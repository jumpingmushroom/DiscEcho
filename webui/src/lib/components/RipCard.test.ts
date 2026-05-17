import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import RipCard from './RipCard.svelte';
import { logs } from '$lib/store';
import type { Drive, Disc, Job, Profile } from '$lib/wire';

const drive: Drive = {
  id: 'd1',
  model: 'ASUS SDRW-08D2S-U',
  bus: 'SR0',
  dev_path: '/dev/sr0',
  state: 'ripping',
  last_seen_at: '2026-05-13T12:00:00Z',
  read_offset: 0,
};

const disc: Disc = {
  id: 'disc-1',
  drive_id: 'd1',
  type: 'DVD',
  title: 'Jackass: The Movie',
  year: 2002,
  candidates: [],
  created_at: '2026-05-13T12:00:00Z',
};

const profile: Profile = {
  id: 'p1',
  disc_type: 'DVD',
  name: 'DVD-Movie',
  engine: 'HandBrake',
  container: 'MKV',
  video_codec: 'x264',
  quality_preset: 'x264 RF 20',
  hdr_pipeline: '',
  drive_policy: 'any',
  options: {},
  output_path_template: '',
  enabled: true,
  step_count: 7,
  created_at: '2026-05-13T12:00:00Z',
  updated_at: '2026-05-13T12:00:00Z',
};

const job: Job = {
  id: 'job-1',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'running',
  active_step: 'transcode',
  progress: 42,
  speed: '4.2x',
  eta_seconds: 125,
  steps: [
    { step: 'detect', state: 'done', attempt_count: 1 },
    { step: 'identify', state: 'done', attempt_count: 1 },
    { step: 'rip', state: 'done', attempt_count: 1 },
    { step: 'transcode', state: 'running', attempt_count: 1 },
  ],
  created_at: '2026-05-13T12:00:00Z',
};

describe('RipCard', () => {
  beforeEach(() => {
    logs.set({});
  });

  it('renders the drive bus and model in the header', () => {
    const { getByText } = render(RipCard, { drive, disc, job, profile });
    expect(getByText('SR0')).toBeInTheDocument();
    expect(getByText('ASUS SDRW-08D2S-U')).toBeInTheDocument();
  });

  it('renders the state pill derived from active_step', () => {
    const { getByText } = render(RipCard, { drive, disc, job, profile });
    expect(getByText('TRANSCODING')).toBeInTheDocument();
  });

  it('renders the AccurateRip badge when the rip step carries an accuraterip note', () => {
    const jobWithAR: Job = {
      ...job,
      steps: [
        { step: 'detect', state: 'done', attempt_count: 1 },
        { step: 'identify', state: 'done', attempt_count: 1 },
        {
          step: 'rip',
          state: 'done',
          attempt_count: 1,
          notes: {
            accuraterip: {
              status: 'verified',
              verified_tracks: 12,
              total_tracks: 12,
              max_confidence: 87,
              min_confidence: 28,
            },
          },
        },
      ],
    };
    const { getByTestId, getByText } = render(RipCard, {
      drive,
      disc,
      job: jobWithAR,
      profile,
    });
    expect(getByTestId('accuraterip-badge')).toBeInTheDocument();
    expect(getByText(/AccurateRip ✓ 12\/12/)).toBeInTheDocument();
  });

  it('omits the AccurateRip badge when no rip-step notes carry it', () => {
    const { queryByTestId } = render(RipCard, { drive, disc, job, profile });
    expect(queryByTestId('accuraterip-badge')).toBeNull();
  });

  it('falls back to drive.state when active_step is empty', () => {
    const noStepJob: Job = { ...job, active_step: undefined };
    const { getByText } = render(RipCard, { drive, disc, job: noStepJob, profile });
    expect(getByText('RIPPING')).toBeInTheDocument();
  });

  it('renders the disc title, year, and profile name', () => {
    const { getByText } = render(RipCard, { drive, disc, job, profile });
    expect(getByText('Jackass: The Movie')).toBeInTheDocument();
    expect(getByText('2002')).toBeInTheDocument();
    expect(getByText('DVD-Movie')).toBeInTheDocument();
  });

  it('renders the progress percent', () => {
    const { getByText } = render(RipCard, { drive, disc, job, profile });
    expect(getByText('42%')).toBeInTheDocument();
  });

  it('shows the speed/ETA chip exactly once — not duplicated by the stepper', () => {
    const { queryAllByText } = render(RipCard, { drive, disc, job, profile });
    expect(queryAllByText('4.2x')).toHaveLength(1);
  });

  it('shows "No log lines yet" when the store has no entries for the job', () => {
    const { getByText } = render(RipCard, { drive, disc, job, profile });
    expect(getByText('No log lines yet.')).toBeInTheDocument();
  });

  it('renders the latest log lines for the job', () => {
    logs.set({
      'job-1': [
        { job_id: 'job-1', t: '2026-05-13T12:00:01.000Z', level: 'info', message: 'first' },
        { job_id: 'job-1', t: '2026-05-13T12:00:02.000Z', level: 'info', message: 'second' },
      ],
    });
    const { getByText } = render(RipCard, { drive, disc, job, profile });
    expect(getByText('first')).toBeInTheDocument();
    expect(getByText('second')).toBeInTheDocument();
  });

  describe('log-tail backfill on mount', () => {
    let fetchSpy: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      fetchSpy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          lines: [
            {
              job_id: 'job-1',
              t: '2026-05-13T11:59:59.000Z',
              level: 'info',
              message: 'backfilled',
            },
          ],
          total: 1,
          limit: 300,
          offset: 0,
        }),
      });
      vi.stubGlobal('fetch', fetchSpy);
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it('fetches the log tail when the ring is empty for a running job', async () => {
      render(RipCard, { drive, disc, job, profile });
      await vi.waitFor(() =>
        expect(fetchSpy).toHaveBeenCalledWith(
          expect.stringContaining('/api/jobs/job-1/logs'),
          expect.any(Object),
        ),
      );
    });

    it('does not fetch when the job is terminal', async () => {
      const terminal: Job = { ...job, state: 'done', active_step: undefined };
      render(RipCard, { drive, disc, job: terminal, profile });
      // Give any unwanted onMount fetch a tick to fire.
      await new Promise((r) => setTimeout(r, 0));
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe('elapsed timer + track summary', () => {
    beforeEach(() => {
      vi.useFakeTimers();
      vi.setSystemTime(new Date('2026-05-13T12:05:30Z'));
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it('renders the elapsed time computed from job.started_at', () => {
      const running: Job = {
        ...job,
        state: 'running',
        started_at: '2026-05-13T12:00:00Z',
      };
      const { getByTestId } = render(RipCard, { drive, disc, job: running, profile });
      expect(getByTestId('elapsed').textContent?.trim()).toBe('5m 30s');
    });

    it('hides the elapsed chip on terminal jobs', () => {
      const done: Job = {
        ...job,
        state: 'done',
        active_step: undefined,
        started_at: '2026-05-13T12:00:00Z',
      };
      const { queryByTestId } = render(RipCard, { drive, disc, job: done, profile });
      expect(queryByTestId('elapsed')).toBeNull();
    });

    it('renders "N tracks · MMm" when disc.metadata_json carries tracks', () => {
      const audioDisc: Disc = {
        ...disc,
        type: 'AUDIO_CD',
        metadata_json: JSON.stringify({
          tracks: [{ duration_seconds: 357 }, { duration_seconds: 661 }, { duration_seconds: 97 }],
        }),
      };
      const { getByText } = render(RipCard, { drive, disc: audioDisc, job, profile });
      expect(getByText('3 tracks · 19m')).toBeInTheDocument();
    });
  });
});
