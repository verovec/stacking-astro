import { describe, it, expect } from "vitest";
import { knobsForStage } from "./knobs";
import en from "@/i18n/en.json";
import fr from "@/i18n/fr.json";

// Both knobs act at composite time, so they belong to the "final" group — the same re-entry tier
// the engine classifies them as (Tier A). Reading them through the public accessor also pins that.
const byKey = (k: string) => knobsForStage("final").find((n) => n.key === k);

// The two narrowband knobs card 0008 adds. Their bounds are not cosmetic: they reach GIMP as a layer
// opacity and an explicit curve, and the engine clamps to exactly these ranges — a UI that let a user
// ask for something outside them would silently disagree with the picture it produced.
describe("narrowband blend knobs", () => {
  it("nb_blend is a 0..1 weight defaulting to full", () => {
    const k = byKey("nb_blend");
    expect(k).toBeDefined();
    expect(k!.min).toBe(0);
    expect(k!.max).toBe(1);
    // Full by default: a run that never touches the slider must composite exactly as before.
    expect(k!.def).toBe(1);
  });

  it("oiii_boost starts at 1 — the boost may only ADD light", () => {
    const k = byKey("oiii_boost");
    expect(k).toBeDefined();
    // Below 1 would turn the anti-clipping shoulder into an attenuator, which is oiii_screen's job.
    expect(k!.min).toBe(1);
    expect(k!.max).toBe(1.6); // the runbook's over-cooked end
    expect(k!.def).toBe(1); // off
  });

  it.each(["nb_blend", "oiii_boost"])(
    "%s has a label and a glossary entry in BOTH locales",
    (key) => {
      for (const [name, msgs] of [
        ["en", en],
        ["fr", fr],
      ] as const) {
        const labels = (msgs as any).rerun.knobs;
        expect(labels[key], `${name} label for ${key}`).toBeTruthy();
        // The glossary block is the one carrying the long-form oiii_screen text.
        const glossary = Object.values(msgs as any).find(
          (v: any) =>
            v &&
            typeof v === "object" &&
            typeof v.oiii_screen === "string" &&
            v.oiii_screen.length > 80,
        ) as Record<string, string>;
        expect(glossary[key], `${name} glossary for ${key}`).toBeTruthy();
        expect(glossary[key].length).toBeGreaterThan(80);
      }
    },
  );
});
