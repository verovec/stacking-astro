import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { apiGet, apiPost, apiPut, apiDelete } from "@/services/api";
import type { PresetItem, PresetPayload } from "@/types";

// Processing presets: the built-in "best params per situation" catalog (served read-only from the engine)
// plus the user's saved presets (persisted in Postgres). A preset is a partial /api/jobs body the Import
// view re-applies to the launch form. All API calls live here (Vue convention); components read state and
// dispatch actions. The list is fetched once and shared (cache + in-flight dedup), refreshed after a write.

// Picker group order. It must list EVERY category the engine serves (internal/preset: Category*),
// because a category missing here is not degraded — it is invisible: the five sun_* builtins were
// served and fully translated yet never rendered, for exactly that reason.
const CATEGORY_ORDER = [
  "deepsky",
  "nebula",
  "narrowband",
  "solar",
  "sun",
  "comet",
  "milkyway",
];

export const usePresetsStore = defineStore("presets", () => {
  const presets = ref<PresetItem[]>([]);
  // The object-type taxonomy, in the engine's display order (GET /api/presets → object_types).
  const objectTypes = ref<string[]>([]);
  const loading = ref(false);
  const error = ref("");
  let loaded = false;
  let inflight: Promise<void> | null = null;

  async function list(force = false): Promise<void> {
    if (!force && loaded) return; // cache hit
    if (inflight) return inflight; // in-flight dedup
    loading.value = true;
    error.value = "";
    inflight = (async () => {
      try {
        const d = await apiGet<{
          presets: PresetItem[];
          object_types?: string[];
        }>("/api/presets");
        presets.value = d.presets || [];
        // Served in display order by the engine (internal/preset.ObjectTypes) — rendered as-is.
        objectTypes.value = d.object_types || [];
        loaded = true;
      } catch (e) {
        error.value = (e as Error).message;
      } finally {
        loading.value = false;
        inflight = null;
      }
    })();
    return inflight;
  }

  // save upserts a user preset by name (re-saving the same name overwrites), then refreshes the list.
  async function save(name: string, payload: PresetPayload): Promise<void> {
    await apiPost("/api/presets", { name, payload });
    await list(true);
  }
  async function rename(id: number, name: string): Promise<void> {
    await apiPut(`/api/presets/${id}`, { name });
    await list(true);
  }
  // setFavorite stars/unstars a saved preset — starred presets sort first in the picker.
  async function setFavorite(id: number, favorite: boolean): Promise<void> {
    await apiPut(`/api/presets/${id}`, { favorite });
    await list(true);
  }
  async function remove(id: number): Promise<void> {
    await apiDelete(`/api/presets/${id}`);
    await list(true);
  }

  const builtins = computed(() => presets.value.filter((p) => p.builtin));
  const userPresets = computed(() => presets.value.filter((p) => !p.builtin));

  // byCategory groups the picker: built-ins by category in a stable order, then the user's own presets
  // under a synthetic "mine" group (only when non-empty).
  const byCategory = computed(() => {
    const groups: { key: string; items: PresetItem[] }[] = [];
    for (const cat of CATEGORY_ORDER) {
      const items = builtins.value.filter((p) => p.category === cat);
      if (items.length) groups.push({ key: cat, items });
    }
    if (userPresets.value.length) {
      // Favorites first; the stable sort keeps the API's name order within each half.
      const mine = [...userPresets.value].sort(
        (a, b) => Number(!!b.favorite) - Number(!!a.favorite),
      );
      groups.push({ key: "mine", items: mine });
    }
    return groups;
  });

  // The object-type chips the user picked ("what did you shoot?"). Empty = no opinion, which is the
  // default and leaves the picker exactly as it was before the taxonomy existed.
  const selectedObjects = ref<string[]>([]);

  // groups is what the picker renders. With no chips picked it IS byCategory. With chips picked it
  // RE-ORDERS rather than filters: matching recipes are promoted to the top, everything else keeps
  // its place under "other", and the user's own presets always get their group. Culling would be the
  // wrong behaviour twice over — a built-in is tagged with what it is BEST for, not exclusively for,
  // and a user preset carries no tags at all, so a filter that hides would hide precisely the recipes
  // the engine had no way to label.
  const groups = computed(() => {
    if (!selectedObjects.value.length) return byCategory.value;
    const wanted = new Set(selectedObjects.value);
    const matches = (p: PresetItem) =>
      (p.objects ?? []).some((o) => wanted.has(o));

    const out: { key: string; items: PresetItem[] }[] = [];
    const matching = builtins.value.filter(matches);
    const other = builtins.value.filter((p) => !matches(p));
    if (matching.length) out.push({ key: "matching", items: matching });
    if (other.length) out.push({ key: "other", items: other });
    if (userPresets.value.length) {
      const mine = [...userPresets.value].sort(
        (a, b) => Number(!!b.favorite) - Number(!!a.favorite),
      );
      out.push({ key: "mine", items: mine });
    }
    return out;
  });

  return {
    presets,
    loading,
    error,
    objectTypes,
    selectedObjects,
    groups,
    list,
    save,
    rename,
    setFavorite,
    remove,
    builtins,
    userPresets,
    byCategory,
  };
});

// payloadFromRunParams extracts the preset recipe from a finished job's persisted params (the
// RunRequest JSON — every PresetPayload field shares its wire name with the run request), so a good
// run can be saved as a preset straight from its job page. Input-specific fields are dropped.
export function payloadFromRunParams(
  params: Record<string, unknown>,
): PresetPayload {
  const out: PresetPayload = {};
  if (typeof params.mode === "string" && params.mode) out.mode = params.mode;
  if (typeof params.format === "string" && params.format)
    out.format = params.format;
  if (typeof params.palette === "string" && params.palette)
    out.palette = params.palette;
  if (typeof params.look === "string" && params.look) out.look = params.look;
  if (typeof params.brightness === "string" && params.brightness)
    out.brightness = params.brightness;
  if (typeof params.goal === "string" && params.goal) out.goal = params.goal;
  if (typeof params.color_calibration === "boolean")
    out.color_calibration = params.color_calibration;
  if (typeof params.denoise === "boolean") out.denoise = params.denoise;
  if (typeof params.ha_exclude_stars === "boolean")
    out.ha_exclude_stars = params.ha_exclude_stars;
  if (typeof params.output_luminance === "boolean")
    out.output_luminance = params.output_luminance;
  if (typeof params.output_mono_stack === "boolean")
    out.output_mono_stack = params.output_mono_stack;
  if (typeof params.drop_wheel_transition === "boolean")
    out.drop_wheel_transition = params.drop_wheel_transition;
  if (typeof params.supervise === "boolean") out.supervise = params.supervise;
  const p = params.params;
  if (p && typeof p === "object" && !Array.isArray(p) && Object.keys(p).length)
    out.params = p as Record<string, unknown>;
  return out;
}
