import { describe, it, expect } from "vitest";
import { mount } from "@vue/test-utils";
import RigPanel from "./RigPanel.vue";
import { testI18n } from "@/test/i18n";
import type { EquipmentSetup } from "@/types";

// The three rigs this fork is actually used with — a refractor, a small apo and a camera lens. The
// spread is the point: 740 mm to 20 mm is a 37× scale range, which is why the engine's single
// configured rig cannot stand in for "the optics" of an arbitrary session.
const rigs: EquipmentSetup[] = [
  { id: "1", name: "FC-100 DF", focal_mm: 740, pixel_um: 3.8, eyepieces: [] },
  { id: "2", name: "RedCat 51", focal_mm: 250, pixel_um: 3.76, eyepieces: [] },
  { id: "3", name: "Nikkor 20mm", focal_mm: 20, pixel_um: 5.94, eyepieces: [] },
];

function mountPanel(props: Record<string, unknown> = {}) {
  return mount(RigPanel, {
    global: { plugins: [testI18n()] },
    props: { colorModel: "auto", ...props },
  });
}

describe("RigPanel colour model", () => {
  // The scan's verdict is the default answer, so a user who agrees with it does nothing and the run
  // stays byte-identical. It has to be legible in the control itself, not only in the summary card.
  it("names the detected verdict on the auto option", () => {
    const osc = mountPanel({ detected: "osc" });
    expect(osc.get('[data-test="color-model"]').text()).toContain("one-shot");

    const mono = mountPanel({ detected: "mono" });
    expect(mono.get('[data-test="color-model"]').text()).toContain(
      "monochrome",
    );
  });

  it("starts on auto so an untouched form asserts nothing", () => {
    const w = mountPanel({ detected: "osc" });
    const select = w.get('[data-test="color-model-select"]')
      .element as HTMLSelectElement;
    expect(select.value).toBe("auto");
  });

  it("emits the explicit choice when the user overrides the verdict", async () => {
    const w = mountPanel({ detected: "osc" });
    await w.get('[data-test="color-model-select"]').setValue("mono");
    expect(w.emitted("update:colorModel")).toEqual([["mono"]]);
  });

  // A mixed folder is the case the knob exists for: auto keeps the monochrome half, and the user
  // needs to be told that before launching rather than discovering it in the warnings afterwards.
  it("warns that auto silently drops half of a mixed folder", () => {
    const w = mountPanel({ detected: "mixed" });
    expect(w.get('[data-test="color-model"]').text().toLowerCase()).toContain(
      "mixed",
    );
  });
});

describe("RigPanel session optics", () => {
  // The trap this closes: omitted optics are NOT "unset", they silently become the engine's
  // configured telescope — which cannot plate-solve a camera-lens field at all.
  it("flags the engine default when the optics are empty", () => {
    const w = mountPanel();
    expect(w.find('[data-test="optics-warning"]').exists()).toBe(true);
  });

  it("drops the flag once the optics are given", () => {
    const w = mountPanel({ focalMm: 20, pixelUm: 5.94 });
    expect(w.find('[data-test="optics-warning"]').exists()).toBe(false);
  });

  it("still flags when only one of the two is given", () => {
    const w = mountPanel({ focalMm: 20 });
    expect(w.find('[data-test="optics-warning"]').exists()).toBe(true);
  });

  it("renders the optics prefilled", () => {
    const w = mountPanel({ focalMm: 20, pixelUm: 5.94 });
    expect(
      (w.get('[data-test="focal-mm"]').element as HTMLInputElement).value,
    ).toBe("20");
    expect(
      (w.get('[data-test="pixel-um"]').element as HTMLInputElement).value,
    ).toBe("5.94");
  });

  it("emits a number, not a string, when the user types a focal length", async () => {
    const w = mountPanel();
    await w.get('[data-test="focal-mm"]').setValue("135");
    expect(w.emitted("update:focalMm")).toEqual([[135]]);
  });

  it("emits undefined when the user clears a field, so no zero reaches the wire", async () => {
    const w = mountPanel({ focalMm: 135 });
    await w.get('[data-test="focal-mm"]').setValue("");
    expect(w.emitted("update:focalMm")).toEqual([[undefined]]);
  });

  // Prefill source: the user's saved rigs, which already live server-side (table equipment_setups)
  // so the desktop and the phone agree on the optics.
  it("fills both optics from a saved rig in one pick", async () => {
    const w = mountPanel({ rigs });
    await w.get('[data-test="rig-select"]').setValue("3");
    expect(w.emitted("update:focalMm")).toEqual([[20]]);
    expect(w.emitted("update:pixelUm")).toEqual([[5.94]]);
  });

  it("renders no rig picker when the user has saved none", () => {
    const w = mountPanel({ rigs: [] });
    expect(w.find('[data-test="rig-select"]').exists()).toBe(false);
  });
});
