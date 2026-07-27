import { Graphics } from "pixi.js";
import { denormalizePoint, normalizePoint, territoryShapes, type Point, type TerritoryShape } from "./territoryShapes";

export interface MapEditorSnapshot {
  territory: string | null;
  polygonCount: number;
  currentPolygonPointCount: number;
  hasLabel: boolean;
  labelMode: boolean;
}

const VERTEX_RADIUS = 5;

/**
 * Dev-only Pixi-side polygon tracing tool (see MAP_EDITOR.md). Owns a single
 * Graphics layer drawn on top of everything else, directly in world space —
 * the same pixel space territoryShapes.ts coordinates denormalize into, so
 * no camera/overlay transform juggling is needed while tracing.
 *
 * Not wired into any production code path: only ever constructed by
 * MapScene.setEditorMode(true), which GameMap.tsx only calls when both
 * import.meta.env.DEV and ?mapEditor=true hold.
 */
export class MapEditorController {
  private readonly gfx: Graphics;
  private readonly onChange: (snapshot: MapEditorSnapshot) => void;

  /** Working copy of all shapes, seeded from committed territoryShapes.ts; edits apply here until exported. */
  private readonly shapes: Record<string, TerritoryShape>;

  private territory: string | null = null;
  private polygons: Point[][] = [];
  private label: Point | null = null;
  private labelMode = false;

  private dragging: { polygonIndex: number; pointIndex: number } | null = null;

  constructor(gfx: Graphics, onChange: (snapshot: MapEditorSnapshot) => void) {
    this.gfx = gfx;
    this.onChange = onChange;
    this.shapes = structuredClone(territoryShapes);
  }

  selectTerritory(name: string) {
    this.commitCurrentShape();
    const existing = this.shapes[name];
    this.territory = name;
    this.polygons = existing ? existing.polygons.map((poly) => poly.map(denormalizePoint)) : [[]];
    this.label = existing ? denormalizePoint(existing.labelPosition) : null;
    this.labelMode = false;
    this.redraw();
  }

  /** Adds a vertex to the current (last) polygon of the active territory, or sets the label position if labelMode is on. */
  handleTap(point: Point) {
    if (!this.territory) return;
    if (this.labelMode) {
      this.label = point;
      this.labelMode = false;
      this.redraw();
      return;
    }
    if (this.polygons.length === 0) this.polygons.push([]);
    this.polygons[this.polygons.length - 1].push(point);
    this.redraw();
  }

  /** Closes off the current polygon and begins a new one — used for islands / disconnected landmasses. */
  startNewPolygon() {
    if (!this.territory) return;
    if (this.polygons[this.polygons.length - 1]?.length) {
      this.polygons.push([]);
      this.redraw();
    }
  }

  undoPoint() {
    if (!this.territory) return;
    const last = this.polygons[this.polygons.length - 1];
    if (last?.length) {
      last.pop();
    } else if (this.polygons.length > 1) {
      this.polygons.pop();
    }
    this.redraw();
  }

  clearCurrentTerritory() {
    if (!this.territory) return;
    this.polygons = [[]];
    this.label = null;
    this.redraw();
  }

  setLabelMode(enabled: boolean) {
    this.labelMode = enabled;
    this.redraw();
  }

  /** Starts dragging the nearest vertex of the active territory within `radius` world px, if any. Returns true if a drag started (caller should suppress panning for this gesture). */
  tryStartDrag(point: Point, radius: number): boolean {
    if (!this.territory) return false;
    for (let pi = 0; pi < this.polygons.length; pi++) {
      const poly = this.polygons[pi];
      for (let vi = 0; vi < poly.length; vi++) {
        const v = poly[vi];
        if (Math.hypot(v.x - point.x, v.y - point.y) <= radius) {
          this.dragging = { polygonIndex: pi, pointIndex: vi };
          return true;
        }
      }
    }
    return false;
  }

  updateDrag(point: Point) {
    if (!this.dragging) return;
    this.polygons[this.dragging.polygonIndex][this.dragging.pointIndex] = point;
    this.redraw();
  }

  endDrag() {
    this.dragging = null;
  }

  private commitCurrentShape() {
    if (!this.territory) return;
    const cleanPolygons = this.polygons.filter((p) => p.length >= 3).map((p) => p.map(normalizePoint));
    if (cleanPolygons.length === 0 || !this.label) return;
    this.shapes[this.territory] = {
      territoryId: this.territory,
      polygons: cleanPolygons,
      labelPosition: normalizePoint(this.label),
    };
  }

  /** Pretty-printed TS source for the full working shape set (every previously-traced territory plus the one currently being edited), ready to paste into territoryShapes.ts. */
  exportSource(): string {
    this.commitCurrentShape();
    const entries = Object.entries(this.shapes)
      .map(([name, shape]) => {
        const polys = shape.polygons
          .map(
            (poly) =>
              `      [\n${poly
                .map((p) => `        { x: ${p.x.toFixed(4)}, y: ${p.y.toFixed(4)} },`)
                .join("\n")}\n      ],`,
          )
          .join("\n");
        return (
          `  ${JSON.stringify(name)}: {\n` +
          `    territoryId: ${JSON.stringify(name)},\n` +
          `    polygons: [\n${polys}\n    ],\n` +
          `    labelPosition: { x: ${shape.labelPosition.x.toFixed(4)}, y: ${shape.labelPosition.y.toFixed(4)} },\n` +
          `  },`
        );
      })
      .join("\n");
    return `export const territoryShapes: Record<string, TerritoryShape> = {\n${entries}\n};\n`;
  }

  snapshot(): MapEditorSnapshot {
    const last = this.polygons[this.polygons.length - 1];
    return {
      territory: this.territory,
      polygonCount: this.polygons.filter((p) => p.length > 0).length,
      currentPolygonPointCount: last?.length ?? 0,
      hasLabel: this.label !== null,
      labelMode: this.labelMode,
    };
  }

  private redraw() {
    const g = this.gfx.clear();

    // Dim reference outlines for every other already-traced territory, so
    // neighboring borders are visible while tracing the active one.
    for (const [name, shape] of Object.entries(this.shapes)) {
      if (name === this.territory) continue;
      for (const poly of shape.polygons) {
        g.poly(poly.map(denormalizePoint), true).stroke({ color: 0x64748b, width: 1, alpha: 0.35 });
      }
    }

    // Active territory: live outline + vertex handles.
    for (const poly of this.polygons) {
      if (poly.length === 0) continue;
      if (poly.length >= 2) {
        g.poly(poly, poly.length >= 3).stroke({ color: 0x22d3ee, width: 2, alpha: 0.9 });
      }
      for (const v of poly) {
        g.circle(v.x, v.y, VERTEX_RADIUS).fill({ color: 0x22d3ee, alpha: 0.9 });
      }
    }

    if (this.label) {
      g.circle(this.label.x, this.label.y, VERTEX_RADIUS + 2).stroke({ color: 0xfbbf24, width: 2 });
    }

    this.onChange(this.snapshot());
  }
}
