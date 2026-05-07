import { describe, it, expect } from 'vitest';
import { dayGroupLabel, formatDuration, relativeTime } from './time';

describe('formatDuration', () => {
  it('formats minutes:seconds for short durations', () => {
    expect(formatDuration(0)).toBe('0s');
    expect(formatDuration(45)).toBe('45s');
    expect(formatDuration(60)).toBe('1m 0s');
    expect(formatDuration(125)).toBe('2m 5s');
  });

  it('formats hours:minutes:seconds for long durations', () => {
    expect(formatDuration(3600)).toBe('1h 0m 0s');
    expect(formatDuration(3725)).toBe('1h 2m 5s');
  });

  it('handles negative or zero', () => {
    expect(formatDuration(-1)).toBe('0s');
    expect(formatDuration(0)).toBe('0s');
  });
});

describe('relativeTime', () => {
  it('formats "just now" within 5 seconds', () => {
    const now = new Date('2026-05-07T12:00:00Z');
    const t = new Date('2026-05-07T11:59:58Z').toISOString();
    expect(relativeTime(t, now)).toBe('just now');
  });

  it('formats minutes ago', () => {
    const now = new Date('2026-05-07T12:00:00Z');
    const t = new Date('2026-05-07T11:45:00Z').toISOString();
    expect(relativeTime(t, now)).toBe('15m ago');
  });

  it('formats hours ago', () => {
    const now = new Date('2026-05-07T12:00:00Z');
    const t = new Date('2026-05-07T09:00:00Z').toISOString();
    expect(relativeTime(t, now)).toBe('3h ago');
  });

  it('returns absolute date past 24 hours', () => {
    const now = new Date('2026-05-07T12:00:00Z');
    const t = new Date('2026-05-05T10:00:00Z').toISOString();
    expect(relativeTime(t, now)).toMatch(/2026/);
  });

  it('handles invalid input', () => {
    expect(relativeTime('', new Date())).toBe('');
    expect(relativeTime('not-a-date', new Date())).toBe('');
  });
});

describe('dayGroupLabel', () => {
  const fixedNow = new Date('2026-05-07T15:00:00Z');

  it('returns Today for the same calendar day', () => {
    expect(dayGroupLabel('2026-05-07T03:00:00Z', fixedNow)).toBe('Today');
    expect(dayGroupLabel('2026-05-07T14:59:59Z', fixedNow)).toBe('Today');
  });

  it('returns Yesterday for one day earlier', () => {
    expect(dayGroupLabel('2026-05-06T12:00:00Z', fixedNow)).toBe('Yesterday');
  });

  it('returns N days ago between 2 and 6 days', () => {
    expect(dayGroupLabel('2026-05-04T12:00:00Z', fixedNow)).toBe('3 days ago');
    expect(dayGroupLabel('2026-05-01T12:00:00Z', fixedNow)).toBe('6 days ago');
  });

  it('returns 1 week ago / N weeks ago between 7 and 29 days', () => {
    expect(dayGroupLabel('2026-04-29T12:00:00Z', fixedNow)).toBe('1 week ago');
    expect(dayGroupLabel('2026-04-15T12:00:00Z', fixedNow)).toBe('3 weeks ago');
  });

  it('returns absolute date past 30 days', () => {
    expect(dayGroupLabel('2025-12-01T12:00:00Z', fixedNow)).toMatch(/2025/);
  });

  it('returns empty string for empty/invalid input', () => {
    expect(dayGroupLabel('', fixedNow)).toBe('');
    expect(dayGroupLabel('not-a-date', fixedNow)).toBe('');
  });
});
