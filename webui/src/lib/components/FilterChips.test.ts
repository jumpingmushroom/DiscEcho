import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import FilterChips from './FilterChips.svelte';

describe('FilterChips', () => {
  it('renders all options including All', () => {
    const { getByText } = render(FilterChips, { active: '' });
    expect(getByText('All')).toBeInTheDocument();
    expect(getByText('Audio CD')).toBeInTheDocument();
    expect(getByText('DVD')).toBeInTheDocument();
  });

  it('marks active chip with .active class', () => {
    const { getByText } = render(FilterChips, { active: 'DVD' });
    const dvd = getByText('DVD').closest('button');
    expect(dvd?.className).toMatch(/active/);
    const all = getByText('All').closest('button');
    expect(all?.className).not.toMatch(/active/);
  });

  it('dispatches change event with the chip id on click', async () => {
    const { getByText, component } = render(FilterChips, { active: '' });
    const events: Array<string> = [];
    component.$on('change', (e) => events.push(String(e.detail)));

    await fireEvent.click(getByText('DVD'));
    await fireEvent.click(getByText('Audio CD'));
    await fireEvent.click(getByText('All'));

    expect(events).toEqual(['DVD', 'AUDIO_CD', '']);
  });
});
