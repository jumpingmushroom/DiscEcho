import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import JobRow from './JobRow.svelte';
import { discs } from '$lib/store';
import type { Job, Disc } from '$lib/wire';

const seedDisc: Disc = {
  id: 'disc-1',
  type: 'DVD',
  title: 'Arrival',
  year: 2016,
  candidates: [],
  created_at: '2026-05-07T12:00:00Z',
};

const runningJob: Job = {
  id: 'job-1',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'running',
  progress: 42.5,
  speed: '4.2x',
  eta_seconds: 90,
  active_step: 'rip',
  created_at: '2026-05-07T12:00:00Z',
};

const queuedJob: Job = {
  id: 'job-2',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'queued',
  progress: 0,
  created_at: '2026-05-07T12:01:00Z',
};

describe('JobRow', () => {
  beforeEach(() => {
    discs.set({ 'disc-1': seedDisc });
    return () => discs.set({});
  });

  it('renders title and disc-type badge', () => {
    const { getByText } = render(JobRow, { job: runningJob });
    expect(getByText('Arrival')).toBeInTheDocument();
    expect(getByText('DVD')).toBeInTheDocument();
  });

  it('renders the drive-id chip from job.drive_id', () => {
    const { getByText } = render(JobRow, { job: runningJob });
    expect(getByText('d1')).toBeInTheDocument();
  });

  it('renders progress percent and formatted ETA for running jobs', () => {
    const { getByText, queryByText } = render(JobRow, { job: runningJob });
    expect(getByText('43%')).toBeInTheDocument();
    expect(getByText('1m 30s')).toBeInTheDocument();
    expect(queryByText('90s')).toBeNull();
    expect(queryByText('QUEUED')).toBeNull();
  });

  it('formats hour-scale ETA as Xh Ym Zs', () => {
    const longJob: Job = { ...runningJob, eta_seconds: 3725 };
    const { getByText } = render(JobRow, { job: longJob });
    expect(getByText('1h 2m 5s')).toBeInTheDocument();
  });

  it('hides percent + ETA, shows QUEUED badge for queued jobs', () => {
    const { getByText, queryByText } = render(JobRow, { job: queuedJob });
    expect(getByText('QUEUED')).toBeInTheDocument();
    expect(queryByText('0%')).toBeNull();
  });

  it('renders Unknown title when disc not in store', () => {
    discs.set({});
    const { getByText } = render(JobRow, { job: runningJob });
    expect(getByText('Unknown')).toBeInTheDocument();
  });

  it('dispatches click event when tapped', async () => {
    const onClick = vi.fn();
    const { container, component } = render(JobRow, { job: runningJob });
    component.$on('click', onClick);
    await fireEvent.click(container.querySelector('button')!);
    expect(onClick).toHaveBeenCalled();
  });
});
