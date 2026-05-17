import { describe, expect, it } from 'vitest';
import { formatProgress } from './formatProgress';

describe('formatProgress', () => {
  it('floors null / undefined / NaN to 0%', () => {
    expect(formatProgress(null)).toBe('0%');
    expect(formatProgress(undefined)).toBe('0%');
    expect(formatProgress(NaN)).toBe('0%');
  });

  it('renders sub-1% with one decimal place so the bar does not look stuck', () => {
    expect(formatProgress(0.24)).toBe('0.2%');
    expect(formatProgress(0.5)).toBe('0.5%');
    expect(formatProgress(0.94)).toBe('0.9%');
  });

  it('rounds 1% and above to integer', () => {
    expect(formatProgress(1)).toBe('1%');
    expect(formatProgress(42.7)).toBe('43%');
  });

  it('snaps near-100 to 100% so the final emit is not rendered as 99%', () => {
    expect(formatProgress(99.95)).toBe('100%');
    expect(formatProgress(100)).toBe('100%');
  });
});
