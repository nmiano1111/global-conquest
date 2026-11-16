import { describe, expect, it } from "vitest";
import { coastReach, computeContinentRegions, type Pt } from "./continentRegions";
import { TERRITORY_MAX_EXTENT } from "./nodeMetrics";

const W = 2048;
const H = 1367;

function pointInPolygon(p: Pt, poly: Pt[]): boolean {
  let inside = false;
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const a = poly[i];
    const b = poly[j];
    if (a.y > p.y !== b.y > p.y && p.x < ((b.x - a.x) * (p.y - a.y)) / (b.y - a.y) + a.x) inside = !inside;
  }
  return inside;
}

function distToSegment(p: Pt, a: Pt, b: Pt): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const l2 = dx * dx + dy * dy;
  const t = l2 === 0 ? 0 : Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / l2));
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}

// Two continents of four territories each: West and East, close enough to share a border.
function sample() {
  const positions = new Map<string, Pt>([
    ["w1", { x: 500, y: 500 }],
    ["w2", { x: 700, y: 500 }],
    ["w3", { x: 500, y: 700 }],
    ["w4", { x: 700, y: 700 }],
    ["e1", { x: 900, y: 500 }],
    ["e2", { x: 1100, y: 500 }],
    ["e3", { x: 900, y: 700 }],
    ["e4", { x: 1100, y: 700 }],
  ]);
  const continentOf = new Map([...positions.keys()].map((n) => [n, n.startsWith("w") ? "West" : "East"]));
  return { positions, continentOf, regions: computeContinentRegions(positions, continentOf, W, H) };
}

describe("computeContinentRegions", () => {
  it("gives each territory a region containing it, clear of its drawn node, and no one else's", () => {
    const { positions, regions } = sample();
    for (const [name, cell] of regions.cells) {
      const own = positions.get(name)!;
      expect(pointInPolygon(own, cell)).toBe(true);
      for (let i = 0; i < cell.length; i++) {
        expect(distToSegment(own, cell[i], cell[(i + 1) % cell.length])).toBeGreaterThanOrEqual(TERRITORY_MAX_EXTENT);
      }
      for (const [other, p] of positions) {
        if (other !== name) expect(pointInPolygon(p, cell)).toBe(false);
      }
    }
  });

  it("draws a border between neighboring continents and a coastline, but none inside a continent", () => {
    const { positions, regions } = sample();
    const midpoint = (s: { a: Pt; b: Pt }) => ({ x: (s.a.x + s.b.x) / 2, y: (s.a.y + s.b.y) / 2 });
    // The West/East border runs along x = 800 between the two clusters.
    expect(regions.borders.some((s) => Math.abs(s.a.x - 800) < 1 && Math.abs(s.b.x - 800) < 1)).toBe(true);
    // Nothing is drawn between w1 and w2 (same continent): no border near their midpoint.
    const inner = { x: 600, y: 500 };
    expect(regions.borders.every((s) => distToSegment(inner, s.a, s.b) > 20)).toBe(true);
    // Coastline exists: border pieces well outside the clusters.
    expect(regions.borders.some((s) => midpoint(s).x < 500 - TERRITORY_MAX_EXTENT)).toBe(true);
    expect(positions.size).toBe(regions.cells.size);
  });

  it("groups regions by continent", () => {
    const { regions } = sample();
    expect(regions.cellsByContinent.get("West")).toHaveLength(4);
    expect(regions.cellsByContinent.get("East")).toHaveLength(4);
  });

  it("keeps the coast reach clear of the node even for tightly packed maps", () => {
    expect(coastReach([{ x: 0, y: 0 }, { x: 10, y: 0 }])).toBeGreaterThan(TERRITORY_MAX_EXTENT);
  });
});
