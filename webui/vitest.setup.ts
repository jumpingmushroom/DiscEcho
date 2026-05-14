import '@testing-library/jest-dom/vitest';

// jsdom has no matchMedia; the viewport store reads it. Provide a
// no-op stub defaulting to desktop so component tests see the desktop
// layout. Tests that exercise the responsive split install their own
// controllable matchMedia before rendering.
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) =>
    ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList) as typeof window.matchMedia;
}
