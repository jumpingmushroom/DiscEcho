import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { toasts, pushToast, dismissToast } from './toasts';

describe('toasts', () => {
  beforeEach(() => {
    toasts.set([]);
  });

  it('pushToast appends a toast with kind and message', () => {
    pushToast('success', 'Saved');
    const list = get(toasts);
    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({ kind: 'success', message: 'Saved' });
  });

  it('pushToast assigns unique ids', () => {
    pushToast('success', 'a');
    pushToast('error', 'b');
    const list = get(toasts);
    expect(list[0].id).not.toBe(list[1].id);
  });

  it('dismissToast removes the toast by id', () => {
    pushToast('success', 'a');
    const id = get(toasts)[0].id;
    dismissToast(id);
    expect(get(toasts)).toHaveLength(0);
  });

  it('auto-dismisses after the TTL', () => {
    vi.useFakeTimers();
    try {
      pushToast('success', 'a');
      expect(get(toasts)).toHaveLength(1);
      vi.advanceTimersByTime(3500);
      expect(get(toasts)).toHaveLength(0);
    } finally {
      vi.useRealTimers();
    }
  });
});
