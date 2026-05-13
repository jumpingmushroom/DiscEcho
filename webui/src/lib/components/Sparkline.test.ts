import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Sparkline from './Sparkline.svelte';

describe('Sparkline', () => {
  it('renders one rect per data point', () => {
    const { container } = render(Sparkline, { data: [1, 2, 3, 4, 5] });
    expect(container.querySelectorAll('rect').length).toBe(5);
  });

  it('renders nothing when data is empty', () => {
    const { container } = render(Sparkline, { data: [] });
    expect(container.querySelector('rect')).toBeNull();
  });

  it('scales bars relative to series max', () => {
    const { container } = render(Sparkline, { data: [1, 5, 2], height: 24 });
    const heights = Array.from(container.querySelectorAll('rect')).map((r) =>
      Number(r.getAttribute('height')),
    );
    expect(Math.max(...heights)).toBeGreaterThan(Math.min(...heights));
  });
});
