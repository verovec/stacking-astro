import { describe, it, expect } from "vitest";
import { mount } from "@vue/test-utils";
import { createTestingPinia } from "@pinia/testing";
import { vi } from "vitest";
import RunResultPanels from "./RunResultPanels.vue";
import { testI18n } from "@/test/i18n";
import type { RunResult, StarTier } from "@/types";

const OUT = "/out/M31";

// A finished deep-sky run: the colour final plus the star-presence set written beside it.
function runResult(starTiers?: StarTier[]): RunResult {
  return {
    input_dir: "/in/M31",
    output_dir: OUT,
    masters: [],
    channels: [],
    warnings: [],
    final: {
      mode: "LRGB",
      channels: ["L", "R", "G", "B"],
      outputs: [
        `${OUT}/final.xcf`,
        `${OUT}/final.tif`,
        `${OUT}/final.png`,
        ...(starTiers ?? []).filter((t) => t.percent !== 100).map((t) => t.png),
      ],
      star_tiers: starTiers,
    },
  } as RunResult;
}

const TIERS: StarTier[] = [
  {
    kind: "starless",
    percent: 0,
    png: `${OUT}/final-starless.png`,
    tif: `${OUT}/final-starless.tif`,
  },
  { kind: "stars", percent: 25, png: `${OUT}/final-25-stars.png` },
  { kind: "stars", percent: 50, png: `${OUT}/final-50-stars.png` },
  { kind: "stars", percent: 75, png: `${OUT}/final-75-stars.png` },
  { kind: "final", percent: 100, png: `${OUT}/final.png` },
];

function mountIt(result: RunResult) {
  return mount(RunResultPanels, {
    props: { result },
    global: {
      plugins: [testI18n(), createTestingPinia({ createSpy: vi.fn })],
      stubs: { StarField3D: true, MetricsChart: true },
    },
  });
}

// The button whose label matches, among the view-switcher buttons.
const buttonByText = (w: ReturnType<typeof mountIt>, label: string) =>
  w.findAll("button").find((b) => b.text() === label);

describe("RunResultPanels star tiers", () => {
  it("offers one switcher entry per star level", () => {
    const w = mountIt(runResult(TIERS));
    for (const label of ["Starless", "25% stars", "50% stars", "75% stars"]) {
      expect(buttonByText(w, label), `missing ${label}`).toBeTruthy();
    }
  });

  it("selecting a star level shows that render in the viewer", async () => {
    const w = mountIt(runResult(TIERS));
    await buttonByText(w, "50% stars")!.trigger("click");
    const img = w.find("img");
    expect(img.attributes("src")).toContain("final-50-stars.png");
  });

  it("the starless entry shows the starless render", async () => {
    const w = mountIt(runResult(TIERS));
    await buttonByText(w, "Starless")!.trigger("click");
    expect(w.find("img").attributes("src")).toContain("final-starless.png");
  });

  it("keeps the first-png viewer contract: the final is what opens", () => {
    const w = mountIt(runResult(TIERS));
    const src = w.find("img").attributes("src") ?? "";
    expect(src).toContain("final.png");
    expect(src).not.toContain("stars.png");
    expect(src).not.toContain("starless");
  });

  it("renders no star-level switcher when the run has no tiers", () => {
    const w = mountIt(runResult());
    expect(buttonByText(w, "Starless")).toBeUndefined();
    expect(buttonByText(w, "50% stars")).toBeUndefined();
  });
});
