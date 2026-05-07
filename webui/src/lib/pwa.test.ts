import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

const registerSWMock = vi.fn();
let capturedHandlers: { onNeedRefresh?: () => void; onOfflineReady?: () => void } = {};

vi.mock('virtual:pwa-register', () => ({
  registerSW: (opts: { onNeedRefresh?: () => void; onOfflineReady?: () => void }) => {
    capturedHandlers = opts;
    return registerSWMock;
  },
}));

import { initPWA, applyUpdate, updateAvailable, _stopPWAForTests } from './pwa';

describe('lib/pwa', () => {
  beforeEach(() => {
    registerSWMock.mockReset();
    capturedHandlers = {};
    _stopPWAForTests();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    _stopPWAForTests();
  });

  it('initPWA registers the SW with handlers', () => {
    initPWA();
    expect(capturedHandlers.onNeedRefresh).toBeTypeOf('function');
    expect(capturedHandlers.onOfflineReady).toBeTypeOf('function');
  });

  it('onNeedRefresh sets updateAvailable to true', () => {
    initPWA();
    expect(get(updateAvailable)).toBe(false);
    capturedHandlers.onNeedRefresh?.();
    expect(get(updateAvailable)).toBe(true);
  });

  it('applyUpdate calls the captured updater with reload=true', () => {
    initPWA();
    applyUpdate();
    expect(registerSWMock).toHaveBeenCalledWith(true);
  });

  it('initPWA schedules a 60-minute background update check', () => {
    initPWA();
    registerSWMock.mockClear();
    vi.advanceTimersByTime(60 * 60 * 1000);
    expect(registerSWMock).toHaveBeenCalledWith(false);
  });

  it('_stopPWAForTests clears the interval and resets state', () => {
    initPWA();
    capturedHandlers.onNeedRefresh?.();
    expect(get(updateAvailable)).toBe(true);
    _stopPWAForTests();
    expect(get(updateAvailable)).toBe(false);
    registerSWMock.mockClear();
    vi.advanceTimersByTime(60 * 60 * 1000);
    expect(registerSWMock).not.toHaveBeenCalled();
  });
});
