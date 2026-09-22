<script setup lang="ts">
// RigPanel asks the two questions the engine otherwise answers for you, silently.
//
//  1. Monochrome or one-shot colour? Inferred from the headers and the pixels, which is right almost
//     always and unrecoverable when it is wrong — a header-less camera, a folder holding two rigs,
//     a capture program stamping BAYERPAT on a mono sensor. "Detected" stays the default, so an
//     untouched form asserts nothing and the run is byte-identical to one launched without the knob.
//  2. What optics shot it? focal_mm / pixel_um were already request fields but reachable only through
//     the advanced JSON, and omitting them is NOT "unset": the engine falls back to its configured
//     telescope, at whose scale a camera-lens field cannot plate-solve at all. Hence the flag.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { card, input } from "@/constants/styles";
import type { EquipmentSetup } from "@/types";

const props = withDefaults(
  defineProps<{
    // The scan's own verdict (inventory.color_model). Absent before the folder is inspected.
    detected?: "mono" | "osc" | "mixed";
    colorModel: "auto" | "mono" | "osc";
    focalMm?: number;
    pixelUm?: number;
    // The user's saved rigs (table equipment_setups) — the prefill source for the optics.
    rigs?: EquipmentSetup[];
  }>(),
  {
    detected: undefined,
    focalMm: undefined,
    pixelUm: undefined,
    rigs: () => [],
  },
);

const emit = defineEmits<{
  "update:colorModel": [value: "auto" | "mono" | "osc"];
  "update:focalMm": [value: number | undefined];
  "update:pixelUm": [value: number | undefined];
}>();

const { t } = useI18n();

// What "detected" resolves to, spelled out. A mixed verdict says which half auto keeps, because that
// is the one case where doing nothing loses frames.
const detectedLabel = computed(() =>
  t(`run.colorModelDetected.${props.detected ?? "unknown"}`),
);

// Both optics are needed for a scale the solver can use, so one alone still earns the flag.
const opticsIncomplete = computed(() => !props.focalMm || !props.pixelUm);

// An empty field means "unset", never 0 — a zero would travel as a real value and be applied.
function numberOrUndefined(raw: string): number | undefined {
  const n = Number(raw);
  return raw.trim() === "" || !Number.isFinite(n) || n <= 0 ? undefined : n;
}

function onFocal(e: Event) {
  emit(
    "update:focalMm",
    numberOrUndefined((e.target as HTMLInputElement).value),
  );
}

function onPixel(e: Event) {
  emit(
    "update:pixelUm",
    numberOrUndefined((e.target as HTMLInputElement).value),
  );
}

// Picking a saved rig fills both optics at once — they describe one physical setup and are useless
// apart, so offering them as two separate prefills would invite a half-applied rig.
function onRig(e: Event) {
  const rig = props.rigs.find(
    (r) => r.id === (e.target as HTMLSelectElement).value,
  );
  if (!rig) return;
  emit("update:focalMm", rig.focal_mm || undefined);
  emit("update:pixelUm", rig.pixel_um || undefined);
}
</script>

<template>
  <div :class="card">
    <h3 class="mb-3 text-sm font-semibold text-slate-700 dark:text-slate-200">
      {{ t("run.optics") }}
    </h3>

    <label data-test="color-model" class="block text-sm">
      <span class="mb-1 block text-xs font-medium text-slate-500">{{
        t("run.colorModel")
      }}</span>
      <select
        data-test="color-model-select"
        :class="input"
        :value="colorModel"
        @change="
          emit(
            'update:colorModel',
            ($event.target as HTMLSelectElement).value as
              | 'auto'
              | 'mono'
              | 'osc',
          )
        "
      >
        <option value="auto">
          {{ t("run.colorModelAuto", { detected: detectedLabel }) }}
        </option>
        <option value="mono">{{ t("run.colorModelMono") }}</option>
        <option value="osc">{{ t("run.colorModelOsc") }}</option>
      </select>
      <span class="mt-1 block text-xs text-slate-500">{{
        t("run.colorModelHint")
      }}</span>
    </label>

    <p class="mt-4 text-xs text-slate-500">{{ t("run.opticsHint") }}</p>

    <label
      v-if="rigs.length"
      class="mt-2 block text-sm"
      data-test="rig-select-label"
    >
      <span class="mb-1 block text-xs font-medium text-slate-500">{{
        t("run.opticsRig")
      }}</span>
      <select data-test="rig-select" :class="input" value="" @change="onRig">
        <option value="">{{ t("run.opticsRigPlaceholder") }}</option>
        <option v-for="rig in rigs" :key="rig.id" :value="rig.id">
          {{ rig.name }}
        </option>
      </select>
    </label>

    <div class="mt-2 grid gap-3 sm:grid-cols-2">
      <label class="block text-sm">
        <span class="mb-1 block text-xs font-medium text-slate-500">{{
          t("run.opticsFocal")
        }}</span>
        <input
          data-test="focal-mm"
          type="number"
          min="0"
          step="0.1"
          :class="input"
          :value="focalMm ?? ''"
          @input="onFocal"
        />
      </label>
      <label class="block text-sm">
        <span class="mb-1 block text-xs font-medium text-slate-500">{{
          t("run.opticsPixel")
        }}</span>
        <input
          data-test="pixel-um"
          type="number"
          min="0"
          step="0.01"
          :class="input"
          :value="pixelUm ?? ''"
          @input="onPixel"
        />
      </label>
    </div>

    <p
      v-if="opticsIncomplete"
      data-test="optics-warning"
      role="status"
      class="mt-2 text-xs text-warning-700 dark:text-warning-300"
    >
      {{ t("run.opticsWarning") }}
    </p>
  </div>
</template>
