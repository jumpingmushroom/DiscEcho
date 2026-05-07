import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import UpdateToast from './UpdateToast.svelte';
import { updateAvailable } from '$lib/pwa';

const applyUpdateMock = vi.fn();

vi.mock('$lib/pwa', async () => {
  const actual = await vi.importActual<typeof import('$lib/pwa')>('$lib/pwa');
  return {
    ...actual,
    applyUpdate: () => applyUpdateMock(),
  };
});

describe('UpdateToast', () => {
  beforeEach(() => {
    applyUpdateMock.mockReset();
    updateAvailable.set(false);
  });

  it('renders nothing when no update is available', () => {
    const { container } = render(UpdateToast);
    expect(container.querySelector('button')).toBeNull();
  });

  it('renders the reload button when an update is available', () => {
    updateAvailable.set(true);
    const { getByText } = render(UpdateToast);
    expect(getByText(/reload/i)).toBeInTheDocument();
  });

  it('calls applyUpdate when Reload is clicked', async () => {
    updateAvailable.set(true);
    const { getByText } = render(UpdateToast);
    await fireEvent.click(getByText(/reload/i));
    expect(applyUpdateMock).toHaveBeenCalledOnce();
  });
});
