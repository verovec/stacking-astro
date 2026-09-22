import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";

const apiPost = vi.fn(async (..._args: unknown[]) => ({ id: 42 }));
vi.mock("@/services/api", () => ({
  ApiError: class extends Error {},
  apiGet: vi.fn(async () => ({})),
  apiPost: (...args: unknown[]) => apiPost(...args),
  health: vi.fn(async () => true),
}));

import { useJobsStore } from "./jobs";

describe("jobs store create body", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    apiPost.mockClear();
  });

  it("sends the target hint when provided", async () => {
    const store = useJobsStore();
    await store.create("input/triplet_m66/CapObj", "deepsky", "image", {
      target: "M66",
    });
    expect(apiPost).toHaveBeenCalledWith(
      "/api/jobs",
      expect.objectContaining({ target: "M66" }),
    );
  });

  it("omits target entirely when unset", async () => {
    const store = useJobsStore();
    await store.create("input/triplet_m66/CapObj", "deepsky", "image", {});
    const body = apiPost.mock.calls[0][1] as Record<string, unknown>;
    expect(body).not.toHaveProperty("target");
  });

  // The colour knob is an OVERRIDE: "auto" is what the engine does without it, so sending it would
  // put a value on every stored job for no behaviour change. Only an explicit answer travels.
  it.each(["mono", "osc"])(
    "sends an explicit color_model=%s",
    async (choice) => {
      const store = useJobsStore();
      await store.create("input/M31/2024-01-01", "deepsky", "image", {
        colorModel: choice as "mono" | "osc",
      });
      expect(apiPost).toHaveBeenCalledWith(
        "/api/jobs",
        expect.objectContaining({ color_model: choice }),
      );
    },
  );

  it.each(["auto", undefined])(
    "omits color_model when the user left it at %s",
    async (choice) => {
      const store = useJobsStore();
      await store.create("input/M31/2024-01-01", "deepsky", "image", {
        colorModel: choice as "auto" | undefined,
      });
      const body = apiPost.mock.calls[0][1] as Record<string, unknown>;
      expect(body).not.toHaveProperty("color_model");
    },
  );

  // focal_mm / pixel_um were already RunRequest fields but reachable only through the advanced JSON.
  // Without them the engine silently plate-solves at its configured rig's scale, which cannot solve a
  // camera-lens session at all — so they have to survive the store as real numbers.
  it("sends the session optics when given", async () => {
    const store = useJobsStore();
    await store.create("input/M31/large-field", "deepsky", "image", {
      focalMm: 20,
      pixelUm: 5.94,
    });
    expect(apiPost).toHaveBeenCalledWith(
      "/api/jobs",
      expect.objectContaining({ focal_mm: 20, pixel_um: 5.94 }),
    );
  });

  it("omits the optics when left empty, and never sends a zero", async () => {
    const store = useJobsStore();
    await store.create("input/M31/large-field", "deepsky", "image", {
      focalMm: 0,
      pixelUm: undefined,
    });
    const body = apiPost.mock.calls[0][1] as Record<string, unknown>;
    expect(body).not.toHaveProperty("focal_mm");
    expect(body).not.toHaveProperty("pixel_um");
  });
});
