import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import NotificationEditor from './NotificationEditor.svelte';
import { toasts } from '$lib/toasts';
import type { Notification } from '$lib/wire';

const seed: Notification = {
  id: 'n-1',
  name: 'ntfy-1',
  url: 'ntfy://example/topic',
  tags: '',
  triggers: 'done,failed',
  enabled: true,
  created_at: '2026-05-08T12:00:00Z',
  updated_at: '2026-05-08T12:00:00Z',
};

async function flush(): Promise<void> {
  await tick();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

describe('NotificationEditor', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    toasts.set([]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders fields populated from a loaded notification', () => {
    const { getByDisplayValue } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    expect(getByDisplayValue('ntfy-1')).toBeInTheDocument();
    expect(getByDisplayValue('ntfy://example/topic')).toBeInTheDocument();
  });

  it('Save calls update for an existing row', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ...seed, name: 'renamed' }),
    });
    const { getByLabelText, getByText } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.input(getByLabelText(/name/i), { target: { value: 'renamed' } });
    await fireEvent.click(getByText(/^save$/i));
    await flush();
    expect(fetchSpy).toHaveBeenCalledWith(
      `/api/notifications/${seed.id}`,
      expect.objectContaining({ method: 'PUT' }),
    );
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'Notification saved' }),
    );
  });

  it('Save POSTs in creating mode', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({ ...seed, id: 'n-2', name: 'new', url: 'ntfy://x' }),
    });
    const { getByLabelText, getByText } = render(NotificationEditor, {
      notification: null,
      creating: true,
    });
    await fireEvent.input(getByLabelText(/name/i), { target: { value: 'new' } });
    await fireEvent.input(getByLabelText(/url/i), { target: { value: 'ntfy://x' } });
    await fireEvent.click(getByText(/^create$/i));
    await flush();
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/notifications',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'Notification created' }),
    );
  });

  it('Save with 422 surfaces field error inline', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 422,
      text: async () => '{"url":"apprise dry-run: Could not load URL"}',
    });
    const { getByText, container } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.click(getByText(/^save$/i));
    await flush();
    expect(container.textContent).toMatch(/Could not load URL/);
  });

  it('Validate button disabled while editing', async () => {
    const { getByLabelText, getByText } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.input(getByLabelText(/name/i), { target: { value: 'edited' } });
    await tick();
    expect((getByText(/^validate$/i) as HTMLButtonElement).disabled).toBe(true);
  });

  it('Validate pushes a success toast', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ok: true }),
    });
    const { getByText } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.click(getByText(/^validate$/i));
    await flush();
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'URL is valid.' }),
    );
  });

  it('Validate pushes an error toast with apprise message', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ok: false, error: 'apprise dry-run: bad URL' }),
    });
    const { getByText } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.click(getByText(/^validate$/i));
    await flush();
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'error', message: 'apprise dry-run: bad URL' }),
    );
  });

  it('Test pushes a success toast', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ sent: true }),
    });
    const { getByText } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.click(getByText(/^test$/i));
    await flush();
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'Test notification sent.' }),
    );
  });

  it('Test pushes an error toast on 502', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 502,
      text: async () => '{"sent":false,"error":"delivery failed"}',
    });
    const { getByText } = render(NotificationEditor, {
      notification: seed,
      creating: false,
    });
    await fireEvent.click(getByText(/^test$/i));
    await flush();
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'error', message: 'delivery failed' }),
    );
  });

  it('Delete two-step', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 204,
      text: async () => '',
      json: async () => null,
    });
    const { getByText } = render(NotificationEditor, { notification: seed, creating: false });
    await fireEvent.click(getByText(/^delete$/i));
    expect(getByText(/^confirm delete$/i)).toBeInTheDocument();
    await fireEvent.click(getByText(/^confirm delete$/i));
    await flush();
    expect(fetchSpy).toHaveBeenCalledWith(
      `/api/notifications/${seed.id}`,
      expect.objectContaining({ method: 'DELETE' }),
    );
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'Notification deleted' }),
    );
  });
});
