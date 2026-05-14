import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import SystemSection from './SystemSection.svelte';
import { settings, drives } from '$lib/store';
import { toasts } from '$lib/toasts';

const versionResp = { version: '1.0.0', commit: 'abc', build_date: '2026-05-08' };
const hostResp = {
  hostname: 'discecho-host',
  kernel: '6.12.1',
  cpu_count: 8,
  uptime_seconds: 7200,
  disks: [{ path: '/library/movies', total_bytes: 1000, used_bytes: 250, available_bytes: 750 }],
};
const integrationsResp = {
  tmdb: { configured: true, language: 'en-US' },
  musicbrainz: { base_url: 'https://musicbrainz.org', user_agent: 'DiscEcho/test' },
  apprise: { bin: 'apprise', version: '1.7.0' },
  library_roots: {
    movies: '/library/movies',
    tv: '/library/tv',
    music: '/library/music',
    games: '/library/games',
    data: '/library/data',
  },
  items: [
    {
      name: 'TMDB',
      hint: 'movie & TV metadata',
      status: 'connected',
      detail: 'en-US',
      editable: 'DISCECHO_TMDB_KEY',
    },
    {
      name: 'MusicBrainz',
      hint: 'audio CD metadata + AccurateRip',
      status: 'connected',
      detail: 'https://musicbrainz.org',
    },
    {
      name: 'redump',
      hint: 'game disc fingerprints',
      status: 'connected',
      editable: 'DISCECHO_REDUMPER_BIN',
    },
    { name: 'Apprise', hint: 'notification dispatch', status: 'no URLs configured' },
  ],
};

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
    json: async () => body,
  } as unknown as Response;
}

function mockEndpoints(overrides: Record<string, unknown> = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : (input as URL).toString();
      const method = init?.method ?? 'GET';
      if (method === 'GET' && url === '/api/version')
        return Promise.resolve(jsonResponse(overrides.version ?? versionResp));
      if (method === 'GET' && url === '/api/system/host')
        return Promise.resolve(jsonResponse(overrides.host ?? hostResp));
      if (method === 'GET' && url === '/api/system/integrations')
        return Promise.resolve(jsonResponse(overrides.integrations ?? integrationsResp));
      if (method === 'PUT' && url === '/api/settings') {
        apiPutMock(url, init?.body ? JSON.parse(init.body as string) : null);
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      return Promise.reject(new Error('unexpected ' + method + ' ' + url));
    }),
  );
}

const apiPutMock = vi.fn();

describe('SystemSection', () => {
  beforeEach(() => {
    apiPutMock.mockReset();
    mockEndpoints();
    toasts.set([]);
    settings.set({});
    drives.set([
      {
        id: 'd1',
        model: 'ASUS BW-16D1HT',
        bus: 'usb',
        dev_path: '/dev/sr0',
        state: 'idle',
        last_seen_at: '2026-05-08T00:00:00Z',
      },
    ]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders four subsections after data loads', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('discecho-host'));
    expect(container.textContent).toContain('Library paths');
    expect(container.textContent).toContain('Drives');
    expect(container.textContent).toContain('API keys & connections');
    expect(container.textContent).toContain('Host');
  });

  it('renders one ApiRow per integration item', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('TMDB'));
    expect(container.textContent).toContain('movie & TV metadata');
    expect(container.textContent).toContain('audio CD metadata');
    expect(container.textContent).toContain('redump');
    expect(container.textContent).toContain('Apprise');
    // Connected pill rendered for the three connected rows.
    const connectedBadges = container.querySelectorAll('span.text-accent');
    expect(connectedBadges.length).toBeGreaterThanOrEqual(3);
  });

  it('renders one PathField row per typed library root', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('/library/movies'));
    for (const label of ['Movies', 'TV', 'Music', 'Games', 'Data archive']) {
      expect(container.textContent).toContain(label);
    }
    for (const path of [
      '/library/movies',
      '/library/tv',
      '/library/music',
      '/library/games',
      '/library/data',
    ]) {
      expect(container.textContent).toContain(path);
    }
  });

  it('saves edited library roots via apiPut with typed keys', async () => {
    const { container, getByText } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('/library/movies'));
    // Click the first PathField (Movies) to enter edit mode.
    const moviesButton = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent && b.textContent.includes('/library/movies'),
    ) as HTMLButtonElement;
    expect(moviesButton).toBeTruthy();
    await fireEvent.click(moviesButton);
    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    expect(input).toBeTruthy();
    input.value = '/srv/films';
    await fireEvent.input(input);
    await fireEvent.blur(input);
    // Save changes button appears once a row is dirty.
    await waitFor(() => expect(getByText('Save changes')).toBeInTheDocument());
    await fireEvent.click(getByText('Save changes'));
    await waitFor(() =>
      expect(apiPutMock).toHaveBeenCalledWith('/api/settings', {
        'library.movies': '/srv/films',
      }),
    );
    await waitFor(() =>
      expect(get(toasts)).toContainEqual(
        expect.objectContaining({ kind: 'success', message: 'Library paths saved' }),
      ),
    );
  });

  it('shows the drive row from the store', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('ASUS BW-16D1HT'));
    expect(container.textContent).toContain('/dev/sr0');
    expect(container.textContent).toContain('idle');
  });

  it('shows TMDB connected badge with language detail', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('connected'));
    expect(container.textContent).toContain('en-US');
  });

  it('shows TMDB not-configured row when key is missing', async () => {
    mockEndpoints({
      integrations: {
        tmdb: { configured: false, language: 'en-US' },
        musicbrainz: integrationsResp.musicbrainz,
        apprise: integrationsResp.apprise,
        library_roots: integrationsResp.library_roots,
        items: [
          {
            name: 'TMDB',
            hint: 'movie & TV metadata',
            status: 'not configured',
            editable: 'DISCECHO_TMDB_KEY',
          },
          { name: 'MusicBrainz', status: 'connected' },
          { name: 'redump', status: 'connected' },
          { name: 'Apprise', status: 'no URLs configured' },
        ],
      },
    });
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toMatch(/not configured/i));
  });

  it('renders disk usage bar for each disk in host info', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('/library/movies'));
    expect(container.textContent).toMatch(/free/);
  });

  it('renders empty state when no drives', async () => {
    drives.set([]);
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('No drives detected'));
  });
});
