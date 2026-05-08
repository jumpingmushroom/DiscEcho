import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import SystemSection from './SystemSection.svelte';
import { settings, drives } from '$lib/store';

const versionResp = { version: '1.0.0', commit: 'abc', build_date: '2026-05-08' };
const hostResp = {
  hostname: 'discecho-host',
  kernel: '6.12.1',
  cpu_count: 8,
  uptime_seconds: 7200,
  disks: [{ path: '/library', total_bytes: 1000, used_bytes: 250, available_bytes: 750 }],
};
const integrationsResp = {
  tmdb: { configured: true, language: 'en-US' },
  musicbrainz: { base_url: 'https://musicbrainz.org', user_agent: 'DiscEcho/test' },
  apprise: { bin: 'apprise', version: '1.7.0' },
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
    settings.set({ 'library.path': '/library' });
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
    expect(container.textContent).toContain('Library');
    expect(container.textContent).toContain('Drives');
    expect(container.textContent).toContain('Connections');
    expect(container.textContent).toContain('Host');
  });

  it('shows the drive row from the store', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('ASUS BW-16D1HT'));
    expect(container.textContent).toContain('/dev/sr0');
    expect(container.textContent).toContain('idle');
  });

  it('shows TMDB configured badge with language', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('configured'));
    expect(container.textContent).toContain('en-US');
  });

  it('shows TMDB unconfigured hint when key is missing', async () => {
    mockEndpoints({
      integrations: {
        tmdb: { configured: false, language: 'en-US' },
        musicbrainz: integrationsResp.musicbrainz,
        apprise: integrationsResp.apprise,
      },
    });
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toMatch(/not configured/i));
    expect(container.textContent).toContain('DISCECHO_TMDB_KEY');
  });

  it('renders disk usage bar for each disk in host info', async () => {
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('/library'));
    expect(container.textContent).toMatch(/free/);
  });

  it('saves library.path via apiPut', async () => {
    const { container, getByText } = render(SystemSection);
    await waitFor(() => expect(container.querySelector('input[type="text"]')).not.toBeNull());
    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    input.value = '/srv/media';
    await fireEvent.input(input);
    await fireEvent.click(getByText('Save'));
    await waitFor(() =>
      expect(apiPutMock).toHaveBeenCalledWith('/api/settings', { 'library.path': '/srv/media' }),
    );
  });

  it('renders empty state when no drives', async () => {
    drives.set([]);
    const { container } = render(SystemSection);
    await waitFor(() => expect(container.textContent).toContain('No drives detected'));
  });
});
