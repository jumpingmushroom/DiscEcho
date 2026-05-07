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

export async function apiDelete(path: string): Promise<void> {
  const res = await fetch(path, { method: 'DELETE' });
  await checkResponse(res);
}
