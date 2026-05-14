import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';
import RetentionSection from './RetentionSection.svelte';
import { settings } from '$lib/store';
import { toasts } from '$lib/toasts';

const updateRetentionMock = vi.fn();

vi.mock('$lib/store', async () => {
  const actual = await vi.importActual<typeof import('$lib/store')>('$lib/store');
  return { ...actual, updateRetention: (...args: unknown[]) => updateRetentionMock(...args) };
});

describe('RetentionSection', () => {
  beforeEach(() => {
    updateRetentionMock.mockReset();
    settings.set({ 'retention.forever': 'true', 'retention.days': '30' });
    toasts.set([]);
  });

  it('toggle off reveals days input', async () => {
    const { container } = render(RetentionSection);
    const toggle = container.querySelector('input[type="checkbox"]') as HTMLInputElement;
    expect(container.querySelector('input[type="number"]')).toBeNull();
    toggle.checked = false;
    await fireEvent.change(toggle);
    expect(container.querySelector('input[type="number"]')).not.toBeNull();
  });

  it('save with days < 1 surfaces inline error and skips PUT', async () => {
    const { container, getByText } = render(RetentionSection);
    const toggle = container.querySelector('input[type="checkbox"]') as HTMLInputElement;
    toggle.checked = false;
    await fireEvent.change(toggle);
    const days = container.querySelector('input[type="number"]') as HTMLInputElement;
    days.value = '0';
    await fireEvent.input(days);
    await fireEvent.click(getByText(/save/i));
    expect(container.textContent).toMatch(/at least 1/i);
    expect(updateRetentionMock).not.toHaveBeenCalled();
  });

  it('save with valid combo PUTs', async () => {
    updateRetentionMock.mockResolvedValueOnce(undefined);
    const { container, getByText } = render(RetentionSection);
    const toggle = container.querySelector('input[type="checkbox"]') as HTMLInputElement;
    toggle.checked = false;
    await fireEvent.change(toggle);
    const days = container.querySelector('input[type="number"]') as HTMLInputElement;
    days.value = '60';
    await fireEvent.input(days);
    await fireEvent.click(getByText(/save/i));
    await Promise.resolve();
    await Promise.resolve();
    expect(updateRetentionMock).toHaveBeenCalledWith({ forever: false, days: 60 });
    expect(get(toasts)).toContainEqual(
      expect.objectContaining({ kind: 'success', message: 'Retention settings saved' }),
    );
  });
});
