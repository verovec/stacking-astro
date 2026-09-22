import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import type { PresetItem } from "@/types";

const apiGet = vi.fn(async (..._args: unknown[]) => ({ presets: catalog }));
vi.mock("@/services/api", () => ({
  ApiError: class extends Error {},
  apiGet: (...args: unknown[]) => apiGet(...args),
  apiPost: vi.fn(async () => ({})),
  apiPut: vi.fn(async () => ({})),
  apiDelete: vi.fn(async () => ({})),
}));

import { usePresetsStore } from "./presets";

function builtin(
  name: string,
  category: string,
  objects: string[],
): PresetItem {
  return {
    id: 0,
    name,
    category,
    builtin: true,
    objects,
    payload: { mode: "deepsky" },
  } as PresetItem;
}

// A slice of the real catalog: one per category, including the five sun builtins that the picker
// never rendered because CATEGORY_ORDER omitted "sun".
let catalog: PresetItem[] = [];

beforeEach(() => {
  catalog = [
    builtin("galaxy-lrgb", "deepsky", ["galaxy"]),
    builtin("star-cluster", "deepsky", ["star-cluster"]),
    builtin("emission-hargb", "nebula", ["emission-nebula"]),
    builtin("oxygen-cloud", "nebula", ["oxygen-cloud"]),
    builtin("narrowband-sho", "narrowband", [
      "emission-nebula",
      "supernova-remnant",
    ]),
    builtin("moon", "solar", ["moon"]),
    builtin("comet", "comet", ["comet"]),
    builtin("milkyway-natural", "milkyway", ["milkyway"]),
    builtin("sun_ha_full", "sun", ["sun"]),
    {
      id: 3,
      name: "my recipe",
      builtin: false,
      payload: { mode: "deepsky" },
    } as PresetItem,
  ];
  setActivePinia(createPinia());
  apiGet.mockClear();
});

const keys = (groups: { key: string }[]) => groups.map((g) => g.key);
const names = (items: PresetItem[]) => items.map((p) => p.name);

describe("presets store grouping", () => {
  // The bug this pins: the five sun builtins were served by the engine AND translated, but
  // CATEGORY_ORDER never listed "sun", so the picker silently dropped a whole category.
  it("renders the sun category", async () => {
    const store = usePresetsStore();
    await store.list();
    expect(keys(store.byCategory)).toContain("sun");
    const sun = store.byCategory.find((g) => g.key === "sun");
    expect(names(sun!.items)).toEqual(["sun_ha_full"]);
  });

  it("keeps every served category reachable", async () => {
    const store = usePresetsStore();
    await store.list();
    const served = new Set(
      catalog.filter((p) => p.builtin).map((p) => p.category),
    );
    for (const cat of served) {
      expect(keys(store.byCategory)).toContain(cat);
    }
  });
});

describe("presets store object-type filter", () => {
  it("is inert until the user picks a type", async () => {
    const store = usePresetsStore();
    await store.list();
    expect(store.selectedObjects).toEqual([]);
    expect(keys(store.groups)).toEqual(keys(store.byCategory));
  });

  it("ranks matching presets first and keeps the rest under 'other'", async () => {
    const store = usePresetsStore();
    await store.list();
    store.selectedObjects = ["galaxy"];

    expect(keys(store.groups)).toEqual(["matching", "other", "mine"]);
    const matching = store.groups.find((g) => g.key === "matching")!;
    expect(names(matching.items)).toEqual(["galaxy-lrgb"]);
  });

  it("never hides a preset — filtering re-orders, it does not cull", async () => {
    const store = usePresetsStore();
    await store.list();
    store.selectedObjects = ["comet"];

    const shown = store.groups.flatMap((g) => names(g.items));
    expect(shown.sort()).toEqual(names(catalog).sort());
  });

  // A user's own preset carries no taxonomy (the engine cannot tag it), so a chip must never push it
  // out of sight — the user already knows what their recipe is for.
  it("always keeps user presets in their own group", async () => {
    const store = usePresetsStore();
    await store.list();
    store.selectedObjects = ["planetary-nebula"];

    const mine = store.groups.find((g) => g.key === "mine");
    expect(mine).toBeDefined();
    expect(names(mine!.items)).toEqual(["my recipe"]);
  });

  it("treats several chips as a union, not an intersection", async () => {
    const store = usePresetsStore();
    await store.list();
    store.selectedObjects = ["galaxy", "comet"];

    const matching = store.groups.find((g) => g.key === "matching")!;
    expect(names(matching.items).sort()).toEqual(["comet", "galaxy-lrgb"]);
  });

  it("matches a preset that carries several object types", async () => {
    const store = usePresetsStore();
    await store.list();
    store.selectedObjects = ["supernova-remnant"];

    const matching = store.groups.find((g) => g.key === "matching")!;
    expect(names(matching.items)).toEqual(["narrowband-sho"]);
  });

  it("yields no matching group when nothing is tagged with the pick", async () => {
    const store = usePresetsStore();
    await store.list();
    store.selectedObjects = ["dark-nebula"];

    expect(keys(store.groups)).toEqual(["other", "mine"]);
  });
});
