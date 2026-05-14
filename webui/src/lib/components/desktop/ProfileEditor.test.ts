import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import ProfileEditor from './ProfileEditor.svelte';
import { toasts } from '$lib/toasts';
import type { Profile } from '$lib/wire';

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve();
    await tick();
  }
}

const seed: Profile = {
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

const bd: Profile = {
  ...seed,
  id: 'p-bd',
  disc_type: 'BDMV',
  name: 'BD-1080p',
  engine: 'MakeMKV+HandBrake',
  format: 'MKV',
  container: 'MKV',
  video_codec: 'x265',
  quality_preset: 'x265 RF 19 10-bit',
  hdr_pipeline: 'passthrough',
  drive_policy: 'any',
  auto_eject: true,
  output_path_template: '/library/movies/{{.Title}} ({{.Year}}).mkv',
  step_count: 7,
};

describe('ProfileEditor', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    toasts.set([]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders empty state when no profile and not creating', () => {
    const { getByText } = render(ProfileEditor, { profile: null, creating: false });
    expect(getByText(/select a profile/i)).toBeInTheDocument();
  });

  it('renders form fields populated from a loaded profile', () => {
    const { getByDisplayValue, getByText } = render(ProfileEditor, {
      profile: seed,
      creating: false,
    });
    expect(getByDisplayValue('CD-FLAC')).toBeInTheDocument();
    expect(getByDisplayValue('AccurateRip')).toBeInTheDocument();
    expect(getByText('{{.Title}}.flac')).toBeInTheDocument();
  });

  it('renders the four mockup FormSections in edit mode', () => {
    const { getByText } = render(ProfileEditor, { profile: bd, creating: false });
    expect(getByText('Engine')).toBeInTheDocument();
    expect(getByText('Encoding')).toBeInTheDocument();
    expect(getByText('Post-processing')).toBeInTheDocument();
    expect(getByText('Library')).toBeInTheDocument();
  });

  it('locks Engine in edit mode', () => {
    const { container } = render(ProfileEditor, { profile: seed, creating: false });
    const engine = container.querySelector('[name="engine"]') as HTMLSelectElement;
    expect(engine.disabled).toBe(true);
  });

  it('Container select limits to engine schema containers', () => {
    const { container } = render(ProfileEditor, { profile: seed, creating: false });
    const containerSel = container.querySelector('[name="container"]') as HTMLSelectElement;
    const opts = Array.from(containerSel.options).map((o) => o.value);
    // whipper engine has only FLAC
    expect(opts).toEqual(['FLAC']);
  });

  it('exposes Video codec + HDR pipeline for video engines', () => {
    const { container } = render(ProfileEditor, { profile: bd, creating: false });
    const codec = container.querySelector('[name="video_codec"]') as HTMLSelectElement;
    const hdr = container.querySelector('[name="hdr_pipeline"]') as HTMLSelectElement;
    expect(codec).not.toBeNull();
    expect(hdr).not.toBeNull();
    expect(codec.value).toBe('x265');
    expect(hdr.value).toBe('passthrough');
  });

  it('hides Video codec for audio-only engines', () => {
    const { container } = render(ProfileEditor, { profile: seed, creating: false });
    expect(container.querySelector('[name="video_codec"]')).toBeNull();
    expect(container.querySelector('[name="hdr_pipeline"]')).toBeNull();
  });

  it('Save in new mode POSTs and dispatches saved on success', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({ ...seed, id: 'p-new' }),
    });
    const onSaved = vi.fn();
    const { getByRole, getByLabelText, component } = render(ProfileEditor, {
      profile: null,
      creating: true,
    });
    component.$on('saved', onSaved);

    await fireEvent.input(getByLabelText(/name/i), { target: { value: 'CD-FLAC-2' } });
    await fireEvent.click(getByRole('button', { name: /^create$/i }));

    // Allow microtasks to drain.
    await flush();
    expect(fetchSpy).toHaveBeenCalled();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/profiles');
    expect((init as RequestInit).method).toBe('POST');
    expect(onSaved).toHaveBeenCalled();
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'Profile created' }),
    );
  });

  it('Save with 422 surfaces field errors against typed container field', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 422,
      text: async () =>
        '{"container":"engine MakeMKV+HandBrake requires container in [MKV], got \\"MP4\\""}',
    });
    const { getByText, container } = render(ProfileEditor, { profile: bd, creating: false });

    await fireEvent.click(getByText(/save changes/i));
    await flush();

    expect(container.textContent).toMatch(/requires container in/);
  });

  it('Duplicate dispatches a draft profile with empty id and "(copy)" suffix', async () => {
    const onDuplicate = vi.fn();
    const { getByText, component } = render(ProfileEditor, {
      profile: bd,
      creating: false,
    });
    component.$on('duplicate', (e) => onDuplicate(e.detail));

    await fireEvent.click(getByText(/duplicate/i));
    expect(onDuplicate).toHaveBeenCalledTimes(1);
    const draft = onDuplicate.mock.calls[0][0] as Profile;
    expect(draft.id).toBe('');
    expect(draft.name).toBe('BD-1080p (copy)');
    expect(draft.engine).toBe('MakeMKV+HandBrake');
  });
});
