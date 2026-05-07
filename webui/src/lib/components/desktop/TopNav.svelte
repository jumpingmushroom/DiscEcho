<script lang="ts">
  import { page } from '$app/stores';
  import { liveStatus } from '$lib/store';
  import LiveDot from '$lib/components/LiveDot.svelte';

  type Section = { id: string; label: string; href: string };

  const SECTIONS: Section[] = [
    { id: 'dashboard', label: 'Dashboard', href: '/' },
    { id: 'history', label: 'History', href: '/history' },
    { id: 'profiles', label: 'Profiles', href: '/profiles' },
    { id: 'system', label: 'System', href: '/system' },
  ];

  function isActive(pathname: string, href: string): boolean {
    if (href === '/') return pathname === '/' || pathname.startsWith('/jobs');
    return pathname.startsWith(href);
  }
</script>

<header
  class="sticky top-0 z-30 hidden border-b border-border backdrop-blur lg:block"
  style="background: rgba(10,10,10,0.86)"
>
  <div class="mx-auto flex h-14 max-w-screen-2xl items-center justify-between px-6">
    <div class="text-[16px] font-semibold tracking-tight text-text">DiscEcho</div>
    <nav class="flex items-center gap-1">
      {#each SECTIONS as s (s.id)}
        <a
          href={s.href}
          class="nav-link rounded-md px-3 py-1.5 text-[13px] font-medium text-text-2
                 transition-colors hover:bg-surface-2 hover:text-text"
          class:active={isActive($page.url.pathname, s.href)}
          data-sveltekit-preload-data="hover"
        >
          {s.label}
        </a>
      {/each}
    </nav>
    <LiveDot label={$liveStatus === 'live' ? 'LIVE' : 'WAIT'} />
  </div>
</header>

<style>
  .nav-link.active {
    color: var(--accent);
    background: var(--accent-soft);
  }
</style>
