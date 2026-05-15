<script lang="ts">
  import Icon from '$lib/icons/Icon.svelte';
  import { goto } from '$app/navigation';

  export let title: string;
  export let backHref: string = '/settings';
</script>

<div class="min-h-screen pb-12">
  <div
    class="sticky top-0 z-20 flex items-center justify-between border-b border-border px-3 backdrop-blur"
    style="background: rgba(10,10,10,0.92); padding-top: calc(env(safe-area-inset-top, 0px) + 12px); padding-bottom: 12px"
  >
    <button
      class="flex min-h-[44px] min-w-[44px] items-center justify-center text-text-2"
      aria-label="Back"
      on:click={() => goto(backHref)}
    >
      <Icon name="chevron-left" size={20} />
    </button>
    <div class="min-w-0 flex-1 px-2 text-center">
      <div class="truncate font-medium text-text" style="font-size: var(--ts-body)">
        {title}
      </div>
    </div>
    <div class="w-11"></div>
  </div>

  <div class="px-4 pt-4">
    <slot />
  </div>
</div>
