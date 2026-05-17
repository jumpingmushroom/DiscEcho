// Minimal fetch wrappers. No auth header in M1.2; the daemon either
// runs with DISCECHO_AUTH_DISABLED=true or sits behind a reverse proxy
// that handles auth.

async function checkResponse(res: Response): Promise<Response> {
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`HTTP ${res.status}: ${body}`);
  }
  return res;
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path, { method: 'GET' });
  await checkResponse(res);
  return res.json() as Promise<T>;
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const init: RequestInit = {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  await checkResponse(res);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function apiPut<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  await checkResponse(res);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function apiPatch<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  await checkResponse(res);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function apiDelete(path: string): Promise<void> {
  const res = await fetch(path, { method: 'DELETE' });
  await checkResponse(res);
}

export interface ValidationErrors {
  [field: string]: string;
}

const HTTP_422_PREFIX = 'HTTP 422: ';

/**
 * parseValidationErrors recognises the daemon's 422-response wire
 * format and returns the field-error map. Returns null when the input
 * isn't a 422 Error, when the body isn't JSON, or when the JSON isn't
 * a flat string map.
 *
 * Used by the profile editor to surface server-side validation
 * results inline in the form, falling back to a generic toast on null.
 */
export function parseValidationErrors(e: unknown): ValidationErrors | null {
  if (!(e instanceof Error)) return null;
  if (!e.message.startsWith(HTTP_422_PREFIX)) return null;
  const body = e.message.slice(HTTP_422_PREFIX.length);
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    return null;
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
  const out: ValidationErrors = {};
  for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
    if (typeof v !== 'string') return null;
    out[k] = v;
  }
  return out;
}
