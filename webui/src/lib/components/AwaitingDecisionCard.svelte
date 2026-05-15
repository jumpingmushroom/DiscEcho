<script lang="ts">
  import { onDestroy } from 'svelte';
  import {
    profiles,
    startDisc,
    discs,
    manualIdentify,
    pendingDiscID,
    skipDisc,
    operationMode,
  } from '$lib/store';
  import type { Disc, Candidate } from '$lib/wire';
  import DiscTypeBadge from './DiscTypeBadge.svelte';
  import DiscArt from './DiscArt.svelte';

  export let disc: Disc;

  const COUNTDOWN_SEC = 8;
  const AUTO_CONFIRM_MIN_CONFIDENCE = 50;

  let countdownSec = COUNTDOWN_SEC;
  let cancelled = false;
  // `starting` becomes true the instant a rip-start request is in
  // flight, whether from a manual click or the auto-confirm timer.
  // It disables both buttons + the timer's startDisc call so two
  // jobs can never enqueue for the same disc from this card.
  let starting = false;

  $: liveDisc = $discs[disc.id] ?? disc;
  $: candidates = liveDisc.candidates ?? [];
  $: topConfidence = candidates[0]?.confidence ?? 0;
  // Auto-confirm only fires in batch mode. Manual mode shows the same
  // candidate list but never starts a rip without an explicit click.
  $: autoConfirmAllowed =
    $operationMode === 'batch' && topConfidence >= AUTO_CONFIRM_MIN_CONFIDENCE;

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

  const timer: ReturnType<typeof setInterval> = setInterval(() => {
    if (cancelled || !autoConfirmAllowed) return;
    countdownSec--;
    if (countdownSec <= 0) {
      clearInterval(timer);
      autoConfirm();
    }
  }, 1000);

  onDestroy(() => clearInterval(timer));

  async function pick(idx: number): Promise<void> {
    if (starting) return;
    cancelled = true;
    clearInterval(timer);
    const c = candidates[idx];
    if (!c) return;
    const profileId = profileForCandidate(c);
    if (!profileId) return;
    starting = true;
    try {
      await startDisc(liveDisc.id, profileId, idx);
    } catch (_e) {
      // Daemon refuses duplicate starts with 409 since 0.4.1; if we
      // hit any error, release the lock so the user can retry from
      // the same card rather than being stuck behind a disabled button.
      starting = false;
    }
  }

  async function autoConfirm(): Promise<void> {
    if (starting || cancelled || !autoConfirmAllowed) return;
    cancelled = true;
    const c = candidates[0];
    if (!c) return;
    const profileId = profileForCandidate(c);
    if (!profileId) return;
    starting = true;
    try {
      await startDisc(liveDisc.id, profileId, 0);
    } catch (_e) {
      starting = false;
    }
  }

  let skipping = false;

  async function skip(): Promise<void> {
    if (skipping) return;
    skipping = true;
    cancelled = true;
    clearInterval(timer);
    // Clearing pendingDiscID hides the legacy bottom-sheet path too.
    pendingDiscID.update((cur) => (cur === liveDisc.id ? null : cur));
    try {
      await skipDisc(liveDisc.id);
      // disc.deleted SSE removes the row from the store; the
      // AwaitingDecisionList rerender drops the card.
    } catch (_err) {
      // Server-side delete failed (e.g. the disc has a job after all).
      // Leave the card visible so the user can retry — no UX-corrupt
      // optimistic delete.
      skipping = false;
    }
  }

  let searching = false;
  let searchQuery = '';
  let searchPending = false;
  let searchError: string | null = null;
  let searchedNoResults = false;

  function openSearch(): void {
    searching = true;
    searchQuery = '';
    searchError = null;
    searchedNoResults = false;
    cancelled = true;
    clearInterval(timer);
  }

  function cancelSearch(): void {
    searching = false;
    searchQuery = '';
    searchError = null;
    searchedNoResults = false;
  }

  async function submitSearch(): Promise<void> {
    const q = searchQuery.trim();
    if (!q) return;
    cancelled = true;
    clearInterval(timer);
    searchPending = true;
    searchError = null;
    searchedNoResults = false;
    try {
      const cands = await manualIdentify(liveDisc.id, q);
      if (cands.length === 0) {
        searchedNoResults = true;
      } else {
        searching = false;
      }
    } catch (e) {
      searchError = (e as Error).message;
    } finally {
      searchPending = false;
    }
  }
