<script setup lang="ts">
// "What did you shoot?" — an optional multi-select above the preset picker.
//
// It answers the one question the engine genuinely cannot: a galaxy and a dark nebula reach the
// pipeline as the same pixels, but they want opposite finishes (a galaxy tolerates stretch, a dark
// nebula is RUINED by it). Picking a type promotes the recipes suited to it; it never hides
// anything, because a built-in is tagged with what it is BEST for rather than exclusively for, and
// the user's own presets carry no tags at all.
//
// The list and its ORDER come from the engine (GET /api/presets → object_types); this component
// declares no taxonomy of its own — see internal/preset/objects.go.
import { useI18n } from "vue-i18n";

const props = defineProps<{
  types: string[];
  selected: string[];
}>();

const emit = defineEmits<{ "update:selected": [value: string[]] }>();

const { t } = useI18n();

// A chip label falls back to its raw slug rather than rendering an i18n key path: a type served by a
// newer engine than this UI should read as "oxygen-cloud", not "preset.objects.oxygen-cloud".
function label(type: string): string {
  const key = `preset.objects.${type}`;
  const translated = t(key);
  return translated === key ? type : translated;
}

function toggle(type: string) {
  const next = props.selected.includes(type)
    ? props.selected.filter((s) => s !== type)
    : [...props.selected, type];
  emit("update:selected", next);
}
</script>

<template>
  <div v-if="types.length" data-test="object-chips" class="mb-3">
    <div class="mb-1 flex flex-wrap items-center gap-2">
      <span class="text-xs font-medium text-slate-500">{{
        t("preset.objectsLabel")
      }}</span>
      <button
        v-if="selected.length"
        type="button"
        data-test="object-chips-clear"
        class="text-xs text-brand-600 hover:underline dark:text-brand-400"
        @click="emit('update:selected', [])"
      >
        {{ t("preset.objectsClear") }}
      </button>
    </div>
    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="type in types"
        :key="type"
        type="button"
        data-test="object-chip"
        :data-type="type"
        :aria-pressed="selected.includes(type)"
        :class="[
          'rounded-full border px-2.5 py-1 text-xs transition-colors',
          selected.includes(type)
            ? 'border-brand-600 bg-brand-600 text-white'
            : 'border-slate-300 text-slate-600 hover:bg-slate-100 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-700',
        ]"
        @click="toggle(type)"
      >
        {{ label(type) }}
      </button>
    </div>
    <p class="mt-1 text-xs text-slate-500">{{ t("preset.objectsHint") }}</p>
  </div>
</template>
