import '@testing-library/jest-dom/vitest';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import ClearHistoryButton from './ClearHistoryButton.svelte';

describe('ClearHistoryButton', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders nothing when total is 0', () => {
    const { queryByRole } = render(ClearHistoryButton, { total: 0 });
    expect(queryByRole('button')).toBeNull();
  });

  it('first click arms a confirm label, second click clears', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ deleted: 3 }),
    });
    const { getByRole, component } = render(ClearHistoryButton, { total: 3 });

    const cleared = vi.fn();
    component.$on('cleared', cleared);

    const btn = getByRole('button');
    expect(btn).toHaveTextContent('Clear history');

    await fireEvent.click(btn);
    expect(btn).toHaveTextContent('Confirm — clear 3 rips?');
    expect(fetchSpy).not.toHaveBeenCalled();

    await fireEvent.click(btn);
    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/history/clear',
        expect.objectContaining({ method: 'POST' }),
      );
      expect(cleared).toHaveBeenCalledOnce();
    });
  });

  it('shows an error and disarms when the clear fails', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => 'boom',
    });
    const { getByRole, findByText } = render(ClearHistoryButton, { total: 2 });

    const btn = getByRole('button');
    await fireEvent.click(btn); // arm
    await fireEvent.click(btn); // attempt clear → fails

    expect(await findByText(/500/)).toBeInTheDocument();
    expect(btn).toHaveTextContent('Clear history'); // disarmed
  });
});
