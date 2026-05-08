import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import AppearanceSection from './AppearanceSection.svelte';
import { settings } from '$lib/store';

const updatePrefsMock = vi.fn();

vi.mock('$lib/store', async () => {
  const actual = await vi.importActual<typeof import('$lib/store')>('$lib/store');
  return { ...actual, updatePrefs: (...args: unknown[]) => updatePrefsMock(...args) };
});

describe('AppearanceSection', () => {
  beforeEach(() => {
    updatePrefsMock.mockReset();
    settings.set({
      'prefs.accent': 'aurora',
      'prefs.mood': 'void',
      'prefs.density': 'standard',
    });
  });

  it('renders current values', () => {
    const { container } = render(AppearanceSection);
    const accent = container.querySelector('select[name="accent"]') as HTMLSelectElement;
    expect(accent.value).toBe('aurora');
  });

  it('changing accent calls updatePrefs', async () => {
    const { container } = render(AppearanceSection);
    const accent = container.querySelector('select[name="accent"]') as HTMLSelectElement;
    accent.value = 'amber';
    await fireEvent.change(accent);
    expect(updatePrefsMock).toHaveBeenCalled();
    const arg = updatePrefsMock.mock.calls[0][0];
    expect(arg.accent).toBe('amber');
  });
});
