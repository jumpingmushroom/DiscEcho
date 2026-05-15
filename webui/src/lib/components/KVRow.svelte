<script lang="ts">
  export let label: string;
</script>

<div class="flex items-center justify-between px-4 py-3">
  <span class="text-text-3" style="font-size: var(--ts-meta)">{label}</span>
  <span class="font-mono text-text-2" style="font-size: var(--ts-body)"><slot /></span>
</div>
