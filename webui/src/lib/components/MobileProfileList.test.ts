import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import MobileProfileList from './MobileProfileList.svelte';
import { profiles } from '$lib/store';
import type { Profile } from '$lib/wire';

const cdProfile: Profile = {
  id: 'p-cd',
  disc_type: 'AUDIO_CD',
  name: 'CD-FLAC',
  engine: 'whipper',
  format: 'FLAC',
  preset: 'AccurateRip',
  container: 'FLAC',
  video_codec: '',
  quality_preset: 'AccurateRip',
  hdr_pipeline: '',
  drive_policy: 'any',
  auto_eject: true,
  options: {},
  output_path_template: '{{.Title}}.flac',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

const dvdProfile: Profile = {
  ...cdProfile,
  id: 'p-dvd',
  disc_type: 'DVD',
  name: 'DVD-Movie',
  engine: 'HandBrake',
  format: 'MP4',
  step_count: 7,
};

describe('MobileProfileList', () => {
  beforeEach(() => {
    profiles.set([]);
  });

  it('renders profiles grouped by disc-type', () => {
    profiles.set([cdProfile, dvdProfile]);
    const { getByText } = render(MobileProfileList);
    expect(getByText('AUDIO_CD')).toBeInTheDocument();
    expect(getByText('DVD')).toBeInTheDocument();
    expect(getByText('CD-FLAC')).toBeInTheDocument();
    expect(getByText('DVD-Movie')).toBeInTheDocument();
  });

  it('shows "Edit profiles on desktop" footer', () => {
    profiles.set([cdProfile]);
    const { getByText } = render(MobileProfileList);
    expect(getByText(/edit profiles on desktop/i)).toBeInTheDocument();
  });

  it('renders empty-state hint when no profiles', () => {
    profiles.set([]);
    const { getByText } = render(MobileProfileList);
    expect(getByText(/no profiles/i)).toBeInTheDocument();
  });
});
