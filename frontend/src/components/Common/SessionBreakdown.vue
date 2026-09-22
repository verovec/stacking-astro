<script setup lang="ts">
// Per-capture-night breakdown of a multi-night selection: one collapsible panel per night (date,
// time window, per-config light counts, that night's own calibration counters) plus the read-only
// per-night calibration mapping from the joined run plan (which dark/flat/bias each night's lights
// will get — library / from this capture / rebuilt from that night's flats), the clip filter a
// one-shot-colour night was shot through, and a warning chip when the planned flat is a fallback.
//
// It renders for a SINGLE night too (card 0006). It used to hide itself below two nights to keep the
// Import view pixel-identical for the common case — which left the single-night capture, i.e. most
// captures, as the one case with no way to see what it was about to be calibrated with. It still
// renders nothing when there are no nights at all (an undated scan reports none).
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import AccordionGroup from "@/components/Common/AccordionGroup.vue";
import StatGrid from "@/components/Common/StatGrid.vue";
import FilterChip from "@/components/Common/FilterChip.vue";
import Pill from "@/components/Common/Pill.vue";
import { frameTypeAccentClass } from "@/constants/styles";
import { isKnownFilterSet } from "@/constants/filters";
import { humanizeMs } from "@/utils/format";
import type {
  PlanGroup,
  PlanMaster,
  RunPlanPreview,
  SessionInfo,
} from "@/types";

const props = defineProps<{
  sessions: SessionInfo[];
  // The joined run plan (POST /api/calib/plan); optional — without it the calibration mapping block
  // shows the "no matched calibration" line.
  plan?: RunPlanPreview | null;
}>();
const { t } = useI18n();

// One accordion item per night, all open by default (transient UI — no storage key).
const keyOf = (s: SessionInfo) => s.key || "undated";
const items = computed(() =>
  props.sessions.map((s) => ({ key: keyOf(s), title: title(s) })),
);
const openKeys = computed(() => items.value.map((it) => it.key));

function title(s: SessionInfo): string {
  return s.key
    ? t("sessions.nightTitle", { date: s.key })
    : t("sessions.undated");
}

const lightCount = (s: SessionInfo) => s.counts?.LIGHT ?? 0;
const integrationMs = (s: SessionInfo) =>
  (s.configs ?? []).reduce((sum, c) => sum + c.exposure_ms * c.count, 0);
const filtersOf = (s: SessionInfo) => [
  ...new Set(
    (s.configs ?? []).map((c) => c.filter).filter(Boolean) as string[],
  ),
];
// Uneven channel sets across nights (a night that shot only L/R): the union of all nights'
// filters, so each night can name the channels it does NOT feed — those come from the others.
const allFilters = computed(() => [
  ...new Set(props.sessions.flatMap((s) => filtersOf(s))),
]);
const missingFilters = (s: SessionInfo) => {
  const own = new Set(filtersOf(s));
  return allFilters.value.filter((f) => !own.has(f));
};
const isAnchor = (s: SessionInfo) =>
  !!props.plan?.anchor_night && props.plan.anchor_night === s.key;
// Calibration counters for the chips: that night's own dark/flat/bias frames.
const calibCounts = (s: SessionInfo) =>
  ["DARK", "FLAT", "BIAS", "DARKFLAT"]
    .map((type) => ({ type, n: s.counts?.[type] ?? 0 }))
    .filter((c) => c.n > 0);

function clock(ms?: number): string {
  if (!ms) return "—";
  const d = new Date(ms);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}
function stats(s: SessionInfo) {
  return [
    {
      label: t("sessions.window"),
      value: `${clock(s.start_ms)} → ${clock(s.end_ms)}`,
    },
    { label: t("capture.lights"), value: lightCount(s) },
    { label: t("capture.integration"), value: humanizeMs(integrationMs(s)) },
    {
      label: t("fields.gain"),
      value:
        [...new Set((s.configs ?? []).map((c) => c.gain))].join(" / ") || "—",
    },
  ];
}

