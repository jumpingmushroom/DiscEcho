import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import IOSInstallHint from './IOSInstallHint.svelte';

const IPHONE_UA =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1';
const ANDROID_UA =
  'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.230 Mobile Safari/537.36';
const STORAGE_KEY = 'discecho.iosInstallDismissed';

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve();
    await tick();
  }
}

function setUserAgent(ua: string): void {
  Object.defineProperty(navigator, 'userAgent', { value: ua, configurable: true });
}

function setStandalone(value: boolean): void {
  Object.defineProperty(navigator, 'standalone', { value, configurable: true });
}

function makeLocalStorageMock(): Storage {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      for (const key of Object.keys(store)) delete store[key];
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
    get length() {
      return Object.keys(store).length;
    },
  };
}

describe('IOSInstallHint', () => {
  let localStorageMock: Storage;

  beforeEach(() => {
    localStorageMock = makeLocalStorageMock();
    vi.stubGlobal('localStorage', localStorageMock);
    setStandalone(false);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders nothing on Android', async () => {
    setUserAgent(ANDROID_UA);
    const { container } = render(IOSInstallHint);
    await flush();
    expect(container.textContent).not.toMatch(/Add to Home Screen/i);
  });

  it('renders nothing when already standalone-installed', async () => {
    setUserAgent(IPHONE_UA);
    setStandalone(true);
    const { container } = render(IOSInstallHint);
    await flush();
    expect(container.textContent).not.toMatch(/Add to Home Screen/i);
  });

  it('renders nothing when previously dismissed', async () => {
    setUserAgent(IPHONE_UA);
    localStorageMock.setItem(STORAGE_KEY, 'true');
    const { container } = render(IOSInstallHint);
    await flush();
    expect(container.textContent).not.toMatch(/Add to Home Screen/i);
  });

  it('renders the hint on iOS Safari when not installed and not dismissed', async () => {
    setUserAgent(IPHONE_UA);
    const { container } = render(IOSInstallHint);
    await flush();
    expect(container.textContent).toMatch(/Add to Home Screen/i);
  });

  it('Close persists dismissal and hides the hint', async () => {
    setUserAgent(IPHONE_UA);
    const { container, getByText } = render(IOSInstallHint);
    await flush();
    expect(container.textContent).toMatch(/Add to Home Screen/i);
    await fireEvent.click(getByText(/close/i));
    await flush();
    expect(localStorageMock.getItem(STORAGE_KEY)).toBe('true');
    expect(container.textContent).not.toMatch(/Add to Home Screen/i);
  });
});
