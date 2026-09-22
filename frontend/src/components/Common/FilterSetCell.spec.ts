import { describe, it, expect } from "vitest";
import { mount } from "@vue/test-utils";
import FilterSetCell from "./FilterSetCell.vue";
import { testI18n } from "@/test/i18n";

function mountCell(props: Record<string, unknown> = {}) {
  return mount(FilterSetCell, {
    global: { plugins: [testI18n()] },
    props: { detected: "", osc: true, ...props },
  });
}

describe("FilterSetCell", () => {
  it("shows the measured verdict", () => {
    expect(
      mountCell({ detected: "dualband" })
        .get('[data-test="filter-set-badge"]')
        .text(),
    ).toContain("Dual-band");
    expect(
      mountCell({ detected: "broadband" })
        .get('[data-test="filter-set-badge"]')
        .text(),
    ).toContain("Broadband");
  });

  // Unknown is a first-class answer: the classifier needs two independent signals to agree and
  // declines otherwise. Rendering it as a badge would dress a refusal up as a measurement.
  it("renders unknown as plain text, not as a verdict badge", () => {
    const w = mountCell({ detected: "unknown" });
    expect(w.find('[data-test="filter-set-badge"]').exists()).toBe(false);
    expect(w.get('[data-test="filter-set-unknown"]').text()).toBe("unknown");
  });

  it("treats an unmeasured set the same as unknown", () => {
    expect(
      mountCell({ detected: "" })
        .find('[data-test="filter-set-unknown"]')
        .exists(),
    ).toBe(true);
  });

  it("lets the override win over the measurement", () => {
    const w = mountCell({ detected: "broadband", override: "dualband" });
    expect(w.get('[data-test="filter-set-badge"]').text()).toContain(
      "Dual-band",
    );
  });

  // Measured and asserted look identical downstream but mean very different things when a run comes
  // out wrong, so an override has to be visibly distinct.
  it("marks an overridden verdict as the user's, not the engine's", () => {
    const measured = mountCell({ detected: "dualband" });
    expect(
      measured.find('[data-test="filter-set-override-mark"]').exists(),
    ).toBe(false);

    const asserted = mountCell({ detected: "broadband", override: "dualband" });
    expect(
      asserted.find('[data-test="filter-set-override-mark"]').exists(),
    ).toBe(true);
  });

  it("offers auto plus the two real sets — never 'unknown' as a choice", () => {
    const opts = mountCell()
      .get('[data-test="filter-set-select"]')
      .findAll("option")
      .map((o) => o.attributes("value"));
    expect(opts).toEqual(["", "broadband", "dualband"]);
  });

  it("emits the chosen set", async () => {
    const w = mountCell({ detected: "broadband" });
    await w.get('[data-test="filter-set-select"]').setValue("dualband");
    expect(w.emitted("update:override")).toEqual([["dualband"]]);
  });

  it("emits an empty value to hand the decision back to the engine", async () => {
    const w = mountCell({ detected: "broadband", override: "dualband" });
    await w.get('[data-test="filter-set-select"]').setValue("");
    expect(w.emitted("update:override")).toEqual([[""]]);
  });

  // A monochrome rig has a filter wheel and a FILTER card; the whole concept is meaningless there,
  // and offering an override would invite a nonsense assertion.
  it("renders nothing actionable on a monochrome scan", () => {
    const w = mountCell({ detected: "", osc: false });
    expect(w.find('[data-test="filter-set"]').exists()).toBe(false);
    expect(w.find('[data-test="filter-set-select"]').exists()).toBe(false);
    expect(w.text()).toBe("—");
  });
});
