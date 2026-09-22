<script setup lang="ts">
// One light-set row's filter set: which clip filter a ONE-SHOT-COLOUR night was shot through.
//
// It shows the engine's measured verdict and lets the user overrule it. The distinction the cell
// has to make visible is measured-vs-asserted: a detected "dual-band" and a user-asserted
// "dual-band" look the same downstream but mean very different things if the run comes out wrong,
// so an override is marked rather than silently replacing the badge.
//
// "Unknown" is a first-class answer, not a failure: the classifier requires two independent signals
// to agree and declines otherwise (see internal/inspect/filterset.go). An unknown row is where the
// override earns its place.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FILTER_SETS, isKnownFilterSet } from "@/constants/filters";

const props = defineProps<{
  // The engine's measurement for this set ("" / "unknown" when it declined).
  detected: string;
  // The user's assertion, if any — always wins over `detected`.
  override?: string;
  // Filter sets are meaningless on a monochrome rig: it has a wheel and a FILTER card.
  osc?: boolean;
}>();

const emit = defineEmits<{ "update:override": [value: string] }>();

const { t } = useI18n();

const effective = computed(() =>
  isKnownFilterSet(props.override) ? props.override : props.detected,
);
const isOverridden = computed(() => isKnownFilterSet(props.override));

// Dual-band is tinted like the narrowband palettes it feeds; broadband like an ordinary colour run.
const badgeClass = computed(() =>
  effective.value === "dualband"
    ? "bg-brand-100 text-brand-700 dark:bg-brand-900/40 dark:text-brand-300"
    : "bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-200",
);

// The override select offers only real choices plus "auto"; "unknown" is the engine's word, never
// something a user asserts.
const choices = computed(() => FILTER_SETS.filter((f) => f !== "unknown"));
</script>

<template>
  <span v-if="!osc" class="text-slate-400">—</span>
  <span v-else class="inline-flex items-center gap-1.5" data-test="filter-set">
    <span
      v-if="isKnownFilterSet(effective)"
      data-test="filter-set-badge"
      class="rounded px-1.5 py-0.5 text-xs"
      :class="badgeClass"
      :title="isOverridden ? t('import.filterSetOverridden') : ''"
    >
      {{ t("import.filterSets." + effective)
      }}<span v-if="isOverridden" data-test="filter-set-override-mark">*</span>
    </span>
    <span v-else data-test="filter-set-unknown" class="text-xs text-slate-400">
      {{ t("import.filterSets.unknown") }}
    </span>

    <select
      data-test="filter-set-select"
      class="rounded border border-slate-300 bg-transparent px-1 py-0.5 text-xs dark:border-slate-600"
      :value="isKnownFilterSet(override) ? override : ''"
      :title="t('import.filterSetOverrideHint')"
      @change="
        emit('update:override', ($event.target as HTMLSelectElement).value)
      "
    >
      <option value="">{{ t("import.filterSetAuto") }}</option>
      <option v-for="f in choices" :key="f" :value="f">
        {{ t("import.filterSets." + f) }}
      </option>
    </select>
  </span>
</template>
