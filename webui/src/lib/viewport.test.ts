import { describe, it, expect, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { isDesktop } from './viewport';

// Install a controllable matchMedia. The viewport store reads
// window.matchMedia in its start function (on first subscribe), so
// this must be called before subscribing.
function installMatchMedia(initialMatches: boolean) {
  let handler: ((e: MediaQueryListEvent) => void) | null = null;
  const mql = {
    matches: initialMatches,
    media: '',
    onchange: null,
    addEventListener: (_type: string, h: (e: MediaQueryListEvent) => void) => {
      handler = h;
    },
    removeEventListener: () => {
      handler = null;
    },
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  };
  window.matchMedia = ((q: string) => {
    mql.media = q;
    return mql;
  }) as unknown as typeof window.matchMedia;
  return {
    fireChange(matches: boolean) {
      mql.matches = matches;
      handler?.({ matches } as MediaQueryListEvent);
    },
    hasListener: () => handler !== null,
  };
}

const realMatchMedia = window.matchMedia;
afterEach(() => {
  window.matchMedia = realMatchMedia;
});

describe('isDesktop', () => {
  it('reflects matchMedia at subscribe time — desktop', () => {
    installMatchMedia(true);
    expect(get(isDesktop)).toBe(true);
  });

  it('reflects matchMedia at subscribe time — mobile', () => {
    installMatchMedia(false);
    expect(get(isDesktop)).toBe(false);
  });

  it('updates when the media query changes, and cleans up its listener', () => {
    const mm = installMatchMedia(true);
    let value: boolean | undefined;
    const unsub = isDesktop.subscribe((v) => {
      value = v;
    });
    expect(value).toBe(true);

    mm.fireChange(false);
    expect(value).toBe(false);
    mm.fireChange(true);
    expect(value).toBe(true);

    expect(mm.hasListener()).toBe(true);
    unsub();
    expect(mm.hasListener()).toBe(false);
  });

  it('falls back to desktop when matchMedia is unavailable', () => {
    // @ts-expect-error — simulating an environment with no matchMedia
    delete window.matchMedia;
    expect(get(isDesktop)).toBe(true);
  });
});
