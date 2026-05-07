<script lang="ts">
  import { onDestroy } from 'svelte';
  import { profiles, pendingDiscID, startDisc, discs } from '$lib/store';
  import type { Disc, Candidate } from '$lib/wire';
  import BottomSheet from './BottomSheet.svelte';
  import ArtPlaceholder from './ArtPlaceholder.svelte';

  export let disc: Disc;

  const COUNTDOWN_SEC = 8;

  let countdownSec = COUNTDOWN_SEC;
  let cancelled = false;

  // Read disc reactively from the global store so manual-identify updates
  // flow through. The export is a one-shot seed.
  $: liveDisc = $discs[disc.id] ?? disc;
  $: candidates = liveDisc.candidates ?? [];

  function profileForCandidate(c: Candidate): string {
    const enabled = $profiles.filter((p) => p.enabled);
    if (liveDisc.type === 'AUDIO_CD') {
      return enabled.find((p) => p.disc_type === 'AUDIO_CD')?.id ?? '';
    }
    if (liveDisc.type === 'DVD') {
      const wantName = c.media_type === 'tv' ? 'DVD-Series' : 'DVD-Movie';
      return enabled.find((p) => p.disc_type === 'DVD' && p.name === wantName)?.id ?? '';
    }
    return enabled.find((p) => p.disc_type === liveDisc.type)?.id ?? '';
  }

  // The interval is started synchronously during component init so tests
  // using `vi.useFakeTimers()` can advance time immediately after `render()`.
  const timer: ReturnType<typeof setInterval> = setInterval(() => {
    if (cancelled) return;
    countdownSec--;
    if (countdownSec <= 0) {
      clearInterval(timer);
      autoConfirm();
    }
  }, 1000);

  onDestroy(() => clearInterval(timer));

  async function pick(idx: number): Promise<void> {
    cancelled = true;
    clearInterval(timer);
    const c = candidates[idx];
    if (!c) return;
    const profileId = profileForCandidate(c);
    if (!profileId) return;
    await startDisc(liveDisc.id, profileId, idx);
  }

  async function autoConfirm(): Promise<void> {
    if (cancelled) return;
    cancelled = true;
    const c = candidates[0];
    if (!c) return;
    const profileId = profileForCandidate(c);
    if (!profileId) return;
    await startDisc(liveDisc.id, profileId, 0);
  }

  function skip(): void {
    cancelled = true;
    clearInterval(timer);
    pendingDiscID.set(null);
  }
</script>

<BottomSheet open={true} on:close={skip}>
  <div class="px-5 pb-6">
    <div class="mb-4 flex gap-3">
      <ArtPlaceholder label="cover" size={64} ratio="portrait" />
      <div class="flex-1">
        <div class="text-[15px] font-semibold text-text">
          {liveDisc.type === 'AUDIO_CD' ? 'Audio CD' : 'DVD'} · {candidates.length} match{candidates.length ===
          1
            ? ''
            : 'es'}
        </div>
        <div class="mt-1 font-mono text-[11px] text-text-3">
          {`Auto-rip in ${countdownSec}s`}
        </div>
      </div>
    </div>

    <div class="space-y-2">
      {#each candidates as c, i}
        <button
          class="flex w-full min-h-[44px] items-center gap-3 rounded-xl border p-3 text-left"
          style="
            border-color: {i === 0 ? 'rgba(0,214,143,0.35)' : 'var(--border)'};
            background: {i === 0 ? 'rgba(0,214,143,0.04)' : 'transparent'};
          "
          on:click={() => pick(i)}
        >
          <span
            class="relative flex h-5 w-5 items-center justify-center rounded-full border"
            style="border-color: {i === 0 ? 'var(--accent)' : '#3f3f46'}"
          >
            {#if i === 0}<span class="h-2.5 w-2.5 rounded-full bg-accent"></span>{/if}
          </span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              {#if c.media_type === 'movie'}
                <span
                  class="rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-[0.14em]"
                  style="background: rgba(94,163,255,0.15); color: var(--info)"
                >
                  FILM
                </span>
              {:else if c.media_type === 'tv'}
                <span
                  class="rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-[0.14em]"
                  style="background: rgba(94,163,255,0.15); color: var(--info)"
                >
                  TV
                </span>
              {/if}
              <span class="truncate text-[14px] font-medium text-text">{c.title}</span>
            </div>
            <div class="mt-0.5 font-mono text-[11px] text-text-3">
              {c.source}{c.year ? ` · ${c.year}` : ''}
            </div>
          </div>
          <span
            class="font-mono text-[14px] font-semibold"
            style="color: {c.confidence > 90 ? 'var(--accent)' : 'var(--warn)'}"
          >
            {c.confidence}%
          </span>
        </button>
      {/each}
    </div>

    <div class="mt-5 flex flex-col gap-2">
      <button
        class="min-h-[48px] rounded-xl bg-accent text-[15px] font-semibold text-black"
        on:click={() => pick(0)}
      >
        Use top match · Start rip
      </button>
      <button class="min-h-[48px] rounded-xl border border-border text-[15px] text-text-2" disabled>
        Search manually
      </button>
      <button class="min-h-[40px] text-[14px] text-text-3" on:click={skip}>
        Skip identification
      </button>
    </div>
  </div>
</BottomSheet>
