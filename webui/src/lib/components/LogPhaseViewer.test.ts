import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import LogPhaseViewer from './LogPhaseViewer.svelte';
import { logs, type LogLine } from '$lib/store';

vi.mock('$lib/api', () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  apiDelete: vi.fn(),
}));

import { apiGet } from '$lib/api';

const ripLine = (t: string, msg: string): LogLine => ({
  job_id: 'j1',
  t,
  step: 'rip',
  level: 'info',
  message: msg,
});

const txLine = (t: string, msg: string): LogLine => ({
  job_id: 'j1',
  t,
  step: 'transcode',
  level: 'info',
  message: msg,
});

describe('LogPhaseViewer (live mode)', () => {
  beforeEach(() => {
    logs.set({
      j1: [
        ripLine('2026-05-15T10:00:00Z', 'rip-1'),
        ripLine('2026-05-15T10:00:01Z', 'rip-2'),
        txLine('2026-05-15T10:01:00Z', 'tx-1'),
      ],
    });
  });

  it('only renders chips for steps with lines (plus All)', () => {
    const { getByText, queryByText } = render(LogPhaseViewer, {
      jobID: 'j1',
      live: true,
    });
    expect(getByText('All')).toBeInTheDocument();
    expect(getByText('Rip')).toBeInTheDocument();
    expect(getByText('Transcode')).toBeInTheDocument();
    expect(queryByText('Move')).toBeNull();
    expect(queryByText('Identify')).toBeNull();
  });

  it('filters lines to the selected step when a chip is clicked', async () => {
    const { getByText, queryByText } = render(LogPhaseViewer, {
      jobID: 'j1',
      live: true,
    });
    expect(getByText('rip-1')).toBeInTheDocument();
    expect(getByText('tx-1')).toBeInTheDocument();

    await fireEvent.click(getByText('Rip'));
    expect(getByText('rip-1')).toBeInTheDocument();
    expect(getByText('rip-2')).toBeInTheDocument();
    expect(queryByText('tx-1')).toBeNull();
  });

  it('auto-tracks activeStep until the user picks manually', async () => {
    const { getByText, queryByText, component } = render(LogPhaseViewer, {
      jobID: 'j1',
      live: true,
      activeStep: 'rip',
    });
    // Rip is active → only rip lines visible.
    expect(getByText('rip-1')).toBeInTheDocument();
    expect(queryByText('tx-1')).toBeNull();

    // Active step moves to transcode while user hasn't clicked.
    component.$set({ activeStep: 'transcode' });
    await Promise.resolve();
    expect(queryByText('rip-1')).toBeNull();
    expect(getByText('tx-1')).toBeInTheDocument();

    // User picks Rip manually — stays even when activeStep moves again.
    await fireEvent.click(getByText('Rip'));
    component.$set({ activeStep: 'transcode' });
    await Promise.resolve();
    expect(getByText('rip-1')).toBeInTheDocument();
    expect(queryByText('tx-1')).toBeNull();
  });
});

describe('LogPhaseViewer (terminal mode)', () => {
  beforeEach(() => {
    logs.set({});
    vi.mocked(apiGet).mockReset();
  });

  it('fetches /api/jobs/:id/logs on mount and renders the lines', async () => {
    vi.mocked(apiGet).mockResolvedValue({
      lines: [ripLine('2026-05-15T10:00:00Z', 'rip-old'), txLine('2026-05-15T10:01:00Z', 'tx-old')],
      total: 2,
      limit: 2000,
      offset: 0,
    });
    const { findByText } = render(LogPhaseViewer, {
      jobID: 'j1',
      live: false,
    });
    expect(await findByText('rip-old')).toBeInTheDocument();
    expect(await findByText('tx-old')).toBeInTheDocument();
    expect(apiGet).toHaveBeenCalledWith(expect.stringMatching(/\/api\/jobs\/j1\/logs/));
  });

  it('shows an empty message when the daemon returns no lines', async () => {
    vi.mocked(apiGet).mockResolvedValue({
      lines: [],
      total: 0,
      limit: 2000,
      offset: 0,
    });
    const { findByText } = render(LogPhaseViewer, {
      jobID: 'j1',
      live: false,
    });
    expect(await findByText(/No log lines/i)).toBeInTheDocument();
  });
});
