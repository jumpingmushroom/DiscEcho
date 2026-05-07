<script lang="ts">
  import { onDestroy } from 'svelte';
  import { profiles, pendingDiscID, startDisc } from '$lib/store';
  import type { Disc } from '$lib/wire';
  import BottomSheet from './BottomSheet.svelte';
  import ArtPlaceholder from './ArtPlaceholder.svelte';

  export let disc: Disc;

  const COUNTDOWN_SEC = 8;

  let countdownSec = COUNTDOWN_SEC;
  let cancelled = false;

  $: profileId = $profiles.find((p) => p.disc_type === disc.type && p.enabled)?.id ?? '';

  // The interval is started synchronously during component init so that tests
  // using `vi.useFakeTimers()` can advance time immediately after `render()`
  // without needing an extra microtask flush before timers register.
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
    if (!profileId) return;
    await startDisc(disc.id, profileId, idx);
  }

  async function autoConfirm(): Promise<void> {
    if (cancelled || !profileId) return;
    cancelled = true;
    await startDisc(disc.id, profileId, 0);
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
          Audio CD · {disc.candidates.length} matches
        </div>
        <div class="mt-1 font-mono text-[11px] text-text-3">
          {`Auto-rip in ${countdownSec}s`}
        </div>
      </div>
    </div>

    <div class="space-y-2">
      {#each disc.candidates as c, i}
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
            <div class="truncate text-[14px] font-medium text-text">{c.title}</div>
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
