import { describe, it, expect } from 'vitest';
import { formatBytes } from './format';

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
