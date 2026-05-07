import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { connectSSE } from './sse';

class FakeEventSource {
  url: string;
  withCredentials: boolean;
  onopen: ((e: Event) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  listeners: Map<string, ((e: MessageEvent) => void)[]> = new Map();
  closed = false;
  static instances: FakeEventSource[] = [];

  constructor(url: string) {
    this.url = url;
    this.withCredentials = false;
    FakeEventSource.instances.push(this);
  }
  addEventListener(event: string, fn: (e: MessageEvent) => void) {
    if (!this.listeners.has(event)) this.listeners.set(event, []);
    this.listeners.get(event)!.push(fn);
  }
  removeEventListener(event: string, fn: (e: MessageEvent) => void) {
    const arr = this.listeners.get(event);
    if (!arr) return;
    const i = arr.indexOf(fn);
    if (i >= 0) arr.splice(i, 1);
  }
  close() {
    this.closed = true;
  }
  fire(event: string, data: unknown) {
    const fns = this.listeners.get(event) ?? [];
    for (const f of fns) f(new MessageEvent(event, { data: JSON.stringify(data) }));
  }
}

describe('connectSSE', () => {
  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal('EventSource', FakeEventSource);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('subscribes to a list of event names and dispatches parsed payloads', () => {
    const got: Array<{ name: string; payload: unknown }> = [];
    const conn = connectSSE('/api/events', ['drive.changed', 'job.progress'], (name, payload) => {
      got.push({ name, payload });
    });

    const es = FakeEventSource.instances[0];
    expect(es.url).toBe('/api/events');

    es.fire('drive.changed', { drive: { id: 'd1' } });
    es.fire('job.progress', { pct: 50 });
    es.fire('disc.detected', { ignored: true }); // not subscribed

    expect(got).toHaveLength(2);
    expect(got[0]).toEqual({ name: 'drive.changed', payload: { drive: { id: 'd1' } } });
    conn.close();
    expect(es.closed).toBe(true);
  });

  it('reports liveStatus transitions via the optional callback', () => {
    const transitions: string[] = [];
    const conn = connectSSE('/api/events', [], () => {}, {
      onStatusChange: (s) => {
        transitions.push(s);
      },
    });
    const es = FakeEventSource.instances[0];

    es.onopen?.(new Event('open'));
    es.onerror?.(new Event('error'));
    es.onopen?.(new Event('open'));

    expect(transitions).toEqual(['live', 'reconnecting', 'live']);
    conn.close();
  });

  it('ignores malformed JSON payloads without crashing', () => {
    const got: unknown[] = [];
    const conn = connectSSE('/api/events', ['x'], (_, payload) => got.push(payload));
    const es = FakeEventSource.instances[0];

    const fns = es.listeners.get('x') ?? [];
    for (const f of fns) f(new MessageEvent('x', { data: 'not-json' }));

    expect(got).toHaveLength(0);
    conn.close();
  });
});
