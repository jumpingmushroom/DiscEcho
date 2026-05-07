import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import JobDetailPanel from './JobDetailPanel.svelte';
import { discs, profiles, logs } from '$lib/store';
import type { Job, Disc, Profile } from '$lib/wire';

const dvdDisc: Disc = {
  id: 'disc-1',
  type: 'DVD',
  title: 'Arrival',
  year: 2016,
  candidates: [],
  created_at: '2026-05-07T12:00:00Z',
};

const cdProfile: Profile = {
  id: 'p1',
  disc_type: 'DVD',
  name: 'BD-1080p',
  engine: 'MakeMKV+HandBrake',
  format: 'MKV',
  preset: 'x265 RF 19 10-bit',
  options: {},
  output_path_template: '{{.Title}}.mkv',
  enabled: true,
  step_count: 7,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

const ripJob: Job = {
  id: 'job-1',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'running',
  progress: 67.0,
  speed: '4.2x',
  eta_seconds: 134,
  active_step: 'rip',
  created_at: '2026-05-07T12:00:00Z',
};

describe('JobDetailPanel', () => {
  beforeEach(() => {
    discs.set({ 'disc-1': dvdDisc });
    profiles.set([cdProfile]);
    logs.set({});
  });

  it('renders empty state when no job', () => {
    const { getByText } = render(JobDetailPanel, { job: undefined });
    expect(getByText(/click a drive or queue row/i)).toBeInTheDocument();
  });

  it('renders title, year, drive, profile when given a job', () => {
    const { getByText } = render(JobDetailPanel, { job: ripJob });
    expect(getByText('Arrival')).toBeInTheDocument();
    expect(getByText(/2016/)).toBeInTheDocument();
    expect(getByText('d1')).toBeInTheDocument();
    expect(getByText('BD-1080p')).toBeInTheDocument();
  });

  it('renders the log tail when logs exist for the job', () => {
    logs.set({
      'job-1': [
        {
          job_id: 'job-1',
          t: '2026-05-07T12:34:01.123Z',
          level: 'info',
          message: 'starting rip',
        },
        {
          job_id: 'job-1',
          t: '2026-05-07T12:34:02.456Z',
          level: 'info',
          message: 'PRGV 32%',
        },
      ],
    });
    const { getByText } = render(JobDetailPanel, { job: ripJob });
    expect(getByText('starting rip')).toBeInTheDocument();
    expect(getByText('PRGV 32%')).toBeInTheDocument();
  });

  it('renders no-log placeholder when log tail is empty', () => {
    const { getByText } = render(JobDetailPanel, { job: ripJob });
    expect(getByText(/no log lines yet/i)).toBeInTheDocument();
  });
});
