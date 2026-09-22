import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { useEquipmentStore } from "@/stores/equipment";
import type { SkyQueryEcho, LocationFavorite, EquipmentSetup } from "@/types";

// The observation-planning half (Tonight targets, dark-sky finder, weather, calendar) left with
// E01 card 0015 — this store keeps only the observer-location state and equipment surface that the
// Mosaic planner still reads.

// SkyQuery holds the persisted observing location (set via setObserver / read as params.lat/lon).
export interface SkyQuery {
  lat?: number;
  lon?: number;
  elevation_m?: number;
}

const STORAGE_KEY = "astrostack.sky.query";

function loadPersisted(): SkyQuery {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as SkyQuery) : {};
  } catch {
    return {};
  }
}

function persist(q: SkyQuery) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(q));
  } catch {
    // ignore quota / private-mode errors
  }
}

const LOC_FAV_KEY = "astrostack.sky.locationFavorites";

function loadLocationFavorites(): LocationFavorite[] {
  try {
    const raw = localStorage.getItem(LOC_FAV_KEY);
    return raw ? (JSON.parse(raw) as LocationFavorite[]) : [];
  } catch {
    return [];
  }
}

export const useSkyStore = defineStore("sky", () => {
  // The last targets-endpoint echo (location + equipment as the server resolved them). The Tonight
  // planner that populated it is gone, so this stays null; it is kept because the mosaic store and
  // MosaicSetupForm still read it as their last-resort optics/site fallback (behind optional
  // chaining), exactly as they did when the planner had not yet fetched.
  const query = ref<SkyQueryEcho | null>(null);
  const params = ref<SkyQuery>(loadPersisted());
  const locationFavorites = ref<LocationFavorite[]>(loadLocationFavorites()); // saved sites, persisted

  // Saved telescope/camera rigs live server-side (stores/equipment.ts) so the desktop planner and
  // the phone at the scope share them; this store keeps the old surface so existing callers are
  // unchanged. The equipment store imports any legacy localStorage rigs on its first load.
  const equipment = useEquipmentStore();
  void equipment.load();
  const equipmentSetups = computed<EquipmentSetup[]>(() => equipment.setups);

  // setObserver sets the observing location (lat/lon) and persists it.
  function setObserver(lat: number, lon: number): void {
    params.value = { ...params.value, lat, lon };
    persist(params.value);
  }

  // favLocKey rounds to ~11 m so saving the same spot twice is idempotent and "is the current site
  // saved?" is a cheap lookup. It is the only place the id formula lives.
  function favLocKey(lat: number, lon: number): string {
    return `${lat.toFixed(4)},${lon.toFixed(4)}`;
  }
  function persistLocationFavorites() {
    try {
      localStorage.setItem(
        LOC_FAV_KEY,
        JSON.stringify(locationFavorites.value),
      );
    } catch {
      // ignore quota / private-mode errors
    }
  }
  function isLocationFavorite(lat: number, lon: number): boolean {
    const id = favLocKey(lat, lon);
    return locationFavorites.value.some((f) => f.id === id);
  }
  // toggleLocationFavorite saves the site, or removes it when the same coordinates are already saved.
  function toggleLocationFavorite(fav: Omit<LocationFavorite, "id">) {
    const id = favLocKey(fav.lat, fav.lon);
    const i = locationFavorites.value.findIndex((f) => f.id === id);
    if (i >= 0) locationFavorites.value.splice(i, 1);
    else locationFavorites.value.push({ ...fav, id });
    persistLocationFavorites();
  }
  function removeLocationFavorite(id: string) {
    locationFavorites.value = locationFavorites.value.filter(
      (f) => f.id !== id,
    );
    persistLocationFavorites();
  }
  function renameLocationFavorite(id: string, label: string) {
    const fav = locationFavorites.value.find((f) => f.id === id);
    if (!fav) return;
    fav.label = label;
    persistLocationFavorites();
  }

  return {
    query,
    params,
    setObserver,
    locationFavorites,
    favLocKey,
    isLocationFavorite,
    toggleLocationFavorite,
    removeLocationFavorite,
    renameLocationFavorite,
    equipmentSetups,
  };
});
