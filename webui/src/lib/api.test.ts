import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiGet, apiPost, apiDelete, apiPut, parseValidationErrors } from './api';

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

describe('parseValidationErrors', () => {
  it('returns null for non-Error inputs', () => {
    expect(parseValidationErrors(null)).toBeNull();
    expect(parseValidationErrors('string error')).toBeNull();
    expect(parseValidationErrors(undefined)).toBeNull();
  });

  it('returns null for non-422 errors', () => {
    expect(parseValidationErrors(new Error('HTTP 500: boom'))).toBeNull();
    expect(parseValidationErrors(new Error('network'))).toBeNull();
  });

  it('parses 422 with valid JSON body into a field map', () => {
    const e = new Error('HTTP 422: {"format":"unknown","options.bitrate":"unknown option"}');
    const got = parseValidationErrors(e);
    expect(got).toEqual({
      format: 'unknown',
      'options.bitrate': 'unknown option',
    });
  });

  it('returns null when 422 body is not valid JSON', () => {
    const e = new Error('HTTP 422: not-json');
    expect(parseValidationErrors(e)).toBeNull();
  });

  it('returns null when 422 body is JSON but not a flat string map', () => {
    const e = new Error('HTTP 422: {"nested":{"x":1}}');
    expect(parseValidationErrors(e)).toBeNull();
  });
});

describe('apiPut', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('PUTs with JSON body and returns parsed response', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ id: 'x' }),
    });
    const got = await apiPut<{ id: string }>('/api/profiles/x', { name: 'X' });
    expect(got).toEqual({ id: 'x' });
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/profiles/x');
    expect((init as RequestInit).method).toBe('PUT');
    expect((init as RequestInit).body).toBe(JSON.stringify({ name: 'X' }));
  });

  it('returns undefined on 204', async () => {
    fetchSpy.mockResolvedValueOnce({ ok: true, status: 204 });
    const got = await apiPut<undefined>('/api/x', null);
    expect(got).toBeUndefined();
  });

  it('throws on non-OK', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 422,
      text: async () => '{"format":"bad"}',
    });
    await expect(apiPut('/api/x', {})).rejects.toThrow(/HTTP 422/);
  });
});
