<script lang="ts">
  import type { DiscType } from '$lib/wire';
  export let type: DiscType;

  const labels: Record<DiscType, string> = {
    AUDIO_CD: 'CD',
    DVD: 'DVD',
    BDMV: 'BD',
    UHD: 'UHD',
    PSX: 'PSX',
    PS2: 'PS2',
    XBOX: 'XBOX',
    SAT: 'SAT',
    DC: 'DC',
    VCD: 'VCD',
    DATA: 'DATA',
  };
</script>

<span
  class="inline-block rounded border border-border bg-surface-2 px-1.5 py-0.5
         text-[10px] font-bold uppercase tracking-[0.14em] text-text-2"
>
  {labels[type] ?? type}
</span>
