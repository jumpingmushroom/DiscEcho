<script lang="ts">
  import type { Profile } from '$lib/wire';
  import { DISC_TYPES } from '$lib/profile_schema';
  import { createEventDispatcher } from 'svelte';

  export let profiles: Profile[];
  export let selectedID: string | null = null;

  const dispatch = createEventDispatcher<{ select: string; new: void }>();

  $: groups = groupByDiscType(profiles);

  function groupByDiscType(arr: Profile[]): Array<[string, Profile[]]> {
    const map = new Map<string, Profile[]>();
    for (const dt of DISC_TYPES) map.set(dt, []);
    for (const p of arr) {
      if (!map.has(p.disc_type)) map.set(p.disc_type, []);
      map.get(p.disc_type)!.push(p);
    }
    return [...map.entries()].filter(([, ps]) => ps.length > 0);
  }
</script>

<div class="rounded-2xl border border-border bg-surface-1">
  <div class="flex items-center justify-between border-b border-border px-4 py-3">
    <div class="text-[13px] font-semibold text-text">Profiles</div>
    <button
      type="button"
      class="rounded-md px-2 py-1 text-[11px] font-medium text-accent
             transition-colors hover:bg-surface-2"
      on:click={() => dispatch('new')}
    >
      + New profile
    </button>
  </div>

  <div class="p-2">
    {#each groups as [discType, ps] (discType)}
      <div class="mb-3 last:mb-0">
        <div class="px-2 pb-1 text-[10px] font-medium uppercase tracking-[0.14em] text-text-3">
          {discType}
        </div>
        {#each ps as p (p.id)}
          {@const isSelected = p.id === selectedID}
          <button
            type="button"
            data-profile-id={p.id}
            data-selected={isSelected ? 'true' : 'false'}
            class="flex w-full items-center justify-between rounded-md px-2 py-1.5
                   text-left transition-colors hover:bg-surface-2"
            class:selected={isSelected}
            on:click={() => dispatch('select', p.id)}
          >
            <span class="text-[13px] font-medium text-text" class:opacity-50={!p.enabled}>
              {p.name}
            </span>
            {#if !p.enabled}
              <span class="font-mono text-[10px] text-text-3">disabled</span>
            {/if}
          </button>
        {/each}
      </div>
    {/each}
  </div>
</div>

<style>
  button.selected {
    background: var(--surface-2);
    color: var(--accent);
  }
</style>
