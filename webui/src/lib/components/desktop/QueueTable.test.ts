import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import QueueTable from './QueueTable.svelte';
import { discs } from '$lib/store';
import type { Job, Disc } from '$lib/wire';

const dvdDisc: Disc = {
  id: 'disc-1',
  type: 'DVD',
  title: 'Arrival',
  year: 2016,
  candidates: [],
  created_at: '2026-05-07T12:00:00Z',
};

const cdDisc: Disc = {
  id: 'disc-2',
  type: 'AUDIO_CD',
  title: 'Kind of Blue',
  candidates: [],
  created_at: '2026-05-07T12:00:00Z',
};

const runningJob: Job = {
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

const queuedJob: Job = {
  id: 'job-2',
  disc_id: 'disc-2',
  drive_id: 'd2',
  profile_id: 'p2',
  state: 'queued',
  progress: 0,
  created_at: '2026-05-07T12:01:00Z',
};

describe('QueueTable', () => {
  beforeEach(() => {
    discs.set({ 'disc-1': dvdDisc, 'disc-2': cdDisc });
  });

  it('renders empty state when no jobs', () => {
    const { getByText } = render(QueueTable, { jobs: [], selectedJobID: null });
    expect(getByText('No active jobs')).toBeInTheDocument();
  });

  it('renders rows with title, drive, percent, ETA cells', () => {
    const { getByText } = render(QueueTable, {
      jobs: [runningJob],
      selectedJobID: null,
    });
    expect(getByText('Arrival')).toBeInTheDocument();
    expect(getByText('d1')).toBeInTheDocument();
    expect(getByText('67%')).toBeInTheDocument();
    expect(getByText('134s')).toBeInTheDocument();
  });

  it('renders QUEUED badge in pct column for queued rows', () => {
    const { getByText, queryByText } = render(QueueTable, {
      jobs: [queuedJob],
      selectedJobID: null,
    });
    expect(getByText('QUEUED')).toBeInTheDocument();
    expect(queryByText('0%')).toBeNull();
  });

  it('dispatches select with the row job id on row click', async () => {
    const onSelect = vi.fn();
    const { container, component } = render(QueueTable, {
      jobs: [runningJob],
      selectedJobID: null,
    });
    component.$on('select', (e) => onSelect(e.detail));
    const row = container.querySelector('tbody tr');
    expect(row).not.toBeNull();
    await fireEvent.click(row!);
    expect(onSelect).toHaveBeenCalledWith('job-1');
  });

  it('marks the selected row with a highlight attribute', () => {
    const { container } = render(QueueTable, {
      jobs: [runningJob, queuedJob],
      selectedJobID: 'job-2',
    });
    const rows = container.querySelectorAll('tbody tr[data-job-id]');
    expect(rows.length).toBe(2);
    const selected = container.querySelector('tbody tr[data-selected="true"]');
    expect(selected?.getAttribute('data-job-id')).toBe('job-2');
  });
});
