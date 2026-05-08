<script lang="ts" context="module">
  import type { DiscType } from '$lib/wire';

  type Meta = { label: string; color: string };

  export const DISC_TYPE_META: Record<DiscType, Meta> = {
    AUDIO_CD: { label: 'CD', color: '#7eb6ff' },
    DVD: { label: 'DVD', color: '#a78bfa' },
    BDMV: { label: 'Blu-ray', color: '#5ea3ff' },
    UHD: { label: '4K UHD', color: '#f5a524' },
    PSX: { label: 'PSX', color: '#ff7eb6' },
    PS2: { label: 'PS2', color: '#ff7eb6' },
    XBOX: { label: 'Xbox', color: '#7eff9f' },
    SAT: { label: 'Saturn', color: '#ffb37e' },
    DC: { label: 'Dreamcast', color: '#ff9f7e' },
    VCD: { label: 'VCD', color: '#9ca3af' },
    DATA: { label: 'Data', color: '#a1a1aa' },
  };
</script>

<script lang="ts">
  export let type: DiscType;
  export let size: 'sm' | 'md' = 'sm';

  $: meta = DISC_TYPE_META[type] ?? { label: type, color: '#a1a1aa' };
  $: pad = size === 'md' ? 'text-[11px] px-2 py-[3px]' : 'text-[10px] px-1.5 py-[2px]';
</script>

<span
  class="inline-flex items-center rounded border font-semibold uppercase tracking-[0.04em] {pad}"
  style="
    color: {meta.color};
    background: color-mix(in oklab, {meta.color} 12%, transparent);
    border-color: color-mix(in oklab, {meta.color} 28%, transparent);
  "
>
  {meta.label}
</span>