</script>

<div
  class="rounded-2xl border p-5"
  style="border-color: rgba(var(--accent-rgb),0.35); background: var(--surface-1)"
  data-disc-id={liveDisc.id}
>
  <div class="mb-4 flex gap-3">
    <DiscArt
      disc={liveDisc}
      size={64}
      ratio={liveDisc.type === 'AUDIO_CD' ? 'square' : 'portrait'}
    />
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-2">
        <DiscTypeBadge type={liveDisc.type} />
        <span class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
          Awaiting decision
        </span>
      </div>
      <div class="mt-1 truncate text-[15px] font-semibold text-text">
        {candidates.length} match{candidates.length === 1 ? '' : 'es'}
      </div>
      {#if !searching}
        {#if autoConfirmAllowed}
          <div class="mt-1 font-mono text-[11px] text-text-3">
            {`Auto-rip in ${countdownSec}s`}
          </div>
        {:else if $operationMode === 'manual' && candidates.length > 0}
          <div class="mt-1 text-[11px] text-text-3">Manual mode · pick a title to rip</div>
        {:else if candidates.length > 0}
          <div class="mt-1 text-[11px] text-warn">No confident match · pick a title or search</div>
        {:else}
          <div class="mt-1 text-[11px] text-warn">No match found · search manually</div>
        {/if}
      {/if}
    </div>
  </div>

  {#if searching}
    <div class="space-y-3">
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Movie or show title…"
        class="min-h-[44px] w-full rounded-xl border border-border bg-surface-2 px-3 text-[15px] text-text"
        on:keydown={(e) => e.key === 'Enter' && submitSearch()}
      />
      {#if searchError}
        <div class="text-[12px] text-error">{searchError}</div>
      {/if}
      {#if searchedNoResults}
        <div class="text-[12px] text-warn">No matches found — try a different title.</div>
      {/if}
      <div class="flex flex-col gap-2 sm:flex-row">
        <button
          class="min-h-[40px] flex-1 rounded-xl bg-accent text-[14px] font-semibold text-black disabled:opacity-50"
          on:click={submitSearch}
          disabled={searchPending || !searchQuery.trim()}
        >
          {searchPending ? 'Searching…' : 'Search TMDB'}
        </button>
        <button class="min-h-[40px] text-[13px] text-text-3" on:click={cancelSearch}>
          Cancel search
        </button>
      </div>
    </div>
  {:else}
    <div class="space-y-2">
      {#each candidates as c, i}
        <button
          class="flex w-full min-h-[44px] items-center gap-3 rounded-xl border p-3 text-left"
          style="
            border-color: {i === 0 ? 'rgba(0,214,143,0.35)' : 'var(--border)'};
            background: {i === 0 ? 'rgba(0,214,143,0.04)' : 'transparent'};
          "
          on:click={() => pick(i)}
          disabled={starting}
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
              {c.source}{c.year ? ` · ${c.year}` : ''}{c.artist ? ` · ${c.artist}` : ''}
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

    <div class="mt-5 flex flex-col gap-2 sm:flex-row">
      <button
        class="min-h-[44px] flex-1 rounded-xl bg-accent text-[14px] font-semibold text-black disabled:opacity-50"
        on:click={() => pick(0)}
        disabled={candidates.length === 0 || starting}
      >
        Use top match · Start rip
      </button>
      <button
        class="min-h-[44px] flex-1 rounded-xl border border-border text-[14px] text-text-2"
        on:click={openSearch}
      >
        Search manually
      </button>
      <button
        class="min-h-[36px] text-[13px] text-text-3 disabled:opacity-50"
        on:click={skip}
        disabled={skipping}
      >
        Skip
      </button>
    </div>
  {/if}
</div>
