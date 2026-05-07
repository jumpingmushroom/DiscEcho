import { writable } from 'svelte/store';
import { registerSW } from 'virtual:pwa-register';

export const updateAvailable = writable(false);

let updater: ((reload?: boolean) => Promise<void>) | undefined;
let intervalId: ReturnType<typeof setInterval> | undefined;

const PERIODIC_CHECK_MS = 60 * 60 * 1000;

export function initPWA(): void {
  if (typeof window === 'undefined') return;
  updater = registerSW({
    immediate: true,
    onNeedRefresh: () => updateAvailable.set(true),
    onOfflineReady: () => {
      /* silent */
    },
  });
  intervalId = setInterval(() => {
    void updater?.(false);
  }, PERIODIC_CHECK_MS);
}

export function applyUpdate(): void {
  void updater?.(true);
}

// Test-only: clears the interval and store so vitest cleanup doesn't
// leak timers and tests don't bleed state into each other.
export function _stopPWAForTests(): void {
  if (intervalId !== undefined) {
    clearInterval(intervalId);
    intervalId = undefined;
  }
  updater = undefined;
  updateAvailable.set(false);
}
