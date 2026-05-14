import '@testing-library/jest-dom/vitest';
import { describe, it, expect, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ToastHost from './ToastHost.svelte';
import { toasts, pushToast } from '$lib/toasts';

describe('ToastHost', () => {
  beforeEach(() => {
    toasts.set([]);
  });

  it('renders nothing when there are no toasts', () => {
    const { container } = render(ToastHost);
    expect(container.querySelector('[role="status"]')).toBeNull();
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('renders a success toast with the status role', async () => {
    const { container, getByText } = render(ToastHost);
    pushToast('success', 'Profile saved');
    await tick();
    expect(getByText('Profile saved')).toBeInTheDocument();
    expect(container.querySelector('[role="status"]')).not.toBeNull();
  });

  it('renders an error toast with the alert role', async () => {
    const { container, getByText } = render(ToastHost);
    pushToast('error', 'send failed');
    await tick();
    expect(getByText('send failed')).toBeInTheDocument();
    expect(container.querySelector('[role="alert"]')).not.toBeNull();
  });

  it('dismiss button removes the toast', async () => {
    const { getByLabelText, queryByText } = render(ToastHost);
    pushToast('success', 'Profile saved');
    await tick();
    await fireEvent.click(getByLabelText('Dismiss'));
    expect(queryByText('Profile saved')).toBeNull();
  });
});
