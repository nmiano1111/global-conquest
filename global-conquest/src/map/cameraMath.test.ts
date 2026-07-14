import { describe, expect, it } from "vitest";
import {
  clampCameraPosition,
  computeCoverZoom,
  computeMinZoom,
  distance,
  exceedsDragThreshold,
  fitToViewport,
  isDoubleTap,
  zoomToward,
} from "./cameraMath";

const WORLD = { width: 2000, height: 1000 };

describe("computeMinZoom", () => {
  it("picks the width-constrained scale when the viewport is wider than it is tall relative to the world", () => {
    // Viewport is much wider relative to its height than the world is —
    // height is the binding constraint.
    const scale = computeMinZoom({ width: 4000, height: 500 }, WORLD);
    expect(scale).toBeCloseTo(0.5); // 500 / 1000
  });

  it("picks the height-constrained scale when the viewport is narrow", () => {
    const scale = computeMinZoom({ width: 200, height: 2000 }, WORLD);
    expect(scale).toBeCloseTo(0.1); // 200 / 2000
  });
});

describe("clampCameraPosition", () => {
  it("centers the world on an axis where it fits inside the viewport", () => {
    const next = clampCameraPosition({ scale: 0.1, x: 999, y: -999 }, { width: 1000, height: 1000 }, WORLD);
    // world is 200x100 at scale 0.1, viewport is 1000x1000 — fits both axes, so centered.
    expect(next.x).toBeCloseTo((1000 - 200) / 2);
    expect(next.y).toBeCloseTo((1000 - 100) / 2);
  });

  it("never lets the world's right/bottom edge retreat past the viewport's right/bottom edge", () => {
    // scale 1 world (2000x1000) inside a 500x500 viewport, camera dragged far right/down.
    const next = clampCameraPosition({ scale: 1, x: 5000, y: 5000 }, { width: 500, height: 500 }, WORLD);
    expect(next.x).toBe(0);
    expect(next.y).toBe(0);
  });

  it("never lets the world's left/top edge advance past the viewport's left/top edge", () => {
    const next = clampCameraPosition({ scale: 1, x: -9000, y: -9000 }, { width: 500, height: 500 }, WORLD);
    expect(next.x).toBe(500 - 2000);
    expect(next.y).toBe(500 - 1000);
  });
});

describe("computeCoverZoom", () => {
  it("picks the larger (cropping) scale — the opposite constraint from computeMinZoom", () => {
    // A tall narrow viewport against a wide short world: contain is
    // width-bound (small), cover is height-bound (large) — inverted here
    // since the viewport itself is narrow relative to the world's shape.
    const viewport = { width: 400, height: 1000 };
    expect(computeMinZoom(viewport, WORLD)).toBeCloseTo(0.2); // 400/2000
    expect(computeCoverZoom(viewport, WORLD)).toBeCloseTo(1); // 1000/1000
  });
});

describe("fitToViewport", () => {
  it("defaults to contain: scales and centers the world to exactly fit inside the viewport", () => {
    const cam = fitToViewport({ width: 1000, height: 1000 }, WORLD);
    expect(cam.scale).toBeCloseTo(0.5); // width/2000=0.5, height/1000=1 -> min is 0.5
    expect(cam.x).toBeCloseTo((1000 - WORLD.width * cam.scale) / 2);
    expect(cam.y).toBeCloseTo((1000 - WORLD.height * cam.scale) / 2);
  });

  it("mode: cover fills the viewport edge to edge, cropping the larger axis", () => {
    const viewport = { width: 1000, height: 1000 };
    const cam = fitToViewport(viewport, WORLD, "cover");
    expect(cam.scale).toBeCloseTo(1); // max(0.5, 1) = 1
    // At scale 1 the world (2000x1000) overflows the 1000-wide viewport —
    // centered, so x should be negative (world edges extend off-screen).
    expect(cam.x).toBeCloseTo((1000 - WORLD.width * 1) / 2);
    expect(cam.x).toBeLessThan(0);
    // Height fits exactly at scale 1, so y is 0 (no crop on that axis).
    expect(cam.y).toBeCloseTo(0);
  });
});

describe("zoomToward", () => {
  const viewport = { width: 800, height: 400 };

  it("keeps the world point under the pivot fixed after zooming", () => {
    const start = fitToViewport(viewport, WORLD);
    const pivotX = 400;
    const pivotY = 200;
    const worldXBefore = (pivotX - start.x) / start.scale;
    const worldYBefore = (pivotY - start.y) / start.scale;

    const next = zoomToward(start, start.scale * 2, pivotX, pivotY, 0.01, 10, viewport, WORLD);

    const worldXAfter = (pivotX - next.x) / next.scale;
    const worldYAfter = (pivotY - next.y) / next.scale;
    expect(worldXAfter).toBeCloseTo(worldXBefore, 5);
    expect(worldYAfter).toBeCloseTo(worldYBefore, 5);
  });

  it("clamps to maxZoom", () => {
    const start = fitToViewport(viewport, WORLD);
    const next = zoomToward(start, 999, 400, 200, 0.01, 3, viewport, WORLD);
    expect(next.scale).toBe(3);
  });

  it("clamps to minZoom", () => {
    const start = fitToViewport(viewport, WORLD);
    const next = zoomToward(start, 0.0001, 400, 200, 0.25, 10, viewport, WORLD);
    expect(next.scale).toBe(0.25);
  });
});

describe("distance", () => {
  it("computes euclidean distance", () => {
    expect(distance(0, 0, 3, 4)).toBe(5);
  });
});

describe("exceedsDragThreshold", () => {
  it("is false for small movements", () => {
    expect(exceedsDragThreshold(100, 100, 103, 102)).toBe(false);
  });

  it("is true once movement exceeds the threshold", () => {
    expect(exceedsDragThreshold(100, 100, 120, 100)).toBe(true);
  });

  it("respects a custom threshold", () => {
    expect(exceedsDragThreshold(0, 0, 10, 0, 20)).toBe(false);
    expect(exceedsDragThreshold(0, 0, 25, 0, 20)).toBe(true);
  });
});

describe("isDoubleTap", () => {
  it("is false when there is no previous tap", () => {
    expect(isDoubleTap(null, { x: 0, y: 0, timeMs: 0 })).toBe(false);
  });

  it("is true for two nearby taps within the time window", () => {
    const first = { x: 100, y: 100, timeMs: 1000 };
    const second = { x: 110, y: 95, timeMs: 1200 };
    expect(isDoubleTap(first, second)).toBe(true);
  });

  it("is false once the interval is too long", () => {
    const first = { x: 100, y: 100, timeMs: 1000 };
    const second = { x: 100, y: 100, timeMs: 1500 };
    expect(isDoubleTap(first, second, 300)).toBe(false);
  });

  it("is false once the taps are too far apart", () => {
    const first = { x: 100, y: 100, timeMs: 1000 };
    const second = { x: 300, y: 100, timeMs: 1100 };
    expect(isDoubleTap(first, second, 300, 32)).toBe(false);
  });
});
