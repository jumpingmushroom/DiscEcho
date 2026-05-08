import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';
import MobileSettings from './MobileSettings.svelte';
import { notifications, settings, drives } from '$lib/store';

const hostResp = {
  hostname: 'mobile-host',
  kernel: '6.12.1',
  cpu_count: 4,
  uptime_seconds: 60,
  disks: [{ path: '/library', total_bytes: 1000, used_bytes: 250, available_bytes: 750 }],
};

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
    json: async () => body,
  } as unknown as Response;
}

describe('MobileSettings', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = typeof input === 'string' ? input : (input as URL).toString();
        if (url === '/api/system/host') return Promise.resolve(jsonResponse(hostResp));
        return Promise.reject(new Error('unexpected ' + url));
      }),
    );
    notifications.set([
      {
        id: 'n-1',
        name: 'tg',
        url: 'tgram://botToken/chatId',
        tags: '',
        triggers: 'done',
        enabled: true,
        created_at: '',
        updated_at: '',
      },
      {
        id: 'n-2',
        name: 'ntfy',
        url: 'ntfy://example.com/topic',
        tags: '',
        triggers: 'done,failed',
        enabled: false,
        created_at: '',
        updated_at: '',
      },
    ]);
    settings.set({
      'library.path': '/srv/library',
      'prefs.accent': 'aurora',
      'prefs.mood': 'void',
      'prefs.density': 'standard',
      'retention.forever': 'true',
      'retention.days': '30',
    });
    drives.set([]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders all sections', () => {
    const { getAllByText } = render(MobileSettings);
    expect(getAllByText(/system/i).length).toBeGreaterThan(0);
    expect(getAllByText(/drives/i).length).toBeGreaterThan(0);
    expect(getAllByText(/host/i).length).toBeGreaterThan(0);
    expect(getAllByText(/notifications/i).length).toBeGreaterThan(0);
    expect(getAllByText(/appearance/i).length).toBeGreaterThan(0);
    expect(getAllByText(/retention/i).length).toBeGreaterThan(0);
  });

  it('shows library path from settings', () => {
    const { container } = render(MobileSettings);
    expect(container.textContent).toContain('/srv/library');
  });

  it('shows drives empty state when no drives', () => {
    const { container } = render(MobileSettings);
    expect(container.textContent).toMatch(/no drives detected/i);
  });

  it('lists drives when present', () => {
    drives.set([
      {
        id: 'd1',
        model: 'ASUS',
        bus: 'usb',
        dev_path: '/dev/sr0',
        state: 'idle',
        last_seen_at: '2026-05-08T00:00:00Z',
      },
    ]);
    const { container } = render(MobileSettings);
    expect(container.textContent).toContain('/dev/sr0');
    expect(container.textContent).toContain('ASUS');
  });

  it('renders host info after fetch resolves', async () => {
    const { container } = render(MobileSettings);
    await waitFor(() => expect(container.textContent).toContain('mobile-host'));
    expect(container.textContent).toContain('4 CPUs');
    expect(container.textContent).toContain('/library');
  });

  it('truncates notification URL to scheme only (hides credentials)', () => {
    const { container } = render(MobileSettings);
    expect(container.textContent).toMatch(/tgram:\/\//);
    expect(container.textContent).not.toContain('botToken');
    expect(container.textContent).not.toContain('chatId');
  });

  it('shows "forever" when retention.forever is true', () => {
    const { container } = render(MobileSettings);
    expect(container.textContent).toMatch(/forever/i);
  });

  it('shows day count when retention.forever is false', () => {
    settings.update((s) => ({ ...s, 'retention.forever': 'false', 'retention.days': '60' }));
    const { container } = render(MobileSettings);
    expect(container.textContent).toMatch(/60.*days/i);
  });

  it('shows "Edit on desktop" footer', () => {
    const { container } = render(MobileSettings);
    expect(container.textContent).toMatch(/edit on desktop/i);
  });
});
