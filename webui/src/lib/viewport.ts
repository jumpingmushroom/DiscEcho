import { readable } from 'svelte/store';

// LG_QUERY matches Tailwind's `lg` breakpoint (1024px). Route pages
// use the `isDesktop` store to mount EITHER the mobile or the desktop
// component tree — never both. Mounting both (the old `lg:hidden` /
// `hidden lg:block` CSS-visibility approach) ran every stateful child
// twice: most visibly, a disc's auto-confirm timer fired from both the
// mobile and desktop AwaitingDecisionCard, enqueueing duplicate rips.
const LG_QUERY = '(min-width: 1024px)';

// isDesktop is true when the viewport is at least the `lg` breakpoint.
// The start function runs on first subscription (before the first
// render reads the value), so components see the correct layout on
// first paint with no flash. Defaults to true when matchMedia is
// unavailable — the app is client-only, so that path is test-only.
export const isDesktop = readable(true, (set) => {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return;
  }
  const mq = window.matchMedia(LG_QUERY);
  set(mq.matches);
  const onChange = (e: MediaQueryListEvent): void => set(e.matches);
  mq.addEventListener('change', onChange);
  return () => mq.removeEventListener('change', onChange);
});
