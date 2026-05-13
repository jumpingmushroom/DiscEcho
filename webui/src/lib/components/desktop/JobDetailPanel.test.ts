import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import JobDetailPanel from './JobDetailPanel.svelte';
import { discs, drives, profiles, logs } from '$lib/store';
import type { Job, Disc, Drive } from '$lib/wire';

const sr0: Drive = {
  id: 'd1',
  model: 'ASUS SDRW-08D2S-U',
  bus: 'SR0',
  dev_path: '/dev/sr0',
  state: 'ripping',
  last_seen_at: '2026-05-07T12:00:00Z',
};

const dvdDisc: Disc = {
  id: 'disc-1',
  type: 'DVD',
  title: 'Arrival',
  year: 2016,
  candidates: [],
  created_at: '2026-05-07T12:00:00Z',
  metadata_json: JSON.stringify({
    plot: 'A linguist meets aliens.',
    director: 'Denis Villeneuve',
  }),
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
    drives.set([sr0]);
    profiles.set([]);
    logs.set({});
  });

  it('renders empty state when no job', () => {
    const { getByText } = render(JobDetailPanel, { job: undefined });
    expect(getByText(/click a drive or queue row/i)).toBeInTheDocument();
  });

  it('delegates to DiscMetadataPane with the job disc when given a job', () => {
    const { getByText } = render(JobDetailPanel, { job: ripJob });
    expect(getByText('Arrival')).toBeInTheDocument();
    // DiscMetadataPane Overview tab surfaces director from the blob.
    expect(getByText('Denis Villeneuve')).toBeInTheDocument();
    // Tabs row shows Overview / Cast / Files for a DVD.
    expect(getByText('Overview')).toBeInTheDocument();
    expect(getByText('Cast')).toBeInTheDocument();
    expect(getByText('Files')).toBeInTheDocument();
  });
});
