import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import HistoryRow from './HistoryRow.svelte';
import type { HistoryRow as HRow } from '$lib/wire';

const baseRow: HRow = {
  job: {
    id: 'job-1',
    disc_id: 'disc-1',
    profile_id: 'p1',
    state: 'done',
    progress: 100,
    finished_at: '2026-05-07T12:34:00Z',
    created_at: '2026-05-07T12:00:00Z',
  },
  disc: {
    id: 'disc-1',
    type: 'AUDIO_CD',
    title: 'Kind of Blue',
    year: 1959,
    candidates: [],
    created_at: '2026-05-07T12:00:00Z',
  },
};

describe('HistoryRow', () => {
  it('renders title and disc-type label', () => {
    const { getByText } = render(HistoryRow, { row: baseRow });
    expect(getByText('Kind of Blue')).toBeInTheDocument();
    expect(getByText('CD')).toBeInTheDocument();
  });

  it('renders Unknown when title missing', () => {
    const row: HRow = {
      ...baseRow,
      disc: { ...baseRow.disc, title: '' },
    };
    const { getByText } = render(HistoryRow, { row });
    expect(getByText('Unknown')).toBeInTheDocument();
  });

  it('shows FAILED badge for failed state', () => {
    const row: HRow = {
      ...baseRow,
      job: { ...baseRow.job, state: 'failed' },
    };
    const { getByText } = render(HistoryRow, { row });
    expect(getByText('FAILED')).toBeInTheDocument();
  });

  it('shows CANCELLED badge for cancelled state', () => {
    const row: HRow = {
      ...baseRow,
      job: { ...baseRow.job, state: 'cancelled' },
    };
    const { getByText } = render(HistoryRow, { row });
    expect(getByText('CANCELLED')).toBeInTheDocument();
  });

  it('does not show state badge for done', () => {
    const { queryByText } = render(HistoryRow, { row: baseRow });
    expect(queryByText('FAILED')).toBeNull();
    expect(queryByText('CANCELLED')).toBeNull();
  });

  it('dispatches click event when tapped', async () => {
    const onClick = vi.fn();
    const { container, component } = render(HistoryRow, { row: baseRow });
    component.$on('click', onClick);
    await fireEvent.click(container.querySelector('button')!);
    expect(onClick).toHaveBeenCalled();
  });
});