// The night's calibration mapping: the plan's CURRENT groups captured that night (the filter lives
// on the channel, so pair it in).
function groupsFor(s: SessionInfo): { filter: string; g: PlanGroup }[] {
  const night = s.key;
  return (props.plan?.channels ?? []).flatMap((c) =>
    c.groups
      .filter((g) => g.current && (g.session ?? "") === night)
      .map((g) => ({ filter: c.filter, g })),
  );
}
function roles(g: PlanGroup): { role: string; pm: PlanMaster }[] {
  return [
    { role: "dark", pm: g.dark },
    { role: "flat", pm: g.flat },
    { role: "bias", pm: g.bias },
  ].filter((r): r is { role: string; pm: PlanMaster } => !!r.pm);
}
function masterLine(pm: PlanMaster): string {
  if (pm.raw_flats) return t("calib.frames", { n: pm.raw_flats });
  const m = pm.master;
  if (!m) return "";
  const parts = [t("calib.frames", { n: m.frame_count })];
  if (m.exposure_ms) parts.push(humanizeMs(m.exposure_ms));
  parts.push(`gain ${m.gain}`);
  if (m.filter) parts.push(m.filter);
  return parts.join(" · ");
}
function sourceKey(pm: PlanMaster): string {
  if (pm.source === "capture") return "calib.source.capture";
  if (pm.source === "session-rebuild") return "calib.source.sessionFlat";
  return "calib.source.library";
}
function sourceClass(pm: PlanMaster): string {
  if (pm.source === "capture") return "bg-brand-500/10 text-brand-500";
  if (pm.source === "session-rebuild")
    return "bg-violet-500/10 text-violet-500 dark:text-violet-400";
  return "bg-slate-500/10 text-slate-400 dark:text-slate-500";
}

// The clip filter a role's own master was shot through, when the engine measured one. Only ever set
// on a FLAT: a dark and a bias are closed-shutter, so no filter was in their light path at all.
function masterFilterSet(pm: PlanMaster): string | undefined {
  const fs = pm.master?.filter_set;
  return isKnownFilterSet(fs) ? fs : undefined;
}
// A flat shot through a different clip filter than the lights. It only reaches the UI under
// force_calibration_frames — unforced, the run refuses it and there is no flat row to mark — so
// saying so on the row itself is the difference between "your master was applied" and "your master
// was applied and it is the wrong one".
function isCrossSet(g: PlanGroup, pm: PlanMaster): boolean {
  const lights = g.filter_set;
  const flat = masterFilterSet(pm);
  return isKnownFilterSet(lights) && !!flat && flat !== lights;
}
const filterSetLabel = (fs: string) => t("import.filterSets." + fs);
</script>

