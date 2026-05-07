// EventSource wrapper. Subscribes to a fixed list of event names,
// parses JSON payloads, and reports connection status transitions.

export type LiveStatus = 'connecting' | 'live' | 'reconnecting';

export interface SSEOptions {
  onStatusChange?: (status: LiveStatus) => void;
}

export interface SSEConnection {
  close(): void;
}

export function connectSSE(
  url: string,
  eventNames: string[],
  onEvent: (name: string, payload: unknown) => void,
  opts: SSEOptions = {},
): SSEConnection {
  const es = new EventSource(url);
  let status: LiveStatus = 'connecting';
  const setStatus = (s: LiveStatus) => {
    if (s === status) return;
    status = s;
    opts.onStatusChange?.(s);
  };

  const listeners: Array<{ name: string; fn: (e: MessageEvent) => void }> = [];
  for (const name of eventNames) {
    const fn = (e: MessageEvent) => {
      try {
        const payload = JSON.parse(e.data);
        onEvent(name, payload);
      } catch {
        // Drop malformed payloads silently — they're a daemon bug, not the UI's problem.
      }
    };
    es.addEventListener(name, fn);
    listeners.push({ name, fn });
  }

  es.onopen = () => setStatus('live');
  es.onerror = () => setStatus('reconnecting');

  return {
    close() {
      for (const { name, fn } of listeners) es.removeEventListener(name, fn);
      es.close();
    },
  };
}
