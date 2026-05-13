import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import StatsRow from './StatsRow.svelte';
import type { Stats } from '$lib/wire';

const sample: Stats = {
  active_jobs: {
    value: 3,
    delta_1h: 1,
    spark_24h: Array(24).fill(2),
  },
  today_ripped: {
    bytes: 82_400_000_000,
    titles: 12,
    spark_7d_bytes: [38e9, 12e9, 0, 45e9, 0, 87e9, 82.4e9],
  },
  library: {
    used_bytes: 14_200_000_000_000,
    total_bytes: 18_000_000_000_000,
    spark_30d_used: Array(30)
      .fill(0)
      .map((_, i) => i * 1e11),
  },
  failures_7d: {
    value: 1,
    previous: 4,
    spark_30d: Array(30).fill(0),
  },
};

describe('StatsRow', () => {
  it('renders the four labels and headline values', () => {
    const { getByText } = render(StatsRow, { stats: sample });
    expect(getByText('ACTIVE JOBS')).toBeInTheDocument();
    expect(getByText('TODAY RIPPED')).toBeInTheDocument();
    expect(getByText('LIBRARY SIZE')).toBeInTheDocument();
    expect(getByText('FAILURES (7D)')).toBeInTheDocument();

    // ACTIVE JOBS headline
    expect(getByText('3')).toBeInTheDocument();
  });

  it('renders +1 delta for ACTIVE JOBS', () => {
    const { getByText } = render(StatsRow, { stats: sample });
    expect(getByText('+1')).toBeInTheDocument();
  });

  it('renders titles subline for TODAY RIPPED', () => {
    const { getByText } = render(StatsRow, { stats: sample });
    expect(getByText(/\+12 titles/)).toBeInTheDocument();
  });

  it('renders prev-window delta for FAILURES', () => {
    const { getByText } = render(StatsRow, { stats: sample });
    expect(getByText(/-3 vs prev/)).toBeInTheDocument();
  });

  it('renders nothing when stats is undefined', () => {
    const { container } = render(StatsRow, { stats: undefined });
    expect(container.querySelector('.grid')).toBeNull();
  });
});
