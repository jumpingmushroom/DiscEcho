import { describe, expect, it } from 'vitest';
import type { Job } from '$lib/wire';
import { lastDoneJobForDisc } from './lastDoneJobForDisc';

const j = (overrides: Partial<Job>): Job => ({
  id: 'j',
  disc_id: 'disc-1',
  drive_id: 'drv-1',
  profile_id: 'p1',
  state: 'done',
  active_step: 'eject',
  progress: 100,
  output_bytes: 0,
  created_at: '2026-05-15T10:00:00Z',
  started_at: '2026-05-15T10:00:01Z',
  finished_at: '2026-05-15T10:30:00Z',
  steps: [],
  ...overrides,
});

describe('lastDoneJobForDisc', () => {
  it('returns undefined when discID is undefined', () => {
    expect(lastDoneJobForDisc([j({})], undefined)).toBeUndefined();
  });

  it('returns undefined when no done job exists for the disc', () => {
    expect(
      lastDoneJobForDisc(
        [j({ id: 'a', state: 'failed' }), j({ id: 'b', state: 'cancelled' })],
        'disc-1',
      ),
    ).toBeUndefined();
  });

  it('returns the most recently finished done job', () => {
    const got = lastDoneJobForDisc(
      [
        j({ id: 'old', finished_at: '2026-05-10T10:00:00Z' }),
        j({ id: 'new', finished_at: '2026-05-15T10:00:00Z' }),
        j({ id: 'mid', finished_at: '2026-05-12T10:00:00Z' }),
      ],
      'disc-1',
    );
    expect(got?.id).toBe('new');
  });

  it('ignores jobs for other discs', () => {
    const got = lastDoneJobForDisc([j({ id: 'other', disc_id: 'disc-2' })], 'disc-1');
    expect(got).toBeUndefined();
  });
});
