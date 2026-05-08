<script lang="ts">
  import type { Profile, DiscType } from '$lib/wire';
  import { DISC_TYPES } from '$lib/profile_schema';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import { createEventDispatcher } from 'svelte';

  export let profiles: Profile[];
  export let selectedID: string | null = null;

  const dispatch = createEventDispatcher<{ select: string; new: void }>();

  // Sort grouped by disc-type, but render as a single flat list — the
  // DiscTypeBadge per row already carries the type, so the uppercase
  // group headers from the previous design are dropped.
  $: ordered = orderByDiscType(profiles);

  function dtOf(p: Profile): DiscType {
    return p.disc_type as DiscType;
  }

  function orderByDiscType(arr: Profile[]): Profile[] {
    const rank = new Map<string, number>();
    DISC_TYPES.forEach((dt, i) => rank.set(dt, i));
    return [...arr].sort((a, b) => {
      const ra = rank.get(a.disc_type) ?? 999;
      const rb = rank.get(b.disc_type) ?? 999;
      if (ra !== rb) return ra - rb;
      return a.name.localeCompare(b.name);
    });
  }
</script>

<div class="flex h-full flex-col">
  <div class="flex items-center justify-between px-4 pb-3 pt-5">
    <h1 class="text-[18px] font-bold tracking-tight text-text">Profiles</h1>
    <button
      type="button"
      class="rounded-md border border-border px-2 py-1 text-[11px] font-medium text-accent
             transition-colors hover:bg-surface-2"
      on:click={() => dispatch('new')}
    >
      + New
    </button>
  </div>

  <div class="flex-1 overflow-auto px-2 pb-4">
    {#each ordered as p (p.id)}
      {@const isSelected = p.id === selectedID}
      <button
        type="button"
        data-profile-id={p.id}
        data-selected={isSelected ? 'true' : 'false'}
        class="mb-0.5 flex w-full items-center gap-2.5 rounded-md px-3 py-2.5
               text-left transition-colors hover:bg-surface-2"
        class:selected={isSelected}
        on:click={() => dispatch('select', p.id)}
      >
        <DiscTypeBadge type={dtOf(p)} />
        <div class="min-w-0 flex-1">
          <div class="text-[13px] font-medium text-text" class:opacity-50={!p.enabled}>
            {p.name}
          </div>
          <div class="truncate font-mono text-[11px] text-text-3">{p.engine}</div>
        </div>
        <span
          class="h-1.5 w-1.5 flex-shrink-0 rounded-full"
          class:bg-success={p.enabled}
          class:bg-text-3={!p.enabled}
          aria-label={p.enabled ? 'enabled' : 'disabled'}
        />
      </button>
    {/each}
  </div>
</div>

<style>
  button.selected {
    background: var(--surface-2);
  }
  .bg-success {
    background-color: #00d68f;
  }
  .bg-text-3 {
    background-color: var(--text-3);
  }
</style>
