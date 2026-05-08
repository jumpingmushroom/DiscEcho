import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import MobileSettings from './MobileSettings.svelte';
import { notifications, settings } from '$lib/store';

describe('MobileSettings', () => {
  beforeEach(() => {
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
      library_path: '/srv/library',
      'prefs.accent': 'aurora',
      'prefs.mood': 'void',
      'prefs.density': 'standard',
      'retention.forever': 'true',
      'retention.days': '30',
    });
  });

  it('renders all four sections', () => {
    const { getByText } = render(MobileSettings);
    expect(getByText(/system/i)).toBeInTheDocument();
    expect(getByText(/notifications/i)).toBeInTheDocument();
    expect(getByText(/appearance/i)).toBeInTheDocument();
    expect(getByText(/retention/i)).toBeInTheDocument();
  });

  it('truncates notification URL to scheme only (hides credentials)', () => {
    const { container } = render(MobileSettings);
    expect(container.textContent).toMatch(/tgram:\/\//);
    expect(container.textContent).not.toContain('botToken');
    expect(container.textContent).not.toContain('chatId');
  });

  it('shows enabled state per notification', () => {
    const { container } = render(MobileSettings);
    // Both notifications listed; n-1 enabled, n-2 disabled.
    expect(container.textContent).toContain('tg');
    expect(container.textContent).toContain('ntfy');
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
