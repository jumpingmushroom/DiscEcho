import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import DriveCard from './DriveCard.svelte';
import type { Drive, Disc, Job } from '$lib/wire';

const idleDrive: Drive = {
  id: 'd1',
  model: 'Pioneer BDR-XS07B',
  bus: 'USB · sr0',
  dev_path: '/dev/sr0',
  state: 'idle',
  last_seen_at: '2026-05-07T12:00:00Z',
  read_offset: 0,
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
  progress: 35.0,
  speed: '4x',
  eta_seconds: 120,
  active_step: 'rip',
  created_at: '2026-05-07T12:00:00Z',
};

describe('DriveCard', () => {
  it('renders model and bus', () => {
    const { getByText } = render(DriveCard, { drive: idleDrive });
    expect(getByText('Pioneer BDR-XS07B')).toBeInTheDocument();
    expect(getByText('USB · sr0')).toBeInTheDocument();
  });

  it('renders state label', () => {
    const { getByText } = render(DriveCard, { drive: idleDrive });
    expect(getByText('idle')).toBeInTheDocument();
  });

  it('hides queued pill when queuedCount=0', () => {
    const { queryByText } = render(DriveCard, { drive: idleDrive, queuedCount: 0 });
    expect(queryByText(/queued/)).toBeNull();
  });

  it('hides queued pill when queuedCount prop omitted', () => {
    const { queryByText } = render(DriveCard, { drive: idleDrive });
    expect(queryByText(/queued/)).toBeNull();
  });

  it('renders +1 queued when queuedCount=1', () => {
    const { getByText } = render(DriveCard, { drive: rippingDrive, queuedCount: 1 });
    expect(getByText('+1 queued')).toBeInTheDocument();
  });

  it('renders +2 queued when queuedCount=2', () => {
    const { getByText } = render(DriveCard, { drive: rippingDrive, queuedCount: 2 });
    expect(getByText('+2 queued')).toBeInTheDocument();
  });

  it('still renders progress bar for running job (regression)', () => {
    const { container } = render(DriveCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
      queuedCount: 0,
    });
    expect(container.querySelector('span')).not.toBeNull();
  });

  it('dispatches click when the body button is tapped', async () => {
    const onClick = vi.fn();
    const { container, component } = render(DriveCard, { drive: idleDrive });
    component.$on('click', onClick);
    // The body button is the first <button> child; action buttons come after.
    await fireEvent.click(container.querySelector('button')!);
    expect(onClick).toHaveBeenCalled();
  });

  it('hides Stop when no active job', () => {
    const { queryByTestId } = render(DriveCard, { drive: idleDrive, disc: dvdDisc });
    expect(queryByTestId('drive-stop')).toBeNull();
  });

  it('shows Stop while a rip is active', () => {
    const { queryByTestId } = render(DriveCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
    });
    expect(queryByTestId('drive-stop')).not.toBeNull();
  });

  it('Re-identify hidden without a disc', () => {
    const { queryByTestId } = render(DriveCard, { drive: idleDrive });
    expect(queryByTestId('drive-reidentify')).toBeNull();
  });

  it('Re-identify visible when disc present and drive idle', () => {
    const { queryByTestId } = render(DriveCard, { drive: idleDrive, disc: dvdDisc });
    expect(queryByTestId('drive-reidentify')).not.toBeNull();
  });

  it('Re-identify hidden while ripping', () => {
    const { queryByTestId } = render(DriveCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
    });
    expect(queryByTestId('drive-reidentify')).toBeNull();
  });

  it('Eject hidden while ripping', () => {
    const { queryByTestId } = render(DriveCard, {
      drive: rippingDrive,
      disc: dvdDisc,
      job: ripJob,
    });
    expect(queryByTestId('drive-eject')).toBeNull();
  });

  it('Eject visible when idle', () => {
    const { queryByTestId } = render(DriveCard, { drive: idleDrive });
    expect(queryByTestId('drive-eject')).not.toBeNull();
  });
});
