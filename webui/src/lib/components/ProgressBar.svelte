<script lang="ts">
  export let value: number = 0;
  export let height: number = 4;
  export let animated: boolean = true;
</script>

<div class="relative w-full overflow-hidden rounded-full bg-surface-3" style="height: {height}px">
  <div
    class="progress-fill absolute inset-y-0 left-0"
    class:animated
    style="width: {Math.max(0, Math.min(100, value))}%; height: 100%"
  ></div>
</div>

<style>
  .progress-fill {
    background: var(--accent);
  }
</style>
