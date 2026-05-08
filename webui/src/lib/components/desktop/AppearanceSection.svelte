<script lang="ts">
  import { settings, updatePrefs } from '$lib/store';

  $: accent = ($settings['prefs.accent'] as string) ?? 'aurora';
  $: mood = ($settings['prefs.mood'] as string) ?? 'void';
  $: density = ($settings['prefs.density'] as string) ?? 'standard';

  async function onChange(
    patch: Partial<{ accent: string; mood: string; density: string }>,
  ): Promise<void> {
    await updatePrefs({ accent, mood, density, ...patch });
  }
</script>

<section class="rounded-2xl border border-border bg-surface-1 p-5">
  <h2 class="text-[14px] font-semibold text-text">Appearance</h2>
  <div class="mt-4 grid gap-3" style="grid-template-columns: 120px 1fr">
    <label class="text-[12px] text-text-3" for="ap-accent">Accent</label>
    <select
      id="ap-accent"
      name="accent"
      bind:value={accent}
      on:change={() => onChange({ accent })}
      class="rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
    >
      <option value="aurora">Aurora</option>
      <option value="amber">Amber</option>
      <option value="cobalt">Cobalt</option>
      <option value="mono">Mono</option>
    </select>

    <label class="text-[12px] text-text-3" for="ap-mood">Mood</label>
    <select
      id="ap-mood"
      name="mood"
      bind:value={mood}
      on:change={() => onChange({ mood })}
      class="rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
    >
      <option value="void">Void</option>
      <option value="carbon">Carbon</option>
      <option value="aurora">Aurora</option>
    </select>

    <label class="text-[12px] text-text-3" for="ap-density">Density</label>
    <select
      id="ap-density"
      name="density"
      bind:value={density}
      on:change={() => onChange({ density })}
      class="rounded-md border border-border bg-surface-2 px-2 py-1.5 text-[13px] text-text"
    >
      <option value="compact">Compact</option>
      <option value="standard">Standard</option>
      <option value="cinematic">Cinematic</option>
    </select>
  </div>
</section>
