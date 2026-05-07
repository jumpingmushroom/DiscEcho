<script lang="ts">
  const STORAGE_KEY = 'discecho.iosInstallDismissed';

  function isIOS(): boolean {
    if (typeof window === 'undefined') return false;
    return /iPhone|iPad|iPod/.test(navigator.userAgent);
  }

  function isInstalled(): boolean {
    return (navigator as { standalone?: boolean }).standalone === true;
  }

  function isDismissed(): boolean {
    if (typeof window === 'undefined') return false;
    return localStorage.getItem(STORAGE_KEY) === 'true';
  }

  let show: boolean = isIOS() && !isInstalled() && !isDismissed();

  function dismiss(): void {
    localStorage.setItem(STORAGE_KEY, 'true');
    show = false;
  }
</script>

{#if show}
  <div
    class="fixed bottom-0 left-0 right-0 z-40 border-t border-border bg-surface-1 px-4 py-3"
    role="status"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="text-[12px] text-text-2">
        Install: tap <strong>Share</strong> then <strong>Add to Home Screen</strong>.
      </div>
      <button
        class="rounded-md border border-border px-2 py-1 text-[11px] text-text-3"
        on:click={dismiss}
      >
        Close
      </button>
    </div>
  </div>
{/if}
