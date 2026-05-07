import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiGet, apiPost, apiDelete } from './api';

describe('api helpers', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('apiGet returns JSON on 200', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ hello: 'world' }),
    });
    const got = await apiGet<{ hello: string }>('/api/state');
    expect(got).toEqual({ hello: 'world' });
    expect(fetchSpy).toHaveBeenCalledWith('/api/state', expect.objectContaining({ method: 'GET' }));
  });

  it('apiGet throws on non-2xx with status code', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => 'oops',
    });
    await expect(apiGet('/api/state')).rejects.toThrow(/500/);
  });

  it('apiPost sends JSON body', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ id: 'job-1' }),
    });
    await apiPost('/api/discs/d1/start', { profile_id: 'p1' });
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/d1/start',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ profile_id: 'p1' }),
      }),
    );
  });

  it('apiDelete returns void on 204', async () => {
    fetchSpy.mockResolvedValueOnce({ ok: true, status: 204 });
    const got = await apiDelete('/api/jobs/j1/cancel');
    expect(got).toBeUndefined();
  });
});
