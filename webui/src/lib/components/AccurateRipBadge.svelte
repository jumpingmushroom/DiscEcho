<script lang="ts">
  import type { AccurateRipSummary } from '$lib/wire';

  export let summary: AccurateRipSummary;

  // Verified: every track in the rip matched AccurateRip with a peer
  // count >= 1. Render green with the conf range when known.
  // Unverified: drive is calibrated but at least one track failed AR.
  // Uncalibrated: drive has no offset set — AR comparison can't be
  // trusted; render grey with a hint pointing at the offset setting.
  $: tone =
    summary.status === 'verified'
      ? 'verified'
      : summary.status === 'unverified'
        ? 'unverified'
        : 'uncalibrated';

  function label(s: AccurateRipSummary): string {
    if (s.status === 'verified') {
      const confSuffix =
        typeof s.max_confidence === 'number' && s.max_confidence >= 1
          ? ` · conf ${s.max_confidence}`
          : '';
      return `AccurateRip ✓ ${s.verified_tracks}/${s.total_tracks}${confSuffix}`;
    }
    if (s.status === 'unverified') {
      return `AccurateRip — ${s.verified_tracks}/${s.total_tracks} verified`;
    }
    return 'AccurateRip skipped — drive uncalibrated';
  }

  function title(s: AccurateRipSummary): string {
    if (s.status === 'verified') {
      return `All ${s.total_tracks} tracks match AccurateRip checksums.`;
    }
    if (s.status === 'unverified') {
      const failed = Math.max(0, s.total_tracks - s.verified_tracks);
      return `${failed} of ${s.total_tracks} tracks did not match AccurateRip. Drive may need re-calibration.`;
    }
    return 'Drive read-offset is not set. AccurateRip can’t verify rips at offset 0. Configure the offset in Settings → System.';
  }
</script>

<span
  class="inline-flex items-center gap-1.5 rounded px-2 py-1 text-[11px] font-medium"
  class:tone-verified={tone === 'verified'}
  class:tone-unverified={tone === 'unverified'}
  class:tone-uncalibrated={tone === 'uncalibrated'}
  data-status={summary.status}
  title={title(summary)}
>
  {label(summary)}
</span>

<style>
  .tone-verified {
    background: rgb(22 101 52 / 0.15);
    color: rgb(134 239 172);
  }
  .tone-unverified {
    background: rgb(180 83 9 / 0.15);
    color: rgb(253 186 116);
  }
  .tone-uncalibrated {
    background: var(--surface-2);
    color: var(--text-3);
  }
</style>
