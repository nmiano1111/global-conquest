import { TERRITORY_MAX_EXTENT } from "./nodeMetrics";

/** Clear space kept between any territory's drawn edge and the map's edge. */
const EDGE_BREATHING_ROOM = 32;
export const CUSTOM_MAP_MARGIN = TERRITORY_MAX_EXTENT + EDGE_BREATHING_ROOM;

/**
 * Places a custom map's normalized layout in world space. The layout's own
 * bounding box is stretched to fill the map minus CUSTOM_MAP_MARGIN on
 * every side, so no territory (or its highlight ring) touches the map's
 * edge however close to 0 or 1 the stored coordinates are. An axis with no
 * spread (e.g. every territory in one row) is centered.
 */
export function customLayoutToWorld(
  territories: Record<string, { x: number; y: number }>,
  width: number,
  height: number,
): Map<string, { x: number; y: number }> {
  const entries = Object.entries(territories);
  const xs = entries.map(([, p]) => p.x);
  const ys = entries.map(([, p]) => p.y);
  const place = (v: number, lo: number, hi: number, size: number) => {
    const inner = size - 2 * CUSTOM_MAP_MARGIN;
    return hi > lo ? CUSTOM_MAP_MARGIN + ((v - lo) / (hi - lo)) * inner : size / 2;
  };
  const [minX, maxX, minY, maxY] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)];
  return new Map(
    entries.map(([name, p]) => [name, { x: place(p.x, minX, maxX, width), y: place(p.y, minY, maxY, height) }]),
  );
}
