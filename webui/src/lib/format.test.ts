import { describe, it, expect } from 'vitest';
import { formatBytes, trackSummary } from './format';

describe('formatBytes', () => {
  it('renders in the largest unit ≥ 1', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1_500_000)).toBe('1.4 MB');
  });

  it('handles negative / NaN', () => {
    expect(formatBytes(-1)).toBe('0 B');
    expect(formatBytes(NaN)).toBe('0 B');
  });

  it('renders TB for terabyte-scale numbers', () => {
    const v = formatBytes(14_200_000_000_000);
    expect(v).toMatch(/TB$/);
  });
});

describe('trackSummary', () => {
  it('returns "N tracks · MMm" for sub-hour albums', () => {
    const blob = JSON.stringify({
      tracks: [
        { number: 1, duration_seconds: 357 },
        { number: 2, duration_seconds: 661 },
        { number: 3, duration_seconds: 97 },
      ],
    });
    expect(trackSummary(blob)).toBe('3 tracks · 19m');
  });

  it('renders "1h Mm" when total runtime crosses one hour', () => {
    const blob = JSON.stringify({
      tracks: Array.from({ length: 11 }, () => ({ duration_seconds: 400 })),
    });
    // 11 × 400s = 4400s = 73 min = 1h 13m
    expect(trackSummary(blob)).toBe('11 tracks · 1h 13m');
  });

  it('singularises "1 track"', () => {
    const blob = JSON.stringify({ tracks: [{ duration_seconds: 200 }] });
    expect(trackSummary(blob)).toBe('1 track · 3m');
  });

  it('returns empty on undefined / empty / malformed input', () => {
    expect(trackSummary(undefined)).toBe('');
    expect(trackSummary('')).toBe('');
    expect(trackSummary('{not json')).toBe('');
    expect(trackSummary(JSON.stringify({ tracks: [] }))).toBe('');
    expect(trackSummary(JSON.stringify({ foo: 'bar' }))).toBe('');
  });

  it('drops the runtime suffix when no durations are present', () => {
    const blob = JSON.stringify({
      tracks: [{ number: 1 }, { number: 2 }],
    });
    expect(trackSummary(blob)).toBe('2 tracks');
  });
});
