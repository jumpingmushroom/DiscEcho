<script lang="ts">
  export let active: 'dashboard' | 'history' | 'settings';

  const tabs: Array<{ id: 'dashboard' | 'history' | 'settings'; label: string; href: string }> = [
    { id: 'dashboard', label: 'Drives', href: '/' },
    { id: 'history', label: 'History', href: '/history' },
    { id: 'settings', label: 'Settings', href: '/settings' },
  ];
</script>

<div
  class="fixed bottom-0 left-0 right-0 z-30"
  style="padding-bottom: env(safe-area-inset-bottom, 0px)"
>
  <div
    class="mx-3 mb-2 rounded-2xl border border-border backdrop-blur"
    style="background: rgba(11,11,12,0.92); box-shadow: 0 12px 32px rgba(0,0,0,0.5)"
  >
    <div class="flex items-stretch">
      {#each tabs as t (t.id)}
        <a
          href={t.href}
          class="flex min-h-[44px] flex-1 flex-col items-center justify-center gap-0.5 py-2
                 text-[10px] font-medium transition-colors"
          class:active={t.id === active}
          data-sveltekit-preload-data="hover"
        >
          <span class:accent={t.id === active}>{t.label}</span>
        </a>
      {/each}
    </div>
  </div>
</div>

<style>
  .accent {
    color: var(--accent);
  }
  a {
    color: var(--text-3);
  }
</style>
