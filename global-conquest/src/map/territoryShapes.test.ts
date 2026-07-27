import { describe, expect, it } from "vitest";
import { MAP_TERRITORIES } from "../router/pages/gameShared";
import {
  denormalizePoint,
  denormalizePolygons,
  MAP_SOURCE_HEIGHT,
  MAP_SOURCE_WIDTH,
  normalizePoint,
  territoryShapes,
} from "./territoryShapes";

describe("normalizePoint / denormalizePoint", () => {
  it("round-trips a point through normalize then denormalize", () => {
    const original = { x: 512, y: 683.5 };
    const roundTripped = denormalizePoint(normalizePoint(original));
    expect(roundTripped.x).toBeCloseTo(original.x);
    expect(roundTripped.y).toBeCloseTo(original.y);
  });

  it("denormalizes (0,0) and (1,1) to the map's top-left and bottom-right pixels", () => {
    expect(denormalizePoint({ x: 0, y: 0 })).toEqual({ x: 0, y: 0 });
    expect(denormalizePoint({ x: 1, y: 1 })).toEqual({ x: MAP_SOURCE_WIDTH, y: MAP_SOURCE_HEIGHT });
  });
});

describe("denormalizePolygons", () => {
  it("preserves the number of polygons and points per polygon", () => {
    const polygons = [
      [
        { x: 0, y: 0 },
        { x: 0.5, y: 0.5 },
        { x: 1, y: 0 },
      ],
      [
        { x: 0.1, y: 0.1 },
        { x: 0.2, y: 0.2 },
      ],
    ];
    const result = denormalizePolygons(polygons);
    expect(result).toHaveLength(2);
    expect(result[0]).toHaveLength(3);
    expect(result[1]).toHaveLength(2);
  });
});

describe("territoryShapes sample data", () => {
  it("only defines shapes for real territory names from MAP_TERRITORIES", () => {
    for (const name of Object.keys(territoryShapes)) {
      expect(MAP_TERRITORIES).toHaveProperty(name);
    }
  });

  it("gives every shape a territoryId matching its key", () => {
    for (const [name, shape] of Object.entries(territoryShapes)) {
      expect(shape.territoryId).toBe(name);
    }
  });

  it("gives every polygon at least 3 points", () => {
    for (const shape of Object.values(territoryShapes)) {
      for (const polygon of shape.polygons) {
        expect(polygon.length).toBeGreaterThanOrEqual(3);
      }
    }
  });

  it("keeps all coordinates within the normalized 0..1 range", () => {
    for (const shape of Object.values(territoryShapes)) {
      for (const polygon of shape.polygons) {
        for (const point of polygon) {
          expect(point.x).toBeGreaterThanOrEqual(0);
          expect(point.x).toBeLessThanOrEqual(1);
          expect(point.y).toBeGreaterThanOrEqual(0);
          expect(point.y).toBeLessThanOrEqual(1);
        }
      }
      expect(shape.labelPosition.x).toBeGreaterThanOrEqual(0);
      expect(shape.labelPosition.x).toBeLessThanOrEqual(1);
      expect(shape.labelPosition.y).toBeGreaterThanOrEqual(0);
      expect(shape.labelPosition.y).toBeLessThanOrEqual(1);
    }
  });

  it("includes at least one multi-polygon (islands) example", () => {
    const multiPolygon = Object.values(territoryShapes).find((shape) => shape.polygons.length > 1);
    expect(multiPolygon).toBeDefined();
  });
});