<template>
  <section v-if="sessions.length" data-demo="session-breakdown">
    <h3 class="font-semibold">{{ t("sessions.title") }}</h3>
    <p class="mb-2 text-xs text-slate-500 dark:text-slate-400">
      {{
        sessions.length > 1
          ? t("sessions.subtitle", { n: sessions.length })
          : t("sessions.subtitleOne")
      }}
    </p>
    <AccordionGroup :items="items" :default-open="openKeys">
      <template
        v-for="s in sessions"
        :key="'h' + keyOf(s)"
        #[`header-${keyOf(s)}`]
      >
        <span class="flex min-w-0 flex-wrap items-center gap-2">
          <span class="text-sm font-semibold">{{ title(s) }}</span>
          <Pill
            v-if="isAnchor(s)"
            :title="t('sessions.anchorHint')"
            color-class="bg-brand-500/10 text-brand-500"
          >
            {{ t("sessions.anchor") }}
          </Pill>
          <span class="text-xs text-slate-500 dark:text-slate-400">
            {{ t("calib.frames", { n: lightCount(s) }) }} ·
            {{ humanizeMs(integrationMs(s)) }}
          </span>
          <FilterChip v-for="f in filtersOf(s)" :key="f" :filter="f" />
          <span
            v-for="f in missingFilters(s)"
            :key="'m' + f"
            class="text-xs text-slate-400 line-through opacity-60 dark:text-slate-600"
            :title="t('sessions.missingChannels', { filters: f })"
          >
            {{ f }}
          </span>
          <span
            v-for="c in calibCounts(s)"
            :key="c.type"
            class="text-xs font-medium"
            :class="frameTypeAccentClass(c.type)"
          >
            {{ c.n }} {{ c.type }}
          </span>
        </span>
      </template>

      <template v-for="s in sessions" :key="'b' + keyOf(s)" #[keyOf(s)]>
        <StatGrid :items="stats(s)" :cols="4" />
        <p
          v-if="missingFilters(s).length"
          class="mt-2 text-xs text-slate-500 dark:text-slate-400"
        >
          {{
            t("sessions.missingChannels", {
              filters: missingFilters(s).join(", "),
            })
          }}
        </p>

        <!-- Per-config light sets of this night -->
        <div class="mt-3 space-y-1">
          <div
            v-for="(c, i) in s.configs ?? []"
            :key="i"
            class="flex items-center gap-2 text-sm"
          >
            <FilterChip v-if="c.filter" :filter="c.filter" />
            <span v-else class="text-slate-400">—</span>
            <span class="text-xs text-slate-500 dark:text-slate-400">
              {{ humanizeMs(c.exposure_ms) }} · gain {{ c.gain }} ·
              {{ t("calib.frames", { n: c.count }) }}
            </span>
          </div>
        </div>

        <!-- This night's calibration mapping (from the joined run plan) -->
        <h4
          class="mt-4 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400"
        >
          {{ t("sessions.calibTitle") }}
        </h4>
        <div v-if="groupsFor(s).length" class="mt-1 space-y-2">
          <div v-for="({ filter, g }, gi) in groupsFor(s)" :key="gi">
            <div class="mb-0.5 flex flex-wrap items-center gap-2 text-sm">
              <FilterChip v-if="filter" :filter="filter" />
              <span class="text-xs text-slate-500 dark:text-slate-400">
                {{ humanizeMs(g.exposure_ms) }} · gain {{ g.gain }}
              </span>
              <!-- Which clip filter these lights were shot through (one-shot-colour only). -->
              <Pill
                v-if="isKnownFilterSet(g.filter_set)"
                data-test="lights-filter-set"
                :title="t('sessions.lightsFilterSetHint')"
                color-class="bg-brand-500/10 text-brand-500"
              >
                {{ filterSetLabel(g.filter_set) }}
              </Pill>
              <!-- The flat is not the clean case. The notes below say which way. -->
              <Pill
                v-if="g.flat_fallback"
                data-test="flat-fallback"
                :title="t('sessions.flatFallbackHint')"
                color-class="bg-warning/10 text-warning"
              >
                ⚠ {{ t("sessions.flatFallback") }}
              </Pill>
            </div>
            <div
              v-for="r in roles(g)"
              :key="r.role"
              class="flex flex-wrap items-center gap-2 pl-1 text-sm"
            >
              <span
                class="w-10 font-medium"
                :class="frameTypeAccentClass(r.role.toUpperCase())"
              >
                {{ t("calib.role." + r.role) }}
              </span>
              <span class="text-slate-500 dark:text-slate-400">{{
                masterLine(r.pm)
              }}</span>
              <!-- A per-night master names its night: that is how a borrowed flat becomes visible. -->
              <span
                v-if="
                  r.pm.master?.session &&
                  r.pm.master.session !== (g.session ?? '')
                "
                data-test="master-night"
                class="text-xs text-warning"
                :title="t('sessions.flatOtherNightHint')"
              >
                {{ t("sessions.fromNight", { date: r.pm.master.session }) }}
              </span>
              <span
                v-if="masterFilterSet(r.pm)"
                data-test="master-filter-set"
                class="text-xs"
                :class="
                  isCrossSet(g, r.pm)
                    ? 'text-warning'
                    : 'text-slate-500 dark:text-slate-400'
                "
                :title="
                  isCrossSet(g, r.pm) ? t('sessions.flatCrossSetHint') : ''
                "
              >
                {{ filterSetLabel(masterFilterSet(r.pm)!) }}
              </span>
              <Pill class="ml-auto shrink-0" :color-class="sourceClass(r.pm)">
                {{ t(sourceKey(r.pm)) }}
              </Pill>
            </div>
            <p
              v-for="(n, ni) in g.notes"
              :key="ni"
              class="pl-2 text-xs text-warning"
            >
              ⚠ {{ n }}
            </p>
          </div>
        </div>
        <p v-else class="mt-1 text-xs text-slate-500 dark:text-slate-400">
          {{ t("sessions.noCalib") }}
        </p>
      </template>
    </AccordionGroup>
  </section>
</template>
