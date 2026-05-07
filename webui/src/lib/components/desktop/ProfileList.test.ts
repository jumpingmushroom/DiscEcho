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
  options: {},
  output_path_template: '',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

const dvd: Profile = { ...cd, id: 'p-dvd', disc_type: 'DVD', name: 'DVD-Movie' };

describe('ProfileList', () => {
  it('renders profiles grouped by disc-type', () => {
    const { getByText } = render(ProfileList, { profiles: [cd, dvd], selectedID: null });
    expect(getByText('AUDIO_CD')).toBeInTheDocument();
    expect(getByText('DVD')).toBeInTheDocument();
    expect(getByText('CD-FLAC')).toBeInTheDocument();
    expect(getByText('DVD-Movie')).toBeInTheDocument();
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

  it('dispatches new event when "+ New profile" is clicked', async () => {
    const onNew = vi.fn();
    const { getByText, component } = render(ProfileList, {
      profiles: [cd],
      selectedID: null,
    });
    component.$on('new', onNew);
    await fireEvent.click(getByText(/new profile/i));
    expect(onNew).toHaveBeenCalled();
  });
});
