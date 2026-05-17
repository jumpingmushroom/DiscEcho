import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';
import AccurateRipBadge from './AccurateRipBadge.svelte';
import type { AccurateRipSummary } from '$lib/wire';

afterEach(() => cleanup());

describe('AccurateRipBadge', () => {
  it('renders the verified state with a checkmark and confidence', () => {
    const summary: AccurateRipSummary = {
      status: 'verified',
      verified_tracks: 12,
      total_tracks: 12,
      min_confidence: 28,
      max_confidence: 87,
    };
    const { getByText, getByTitle } = render(AccurateRipBadge, { props: { summary } });
    expect(getByText(/AccurateRip ✓ 12\/12 · conf 87/)).toBeInTheDocument();
    // Title carries the full explanation for screen readers + hover tooltip.
    expect(getByTitle(/All 12 tracks match AccurateRip checksums\./)).toBeInTheDocument();
  });

  it('renders the unverified state with the verified/total ratio', () => {
    const summary: AccurateRipSummary = {
      status: 'unverified',
      verified_tracks: 8,
      total_tracks: 12,
    };
    const { getByText, getByTitle } = render(AccurateRipBadge, { props: { summary } });
    expect(getByText(/AccurateRip — 8\/12 verified/)).toBeInTheDocument();
    expect(getByTitle(/4 of 12 tracks did not match AccurateRip/)).toBeInTheDocument();
  });

  it('renders the uncalibrated state with a hint pointing at settings', () => {
    const summary: AccurateRipSummary = {
      status: 'uncalibrated',
      verified_tracks: 0,
      total_tracks: 0,
    };
    const { getByText, getByTitle } = render(AccurateRipBadge, { props: { summary } });
    expect(getByText(/AccurateRip skipped — drive uncalibrated/)).toBeInTheDocument();
    expect(getByTitle(/Drive read-offset is not set/)).toBeInTheDocument();
  });

  it('omits the conf suffix when no confidence is known', () => {
    const summary: AccurateRipSummary = {
      status: 'verified',
      verified_tracks: 4,
      total_tracks: 4,
      // max_confidence absent — e.g. modern whipper sentinel of 1 lifted
      // to 1 only; show the verified ratio without trailing confs.
      max_confidence: 0,
    };
    const { getByText } = render(AccurateRipBadge, { props: { summary } });
    // No trailing " · conf X" when the highest seen confidence is 0.
    expect(getByText('AccurateRip ✓ 4/4')).toBeInTheDocument();
  });
});
