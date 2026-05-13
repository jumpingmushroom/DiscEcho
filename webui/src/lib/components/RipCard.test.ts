import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
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
  auto_eject: true,
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
});
