<script lang="ts">
  export let title: string;
  export let subtitle: string | undefined = undefined;
</script>

<div
  class="sticky top-0 z-20 px-5 pb-3 backdrop-blur"
  style="background: rgba(10,10,10,0.86); padding-top: calc(env(safe-area-inset-top, 0px) + 16px)"
>
  <div class="flex items-center justify-between gap-3">
    <div class="min-w-0">
      {#if subtitle}
        <div
          class="mb-1 font-medium uppercase tracking-[0.14em] text-text-3"
          style="font-size: var(--ts-overline)"
        >
          {subtitle}
        </div>
      {/if}
      <h1 class="text-[22px] font-bold leading-none tracking-tight text-text">{title}</h1>
    </div>
    <slot name="right" />
  </div>
</div>
