<script lang="ts">
  export let label: string;
</script>

<div class="flex items-center justify-between px-4 py-3">
  <span class="text-[12px] text-text-3">{label}</span>
  <span class="font-mono text-[13px] text-text-2"><slot /></span>
</div>
