<script lang="ts">
  import { notifications } from '$lib/store';
  import NotificationEditor from './NotificationEditor.svelte';

  let creating = false;

  function onCreated(): void {
    creating = false;
  }
</script>

<section data-section="notifications" class="rounded-2xl border border-border bg-surface-1 p-5">
  <div class="flex items-center justify-between">
    <h2 class="text-[14px] font-semibold text-text">Notifications</h2>
    <button
      on:click={() => (creating = true)}
      class="rounded-md border border-border px-3 py-1.5 text-[12px] text-text-2"
    >
      + New notification
    </button>
  </div>

  <div class="mt-4 space-y-3">
    {#each $notifications as n (n.id)}
      <NotificationEditor notification={n} creating={false} />
    {/each}
    {#if creating}
      <NotificationEditor notification={null} creating={true} on:saved={onCreated} />
    {/if}
    {#if $notifications.length === 0 && !creating}
      <div class="text-[12px] text-text-3">
        No notifications. Click "+ New notification" to add one.
      </div>
    {/if}
  </div>
</section>
