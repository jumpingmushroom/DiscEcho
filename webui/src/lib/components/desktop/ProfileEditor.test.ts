import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ProfileEditor from './ProfileEditor.svelte';
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
  options: {},
  output_path_template: '{{.Title}}.flac',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

describe('ProfileEditor', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders empty state when no profile and not creating', () => {
    const { getByText } = render(ProfileEditor, { profile: null, creating: false });
    expect(getByText(/select a profile/i)).toBeInTheDocument();
  });

  it('renders form fields populated from a loaded profile', () => {
    const { getByDisplayValue } = render(ProfileEditor, { profile: seed, creating: false });
    expect(getByDisplayValue('CD-FLAC')).toBeInTheDocument();
    expect(getByDisplayValue('AccurateRip')).toBeInTheDocument();
    expect(getByDisplayValue('{{.Title}}.flac')).toBeInTheDocument();
  });

  it('locks Engine and Disc type in edit mode', () => {
    const { container } = render(ProfileEditor, { profile: seed, creating: false });
    const engine = container.querySelector('[name="engine"]') as HTMLSelectElement;
    const dt = container.querySelector('[name="disc_type"]') as HTMLSelectElement;
    expect(engine.disabled).toBe(true);
    expect(dt.disabled).toBe(true);
  });

  it('Format select limits to engine schema formats', () => {
    const { container } = render(ProfileEditor, { profile: seed, creating: false });
    const format = container.querySelector('[name="format"]') as HTMLSelectElement;
    const opts = Array.from(format.options).map((o) => o.value);
    // whipper engine has only FLAC
    expect(opts).toEqual(['FLAC']);
  });

  it('Save in new mode POSTs and dispatches saved on success', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({ ...seed, id: 'p-new' }),
    });
    const onSaved = vi.fn();
    const { getByText, getByLabelText, component } = render(ProfileEditor, {
      profile: null,
      creating: true,
    });
    component.$on('saved', onSaved);

    await fireEvent.input(getByLabelText(/name/i), { target: { value: 'CD-FLAC-2' } });
    await fireEvent.click(getByText(/create/i));

    // Allow microtasks to drain.
    await flush();
    expect(fetchSpy).toHaveBeenCalled();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/profiles');
    expect((init as RequestInit).method).toBe('POST');
    expect(onSaved).toHaveBeenCalled();
  });

  it('Save with 422 surfaces field errors inline', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 422,
      text: async () => '{"format":"engine whipper requires format in [FLAC], got \\"MP3\\""}',
    });
    const { getByText, container } = render(ProfileEditor, { profile: seed, creating: false });

    await fireEvent.click(getByText(/save changes/i));
    await flush();

    expect(container.textContent).toMatch(/engine whipper requires format/);
  });
});
