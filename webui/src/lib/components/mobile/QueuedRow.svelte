<script lang="ts">
  import type { Job, Disc, Profile } from '$lib/wire';
  import DiscTypeBadge from '$lib/components/DiscTypeBadge.svelte';

  export let job: Job;
  export let disc: Disc | undefined = undefined;
  export let profile: Profile | undefined = undefined;
</script>

<a
  href="/jobs/{job.id}"
  data-sveltekit-preload-data="hover"
  class="flex min-h-[44px] items-center gap-3 border-b border-border px-3 py-3 last:border-0 hover:bg-surface-2"
>
  {#if disc}<DiscTypeBadge type={disc.type} />{/if}
  <div class="min-w-0 flex-1">
    <div class="truncate font-medium text-text" style="font-size: var(--ts-meta)">
      {disc?.title || job.id.slice(0, 8)}
    </div>
    {#if profile}
      <div class="text-text-3" style="font-size: 10px">{profile.name}</div>
    {/if}
  </div>
  <span
    class="rounded px-1.5 py-0.5 font-bold uppercase tracking-[0.14em]"
    style="font-size: 9px; background: var(--surface-2); color: var(--text-3)"
  >
    queued
  </span>
</a>
