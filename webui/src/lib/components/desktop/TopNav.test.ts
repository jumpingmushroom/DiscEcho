import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import { liveStatus } from '$lib/store';

vi.mock('$app/stores', async () => {
  const { writable } = await import('svelte/store');
  return { page: writable({ url: new URL('http://localhost/') }) };
});

import { page as pageStore } from '$app/stores';
import type { Writable } from 'svelte/store';
import TopNav from './TopNav.svelte';

const mockPage = pageStore as unknown as Writable<{ url: URL }>;

describe('TopNav', () => {
  beforeEach(() => {
    mockPage.set({ url: new URL('http://localhost/') });
    liveStatus.set('connecting');
  });

  it('renders all four section links', () => {
    const { getByText } = render(TopNav);
    for (const label of ['Dashboard', 'History', 'Profiles', 'Settings']) {
      expect(getByText(label)).toBeInTheDocument();
    }
  });

  it('marks Dashboard as active when pathname is "/"', () => {
    mockPage.set({ url: new URL('http://localhost/') });
    const { getByText } = render(TopNav);
    const dashboard = getByText('Dashboard').closest('a');
    expect(dashboard?.className).toMatch(/active/);
    const history = getByText('History').closest('a');
    expect(history?.className).not.toMatch(/active/);
  });

  it('marks History as active when pathname starts with /history', () => {
    mockPage.set({ url: new URL('http://localhost/history') });
    const { getByText } = render(TopNav);
    expect(getByText('History').closest('a')?.className).toMatch(/active/);
    expect(getByText('Dashboard').closest('a')?.className).not.toMatch(/active/);
  });

  it('renders LIVE label when liveStatus is live, WAIT otherwise', () => {
    liveStatus.set('live');
    const { getByText, unmount } = render(TopNav);
    expect(getByText('LIVE')).toBeInTheDocument();
    unmount();
    liveStatus.set('connecting');
    const r = render(TopNav);
    expect(r.getByText('WAIT')).toBeInTheDocument();
  });
});
