<script lang="ts">
  import type { Job } from '$lib/wire';
  import { discs } from '$lib/store';
  import DiscMetadataPane from '$lib/components/DiscMetadataPane.svelte';

  export let job: Job | undefined = undefined;

  $: disc = job ? $discs[job.disc_id] : undefined;
</script>

{#if !job}
  <div class="rounded-2xl border border-border bg-surface-1 p-5">
    <div class="py-12 text-center text-[13px] text-text-3">
      Click a drive or queue row to inspect a job.
    </div>
  </div>
{:else}
  <DiscMetadataPane {disc} />
{/if}
