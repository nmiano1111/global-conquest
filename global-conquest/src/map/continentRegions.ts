import { TERRITORY_MAX_EXTENT } from "./nodeMetrics";

// Continent regions for custom maps, computed from territory positions
// alone. Every territory gets a Voronoi cell (the area closer to it than to
// any other site); invisible "sea" sites scattered through the empty parts
// of the map cap each cell at a coastline distance. A continent's outline
// is then every cell edge it shares with the sea, the map edge, or a
// different continent -- so outlines follow each continent's real shape,
// and two continents that sit close together share a single border line
// instead of overlapping.

export type Pt = { x: number; y: number };
export type Segment = { a: Pt; b: Pt };
export type ContinentRegions = {
  /** Each territory's region polygon, keyed by territory name. */
  cells: Map<string, Pt[]>;
  /** Region polygons grouped by continent name. */
  cellsByContinent: Map<string, Pt[][]>;
  /** Continent outline pieces: coastlines plus continent-to-continent borders (each once). */
  borders: Segment[];
};

/** Median distance from each territory to its nearest neighbor. */
function medianNearestDistance(points: Pt[]): number {
  if (points.length < 2) return Infinity;
  const nearest = points.map((p, i) => {
    let best = Infinity;
    points.forEach((q, j) => {
      if (i !== j) best = Math.min(best, Math.hypot(p.x - q.x, p.y - q.y));
    });
    return best;
  });
  nearest.sort((a, b) => a - b);
  return nearest[Math.floor(nearest.length / 2)];
}

/**
 * How far a territory's region may reach toward empty space: a little
 * over the typical spacing, so the gaps between a continent's territories
 * fill in and it reads as one solid land mass rather than a ring of
 * blobs, but always clear of the node's own drawn extent.
 */
export function coastReach(points: Pt[]): number {
  const spacing = medianNearestDistance(points);
  const reach = Number.isFinite(spacing) ? spacing * 1.15 : 200;
  return Math.min(280, Math.max(TERRITORY_MAX_EXTENT + 24, reach));
}

/** Sea sites on an offset grid, kept only where no territory is within reach. */
function seaSites(points: Pt[], width: number, height: number, reach: number): Pt[] {
  const step = reach * 0.45;
  const rowHeight = step * 0.866;
  const out: Pt[] = [];
  for (let row = 0, y = -rowHeight; y <= height + rowHeight; row++, y += rowHeight) {
    for (let x = row % 2 ? -step / 2 : -step; x <= width + step; x += step) {
      if (points.every((p) => Math.hypot(p.x - x, p.y - y) > reach)) out.push({ x, y });
    }
  }
  return out;
}

type Edge = { a: Pt; b: Pt; neighbor: number }; // neighbor: site index, or -1 for the map edge

/**
 * Clips a convex polygon (with per-edge neighbor tags) to the half-plane
 * of points at least as close to `site` as to `other`, tagging the new
 * edge along the bisector with `otherIndex`.
 */
function clipToBisector(poly: Edge[], site: Pt, other: Pt, otherIndex: number): Edge[] {
  // Keep points p with (p - mid) . (other - site) <= 0.
  const nx = other.x - site.x;
  const ny = other.y - site.y;
  const mx = (site.x + other.x) / 2;
  const my = (site.y + other.y) / 2;
  const side = (p: Pt) => (p.x - mx) * nx + (p.y - my) * ny;
  const cross = (a: Pt, b: Pt): Pt => {
    const sa = side(a);
    const t = sa / (sa - side(b));
    return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
  };

  const out: Edge[] = [];
  let entry: Pt | null = null;
  let exit: Pt | null = null;
  for (const e of poly) {
    const inA = side(e.a) <= 0;
    const inB = side(e.b) <= 0;
    if (inA && inB) {
      out.push(e);
    } else if (inA && !inB) {
      exit = cross(e.a, e.b);
      out.push({ a: e.a, b: exit, neighbor: e.neighbor });
    } else if (!inA && inB) {
      entry = cross(e.a, e.b);
      out.push({ a: entry, b: e.b, neighbor: e.neighbor });
    }
  }
  if (!entry || !exit) return out;
  // Close the gap along the bisector, from where the boundary left the
  // half-plane to where it came back in.
  const at = out.findIndex((e) => e.a === entry);
  out.splice(at, 0, { a: exit, b: entry, neighbor: otherIndex });
  return out;
}

export function computeContinentRegions(
  positions: Map<string, Pt>,
  continentOf: Map<string, string>,
  width: number,
  height: number,
): ContinentRegions {
  const names = [...positions.keys()].filter((n) => continentOf.has(n));
  const land = names.map((n) => positions.get(n)!);
  const reach = coastReach(land);
  const sites = [...land, ...seaSites(land, width, height, reach)];
  // Site index -> continent name; sea sites have none.
  const owner = (i: number) => (i >= 0 && i < names.length ? continentOf.get(names[i])! : null);

  const corners = [
    { x: 0, y: 0 },
    { x: width, y: 0 },
    { x: width, y: height },
    { x: 0, y: height },
  ];
  const cells = new Map<string, Pt[]>();
  const cellsByContinent = new Map<string, Pt[][]>();
  const borders: Segment[] = [];

  names.forEach((name, i) => {
    let poly: Edge[] = corners.map((a, k) => ({ a, b: corners[(k + 1) % 4], neighbor: -1 }));
    const site = sites[i];
    // A cell never extends much past the reach (sea sites fill the space
    // beyond it), so only sites within a few reaches can bound it.
    sites.forEach((other, j) => {
      if (j !== i && poly.length > 0 && Math.hypot(other.x - site.x, other.y - site.y) < reach * 3.5) {
        poly = clipToBisector(poly, site, other, j);
      }
    });
    const polygon = poly.map((e) => e.a);
    cells.set(name, polygon);
    const continent = owner(i)!;
    cellsByContinent.set(continent, [...(cellsByContinent.get(continent) ?? []), polygon]);

    for (const e of poly) {
      const other = owner(e.neighbor);
      if (other === continent) continue; // inside the continent: no line
      // A border between two continents is shared by both cells; draw it
      // from the lower-indexed side only.
      if (other !== null && e.neighbor < i) continue;
      borders.push({ a: e.a, b: e.b });
    }
  });

  return { cells, cellsByContinent, borders };
}
