import { describe, it, expect } from "vitest";
import { mount } from "@vue/test-utils";
import ObjectTypeChips from "./ObjectTypeChips.vue";
import { testI18n } from "@/test/i18n";

const types = ["galaxy", "emission-nebula", "dark-nebula", "comet"];

function mountChips(props: Record<string, unknown> = {}) {
  return mount(ObjectTypeChips, {
    global: { plugins: [testI18n()] },
    props: { types, selected: [], ...props },
  });
}

describe("ObjectTypeChips", () => {
  it("renders one chip per served type, in the served order", () => {
    const w = mountChips();
    const chips = w.findAll('[data-test="object-chip"]');
    expect(chips.map((c) => c.attributes("data-type"))).toEqual(types);
  });

  it("renders the translated label, not the slug", () => {
    const w = mountChips();
    const galaxy = w.get('[data-test="object-chip"][data-type="galaxy"]');
    expect(galaxy.text()).toBe("Galaxy");
  });

  // Forward compatibility: a newer engine may serve a type this UI has no string for. Showing the
  // raw slug is honest; showing "preset.objects.whatever" is a bug the user reports as garbled text.
  it("falls back to the slug for an unknown type", () => {
    const w = mountChips({ types: ["brand-new-thing"] });
    expect(w.get('[data-test="object-chip"]').text()).toBe("brand-new-thing");
  });

  it("emits the type added on click", async () => {
    const w = mountChips();
    await w
      .get('[data-test="object-chip"][data-type="comet"]')
      .trigger("click");
    expect(w.emitted("update:selected")).toEqual([[["comet"]]]);
  });

  it("adds to the existing selection rather than replacing it", async () => {
    const w = mountChips({ selected: ["galaxy"] });
    await w
      .get('[data-test="object-chip"][data-type="comet"]')
      .trigger("click");
    expect(w.emitted("update:selected")).toEqual([[["galaxy", "comet"]]]);
  });

  it("toggles a selected chip back off", async () => {
    const w = mountChips({ selected: ["galaxy", "comet"] });
    await w
      .get('[data-test="object-chip"][data-type="galaxy"]')
      .trigger("click");
    expect(w.emitted("update:selected")).toEqual([[["comet"]]]);
  });

  it("marks the selected chips as pressed for assistive tech", () => {
    const w = mountChips({ selected: ["galaxy"] });
    const galaxy = w.get('[data-test="object-chip"][data-type="galaxy"]');
    const comet = w.get('[data-test="object-chip"][data-type="comet"]');
    expect(galaxy.attributes("aria-pressed")).toBe("true");
    expect(comet.attributes("aria-pressed")).toBe("false");
  });

  it("offers a clear only once something is selected", async () => {
    expect(mountChips().find('[data-test="object-chips-clear"]').exists()).toBe(
      false,
    );

    const w = mountChips({ selected: ["galaxy"] });
    await w.get('[data-test="object-chips-clear"]').trigger("click");
    expect(w.emitted("update:selected")).toEqual([[[]]]);
  });

  // An engine that serves no taxonomy (older build) must not leave an empty labelled row behind.
  it("renders nothing at all when no types are served", () => {
    const w = mountChips({ types: [] });
    expect(w.find('[data-test="object-chips"]').exists()).toBe(false);
  });
});
