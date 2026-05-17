import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import DriveHeroCard from './DriveHeroCard.svelte';
import { jobs } from '$lib/store';
import type { Drive, Disc, Job } from '$lib/wire';

const idleDrive: Drive = {
  id: 'd1',
  model: 'Pioneer BDR-XS07B',
  bus: 'USB · sr0',
  dev_path: '/dev/sr0',
  state: 'idle',
  last_seen_at: '2026-05-07T12:00:00Z',
};

const rippingDrive: Drive = {
  ...idleDrive,
  state: 'ripping',
  current_disc_id: 'disc-1',
};

const dvdDisc: Disc = {
  id: 'disc-1',
  type: 'DVD',
  title: 'Arrival',
  year: 2016,
  candidates: [],
  created_at: '2026-05-07T12:00:00Z',
};

const ripJob: Job = {
  id: 'job-1',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'running',
  progress: 67.0,
  speed: '4.2x',
  eta_seconds: 134,
  active_step: 'rip',
  created_at: '2026-05-07T12:00:00Z',
};

describe('DriveHeroCard', () => {
  beforeEach(() => {
    jobs.set([]);
  });

  it('renders model and bus', () => {
    const { getByText } = render(DriveHeroCard, { drive: idleDrive });
    expect(getByText('Pioneer BDR-XS07B')).toBeInTheDocument();
    expect(getByText('USB · sr0')).toBeInTheDocument();
  });

  it('renders idle state hint when no disc', () => {
    const { getByText } = render(DriveHeroCard, { drive: idleDrive });
    expect(getByText(/insert a disc/i)).toBeInTheDocument();
  });

  it('renders disc title and percent for an active rip', () => {
    const { getByText } = render(DriveHeroCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
    });
    expect(getByText('Arrival')).toBeInTheDocument();
    expect(getByText('67%')).toBeInTheDocument();
  });

  it('renders +N queued pill when queuedCount > 0', () => {
    const { getByText } = render(DriveHeroCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
      queuedCount: 2,
    });
    expect(getByText('+2 queued')).toBeInTheDocument();
  });

  it('dispatches select with the active job id when clicked', async () => {
    const onSelect = vi.fn();
    const { container, component } = render(DriveHeroCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
    });
    component.$on('select', (e) => onSelect(e.detail));
    await fireEvent.click(container.querySelector('button')!);
    expect(onSelect).toHaveBeenCalledWith('job-1');
  });

  it('dispatches select with null when clicked on an idle drive', async () => {
    const onSelect = vi.fn();
    const { container, component } = render(DriveHeroCard, { drive: idleDrive });
    component.$on('select', (e) => onSelect(e.detail));
    await fireEvent.click(container.querySelector('button')!);
    expect(onSelect).toHaveBeenCalledWith(null);
  });

  it('shows a Re-rip button when the inserted disc has a prior done job', () => {
    jobs.set([
      {
        id: 'old-job',
        disc_id: 'disc-1',
        drive_id: 'd1',
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
    const { getByTestId, getByText } = render(DriveHeroCard, {
      drive: idleDrive,
      disc: { ...dvdDisc, drive_id: 'd1' },
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
    const { getAllByText, getByText } = render(DriveHeroCard, { drive: errorDrive });
    // Both the caption ("Drive error — see logs") and banner heading ("Drive error") render;
    // assert the banner heading specifically by checking for an exact match.
    const headings = getAllByText(/drive error/i);
    expect(headings.length).toBeGreaterThanOrEqual(1);
    expect(getByText(/cd-info: exit status 1/)).toBeInTheDocument();
    expect(getByText(/Kreon firmware/)).toBeInTheDocument();
  });

  it('hides the error banner when last_error is empty', () => {
    const { queryByText } = render(DriveHeroCard, { drive: idleDrive });
    // The idle drive shows no last_error banner (no error message text)
    expect(queryByText(/cd-info/)).toBeNull();
    expect(queryByText(/Kreon firmware/)).toBeNull();
  });

  it('shows the rip sub-step label when active_substep is set', () => {
    const job: Job = { ...ripJob, active_substep: 'REFINE' };
    const { getByTestId } = render(DriveHeroCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job,
    });
    const label = getByTestId('active-step-label');
    expect(label.textContent).toContain('Recovering damaged sectors');
  });

  it('shows default rip label when active_substep is absent', () => {
    const { getByTestId } = render(DriveHeroCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
    });
    const label = getByTestId('active-step-label');
    expect(label.textContent).toContain('Read raw data');
  });
});
