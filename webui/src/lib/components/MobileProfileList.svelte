<script lang="ts">
  import { profiles } from '$lib/store';
  import { DISC_TYPES } from '$lib/profile_schema';
  import AppBar from './AppBar.svelte';
  import type { Profile } from '$lib/wire';

  $: groups = groupByDiscType($profiles);

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

<div class="min-h-screen px-5 pb-12">
  <AppBar title="Profiles" />

  {#if $profiles.length === 0}
    <div class="mt-12 text-center text-[13px] text-text-3">No profiles yet.</div>
  {:else}
    <div class="mt-4 space-y-5">
      {#each groups as [discType, ps] (discType)}
        <div>
          <div class="mb-2 text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">
            {discType}
          </div>
          <div class="space-y-2">
            {#each ps as p (p.id)}
              <div class="rounded-xl border border-border bg-surface-1 p-3">
                <div class="flex items-center justify-between">
                  <div class="text-[14px] font-medium text-text">{p.name}</div>
                  {#if !p.enabled}
                    <span
                      class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
                      style="background: var(--surface-2); color: var(--text-3)"
                    >
                      DISABLED
                    </span>
                  {/if}
                </div>
                <div class="mt-1 font-mono text-[11px] text-text-3">
                  {p.engine} · {p.format} · {p.step_count} steps
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <div class="mt-10 text-center text-[12px] text-text-3">Edit profiles on desktop.</div>
</div>
