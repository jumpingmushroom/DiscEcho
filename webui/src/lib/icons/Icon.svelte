<script lang="ts" context="module">
  // Stroke-based icon set, Lucide-style geometry. Ported from the
  // DiscEcho design handoff (icons.jsx).
  //
  // Each entry is a string of SVG inner content (paths, circles, rects)
  // that the wrapper renders inside a 24×24 viewBox. Some icons need
  // filled glyphs (e.g. dots in IconMore); those use `fill="currentColor"`
  // attributes inline.
  export type IconName =
    | 'disc'
    | 'home'
    | 'history'
    | 'settings'
    | 'activity'
    | 'eject'
    | 'pause'
    | 'play'
    | 'x'
    | 'check'
    | 'chevron-right'
    | 'chevron-down'
    | 'chevron-left'
    | 'alert'
    | 'search'
    | 'filter'
    | 'bell'
    | 'cpu'
    | 'folder'
    | 'key'
    | 'arrow-down'
    | 'plus'
    | 'more'
    | 'keyboard'
    | 'cloud'
    | 'wand'
    | 'shield'
    | 'ripple';

  export const ICON_PATHS: Record<IconName, string> = {
    disc: '<circle cx="12" cy="12" r="9.5"/><circle cx="12" cy="12" r="2.2"/><path d="M12 2.5a9.5 9.5 0 0 1 9.5 9.5"/>',
    home: '<path d="M3 11l9-7.5L21 11v9.5a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/><path d="M9.5 21.5V14h5v7.5"/>',
    history: '<path d="M3 12a9 9 0 1 0 3-6.7"/><path d="M3 4v4h4"/><path d="M12 7v5l3 2"/>',
    settings:
      '<circle cx="12" cy="12" r="2.6"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.9 2.9l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1A2 2 0 1 1 4 16.9l.1-.1A1.7 1.7 0 0 0 4.4 15a1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1A1.7 1.7 0 0 0 4.3 7l-.1-.1a2 2 0 1 1 2.9-2.9l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.9 2.9l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
    activity: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
    eject: '<path d="M5 14l7-8 7 8z"/><path d="M5 18.5h14"/>',
    pause:
      '<rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>',
    play: '<path d="M6 4l14 8-14 8z"/>',
    x: '<path d="M6 6l12 12M18 6L6 18"/>',
    check: '<path d="M4 12.5L9.5 18 20 6.5"/>',
    'chevron-right': '<path d="M9 6l6 6-6 6"/>',
    'chevron-down': '<path d="M6 9l6 6 6-6"/>',
    'chevron-left': '<path d="M15 6l-6 6 6 6"/>',
    alert:
      '<path d="M12 3l10 18H2z"/><path d="M12 10v5"/><circle cx="12" cy="18" r="0.5" fill="currentColor"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="M16 16l5 5"/>',
    filter: '<path d="M3 5h18l-7 9v6l-4-2v-4z"/>',
    bell: '<path d="M6 9a6 6 0 0 1 12 0c0 6 2.5 7 2.5 7H3.5S6 15 6 9z"/><path d="M10 20a2 2 0 0 0 4 0"/>',
    cpu: '<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3"/>',
    folder:
      '<path d="M3 6a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2v9.5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    key: '<circle cx="8" cy="15" r="3.5"/><path d="M11 13l9-9M16 7l3 3M14 9l3 3"/>',
    'arrow-down': '<path d="M12 4v16M5 13l7 7 7-7"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    more: '<circle cx="5" cy="12" r="1.2" fill="currentColor"/><circle cx="12" cy="12" r="1.2" fill="currentColor"/><circle cx="19" cy="12" r="1.2" fill="currentColor"/>',
    keyboard:
      '<rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M7 14h10"/>',
    cloud: '<path d="M7 18a5 5 0 0 1 0-10 6 6 0 0 1 11.5 2A4.5 4.5 0 0 1 17 18z"/>',
    wand: '<path d="M15 4l5 5L9 20H4v-5z"/><path d="M14 5l5 5"/>',
    shield: '<path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/>',
    ripple:
      '<circle cx="12" cy="12" r="2"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="10"/>',
  };
</script>

<script lang="ts">
  import type { IconName as N } from './Icon.svelte';

  export let name: N;
  export let size: number = 16;
  export let stroke: string = 'currentColor';
  export let strokeWidth: number = 1.75;
  export let fill: string = 'none';
  /** Optional aria-label; when omitted the icon is decorative (aria-hidden). */
  export let label: string | undefined = undefined;
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  {fill}
  {stroke}
  stroke-width={strokeWidth}
  stroke-linecap="round"
  stroke-linejoin="round"
  style="flex-shrink: 0"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}>{@html ICON_PATHS[name]}</svg
>
