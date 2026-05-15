<script lang="ts">
  import SystemSection from './SystemSection.svelte';
  import NotificationsSection from './NotificationsSection.svelte';
  import RetentionSection from './RetentionSection.svelte';
  import RipBehaviourSection from './RipBehaviourSection.svelte';
</script>

<div class="mx-auto max-w-3xl space-y-7 px-7 py-6">
  <div>
    <div class="text-[11px] font-medium uppercase tracking-[0.14em] text-text-3">configuration</div>
    <h1 class="mt-1 text-[24px] font-bold tracking-tight text-text">System</h1>
  </div>
  <SystemSection />
  <RipBehaviourSection />
  <NotificationsSection />
  <RetentionSection />
</div>
