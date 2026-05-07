import { describe, it, expect } from 'vitest';
import { formatDuration, relativeTime } from './time';

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
