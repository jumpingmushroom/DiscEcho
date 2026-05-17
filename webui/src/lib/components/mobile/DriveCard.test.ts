import '@testing-library/jest-dom/vitest';
import { render, cleanup } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import DriveCard from './DriveCard.svelte';
import { jobs } from '$lib/store';
import type { Drive, Disc } from '$lib/wire';

const idleDrive: Drive = {
  id: 'drv-1',
  model: 'ASUS SDRW',
  bus: 'USB · sr0',
  dev_path: '/dev/sr0',
  state: 'idle',
  last_seen_at: '2026-05-15T10:00:00Z',
};

const seedDisc = (overrides: Partial<Disc> = {}): Disc => ({
  id: 'disc-1',
  drive_id: 'drv-1',
  type: 'AUDIO_CD',
  title: 'Fear and Bullets',
  year: 1997,
  toc_hash: 'toc',
  candidates: [],
  created_at: '2026-05-16T10:00:00Z',
  ...overrides,
});

describe('mobile/DriveCard', () => {
  beforeEach(() => {
    jobs.set([]);
  });

  afterEach(() => {
    cleanup();
  });

  it('shows a Re-rip button when the inserted disc has a prior done job', () => {
    jobs.set([
      {
        id: 'old-job',
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
      },
    ]);
    const { getByTestId, getByText } = render(DriveCard, {
      drive: idleDrive,
      disc: seedDisc({ id: 'disc-1', drive_id: 'drv-1' }),
    });
    expect(getByTestId('drive-rerip')).toBeTruthy();
    expect(getByText(/Already ripped 2026-05-15 — re-rip\?/)).toBeInTheDocument();
  });

  it('shows the error banner and tip when last_error is set', () => {
    const errorDrive: Drive = {
      ...idleDrive,
      state: 'error',
      last_error: 'cd-info: exit status 1',
      last_error_tip: 'Xbox game discs require a drive with Kreon firmware …',
    };
    const { getByText } = render(DriveCard, { drive: errorDrive });
    expect(getByText(/Drive error/i)).toBeInTheDocument();
    expect(getByText(/cd-info: exit status 1/)).toBeInTheDocument();
    expect(getByText(/Kreon firmware/)).toBeInTheDocument();
  });

  it('hides the error banner when last_error is empty', () => {
    const { queryByText } = render(DriveCard, { drive: idleDrive });
    expect(queryByText(/Drive error/i)).toBeNull();
  });
});
