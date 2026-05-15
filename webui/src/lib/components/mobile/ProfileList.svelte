<script lang="ts">
  import { profiles } from '$lib/store';
  import { DISC_TYPES } from '$lib/profile_schema';
  import AppBar from './AppBar.svelte';
  import TabBar from './TabBar.svelte';
  import Icon from '$lib/icons/Icon.svelte';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';
  import type { DiscType, Profile } from '$lib/wire';

  $: groups = groupByDiscType($profiles);

  function groupByDiscType(arr: Profile[]): Array<[DiscType, Profile[]]> {
    const map = new Map<DiscType, Profile[]>();
    for (const dt of DISC_TYPES) map.set(dt as DiscType, []);
    for (const p of arr) {
      const k = p.disc_type as DiscType;
      if (!map.has(k)) map.set(k, []);
      map.get(k)!.push(p);
    }
    return [...map.entries()].filter(([, ps]) => ps.length > 0);
  }
</script>

<div class="min-h-screen pb-24">
  <AppBar title="Profiles">
    <div slot="right">
      <a
        href="/profiles/new"
        data-sveltekit-preload-data="hover"
        aria-label="New profile"
        class="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-surface-1 text-text-2"
      >
        <Icon name="plus" size={18} />
      </a>
    </div>
  </AppBar>

  {#if $profiles.length === 0}
    <div class="mt-12 px-5 text-center text-text-3" style="font-size: var(--ts-body)">
      No profiles yet. Tap <span class="text-text-2">+</span> to create one.
    </div>
  {:else}
    <div class="mt-4 space-y-5 px-4">
      {#each groups as [discType, ps] (discType)}
        <div>
          <div class="mb-2 flex items-center gap-2">
            <DiscTypeBadge type={discType} />
            <span
              class="font-medium uppercase tracking-[0.14em] text-text-3"
              style="font-size: var(--ts-overline)"
            >
              {ps.length}
            </span>
          </div>
          <div class="space-y-2">
            {#each ps as p (p.id)}
              <a
                href="/profiles/{p.id}"
                data-sveltekit-preload-data="hover"
                class="block min-h-[44px] rounded-xl border border-border bg-surface-1 p-3 transition-colors hover:border-border-strong"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="min-w-0 flex-1">
                    <div class="truncate font-medium text-text" style="font-size: var(--ts-title)">
                      {p.name}
                    </div>
                    <div class="mt-0.5 font-mono text-text-3" style="font-size: var(--ts-overline)">
                      {p.engine} · {p.container} · {p.step_count} steps
                    </div>
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    {#if !p.enabled}
                      <span
                        class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]"
                        style="background: var(--surface-2); color: var(--text-3)"
                      >
                        OFF
                      </span>
                    {/if}
                    <Icon name="chevron-right" size={16} />
                  </div>
                </div>
              </a>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<TabBar active="profiles" />
