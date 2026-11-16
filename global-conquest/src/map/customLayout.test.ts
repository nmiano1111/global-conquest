import { describe, expect, it } from "vitest";
import { CUSTOM_MAP_MARGIN, customLayoutToWorld } from "./customLayout";
import { TERRITORY_MAX_EXTENT } from "./nodeMetrics";

describe("customLayoutToWorld", () => {
  const W = 2048;
  const H = 1367;

  it("keeps every territory's full drawn extent inside the map", () => {
    const out = customLayoutToWorld({ a: { x: 0, y: 0 }, b: { x: 1, y: 1 }, c: { x: 0.03, y: 0.97 }, d: { x: 0.5, y: 0.5 } }, W, H);
    for (const { x, y } of out.values()) {
      expect(x - TERRITORY_MAX_EXTENT).toBeGreaterThan(0);
      expect(y - TERRITORY_MAX_EXTENT).toBeGreaterThan(0);
      expect(x + TERRITORY_MAX_EXTENT).toBeLessThan(W);
      expect(y + TERRITORY_MAX_EXTENT).toBeLessThan(H);
    }
  });

  it("stretches the layout's bounding box to the margins", () => {
    const out = customLayoutToWorld({ a: { x: 0.2, y: 0.4 }, b: { x: 0.6, y: 0.5 } }, W, H);
    expect(out.get("a")).toEqual({ x: CUSTOM_MAP_MARGIN, y: CUSTOM_MAP_MARGIN });
    expect(out.get("b")).toEqual({ x: W - CUSTOM_MAP_MARGIN, y: H - CUSTOM_MAP_MARGIN });
  });

  it("centers an axis with no spread", () => {
    const out = customLayoutToWorld({ a: { x: 0.1, y: 0.5 }, b: { x: 0.9, y: 0.5 } }, W, H);
    expect(out.get("a")!.y).toBe(H / 2);
    expect(out.get("b")!.y).toBe(H / 2);
    expect(customLayoutToWorld({ solo: { x: 0.3, y: 0.3 } }, W, H).get("solo")).toEqual({ x: W / 2, y: H / 2 });
  });
});
