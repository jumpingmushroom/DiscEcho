import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ProfileList from './ProfileList.svelte';
import type { Profile } from '$lib/wire';

const cd: Profile = {
  id: 'p-cd',
  disc_type: 'AUDIO_CD',
  name: 'CD-FLAC',
  engine: 'whipper',
  format: 'FLAC',
  preset: '',
  container: 'FLAC',
  video_codec: '',
  quality_preset: '',
  hdr_pipeline: '',
  drive_policy: 'any',
  options: {},
  output_path_template: '',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

const dvd: Profile = { ...cd, id: 'p-dvd', disc_type: 'DVD', name: 'DVD-Movie' };
const dvdDisabled: Profile = { ...dvd, id: 'p-dvd-2', name: 'DVD-Series', enabled: false };

describe('ProfileList', () => {
  it('renders one row per profile with the disc-type badge', () => {
    const { getByText, container } = render(ProfileList, {
      profiles: [cd, dvd],
      selectedID: null,
    });
    expect(getByText('CD-FLAC')).toBeInTheDocument();
    expect(getByText('DVD-Movie')).toBeInTheDocument();
    // Two rows, two DiscTypeBadge spans.
    expect(container.querySelectorAll('button[data-profile-id]').length).toBe(2);
  });

  it('renders the engine sub-line per row', () => {
    const { getByText } = render(ProfileList, { profiles: [cd], selectedID: null });
    expect(getByText('whipper')).toBeInTheDocument();
  });

  it('marks the enabled state with a coloured dot', () => {
    const { container } = render(ProfileList, {
      profiles: [cd, dvdDisabled],
      selectedID: null,
    });
    const enabledRow = container.querySelector('button[data-profile-id="p-cd"]');
    const disabledRow = container.querySelector('button[data-profile-id="p-dvd-2"]');
    expect(enabledRow?.querySelector('[aria-label="enabled"]')).not.toBeNull();
    expect(disabledRow?.querySelector('[aria-label="disabled"]')).not.toBeNull();
  });

  it('marks the selected row with data-selected attribute', () => {
    const { container } = render(ProfileList, {
      profiles: [cd, dvd],
      selectedID: 'p-dvd',
    });
    const selected = container.querySelector('button[data-selected="true"]');
    expect(selected?.getAttribute('data-profile-id')).toBe('p-dvd');
  });

  it('dispatches select with the row id on click', async () => {
    const onSelect = vi.fn();
    const { container, component } = render(ProfileList, {
      profiles: [cd, dvd],
      selectedID: null,
    });
    component.$on('select', (e) => onSelect(e.detail));
    const cdRow = container.querySelector('button[data-profile-id="p-cd"]');
    expect(cdRow).not.toBeNull();
    await fireEvent.click(cdRow!);
    expect(onSelect).toHaveBeenCalledWith('p-cd');
  });

  it('dispatches new event when "+ New" is clicked', async () => {
    const onNew = vi.fn();
    const { getByText, component } = render(ProfileList, {
      profiles: [cd],
      selectedID: null,
    });
    component.$on('new', onNew);
    await fireEvent.click(getByText(/\+ new/i));
    expect(onNew).toHaveBeenCalled();
  });
});
