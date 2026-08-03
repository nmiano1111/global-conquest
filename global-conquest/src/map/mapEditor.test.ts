import { Graphics } from "pixi.js";
import { describe, expect, it } from "vitest";
import { MapEditorController } from "./mapEditor";
import { MAP_SOURCE_HEIGHT, MAP_SOURCE_WIDTH } from "./territoryShapes";

function makeController() {
  const snapshots: ReturnType<MapEditorController["snapshot"]>[] = [];
  const controller = new MapEditorController(new Graphics(), (s) => snapshots.push(s));
  return { controller, snapshots };
}

describe("MapEditorController", () => {
  it("starts a fresh, empty polygon for a territory with no existing shape", () => {
    const { controller, snapshots } = makeController();
    controller.selectTerritory("Testlandia");
    const last = snapshots.at(-1)!;
    expect(last.territory).toBe("Testlandia");
    expect(last.polygonCount).toBe(0);
    expect(last.hasLabel).toBe(false);
  });

  it("loads an existing shape's points denormalized into world space", () => {
    const { controller, snapshots } = makeController();
    controller.selectTerritory("Iceland"); // has sample data in territoryShapes.ts
    const last = snapshots.at(-1)!;
    expect(last.polygonCount).toBe(1);
    expect(last.hasLabel).toBe(true);
  });

  it("adds a vertex to the current polygon on tap", () => {
    const { controller, snapshots } = makeController();
    controller.selectTerritory("Testlandia");
    controller.handleTap({ x: 100, y: 200 });
    controller.handleTap({ x: 150, y: 250 });
    const last = snapshots.at(-1)!;
    expect(last.currentPolygonPointCount).toBe(2);
  });

  it("sets the label position instead of adding a vertex while label mode is on", () => {
    const { controller, snapshots } = makeController();
    controller.selectTerritory("Testlandia");
    controller.setLabelMode(true);
    controller.handleTap({ x: 300, y: 400 });
    const last = snapshots.at(-1)!;
    expect(last.hasLabel).toBe(true);
    expect(last.currentPolygonPointCount).toBe(0);
    expect(last.labelMode).toBe(false); // placing the label turns the mode back off
  });

  it("starts a new polygon only once the current one has points (for islands)", () => {
    const { controller, snapshots } = makeController();
    controller.selectTerritory("Testlandia");
    controller.startNewPolygon(); // no-op: current polygon is still empty
    expect(snapshots.at(-1)!.polygonCount).toBe(0);

    controller.handleTap({ x: 10, y: 10 });
    controller.startNewPolygon();
    controller.handleTap({ x: 20, y: 20 });
    const last = snapshots.at(-1)!;
    expect(last.polygonCount).toBe(2);
    expect(last.currentPolygonPointCount).toBe(1);
  });

  it("undoes the last point, then the last empty polygon", () => {
    const { controller, snapshots } = makeController();
    controller.selectTerritory("Testlandia");
    controller.handleTap({ x: 1, y: 1 });
    controller.startNewPolygon();
    controller.undoPoint(); // removes the empty second polygon
    expect(snapshots.at(-1)!.polygonCount).toBe(1);
    controller.undoPoint(); // removes the only point in the first polygon
    expect(snapshots.at(-1)!.currentPolygonPointCount).toBe(0);
  });

  it("drags an existing vertex to a new position", () => {
    const { controller } = makeController();
    controller.selectTerritory("Testlandia");
    controller.handleTap({ x: 0, y: 0 });
    controller.handleTap({ x: 100, y: 0 });
    controller.handleTap({ x: 0, y: 100 });
    controller.setLabelMode(true);
    controller.handleTap({ x: 50, y: 50 });

    const started = controller.tryStartDrag({ x: 2, y: 1 }, 10); // near the (0,0) vertex
    expect(started).toBe(true);
    controller.updateDrag({ x: MAP_SOURCE_WIDTH, y: MAP_SOURCE_HEIGHT });
    controller.endDrag();

    const source = controller.exportSource();
    expect(source).toContain('"Testlandia"');
    expect(source).toContain("x: 1.0000"); // the dragged vertex, normalized to the map's far edge
  });

  it("does not start a drag when no vertex is within radius", () => {
    const { controller } = makeController();
    controller.selectTerritory("Testlandia");
    controller.handleTap({ x: 100, y: 100 });
    expect(controller.tryStartDrag({ x: 1000, y: 1000 }, 10)).toBe(false);
  });

  it("exports valid, normalized coordinates for a traced territory", () => {
    const { controller } = makeController();
    controller.selectTerritory("Testlandia");
    controller.handleTap({ x: 0, y: 0 });
    controller.handleTap({ x: MAP_SOURCE_WIDTH, y: 0 });
    controller.handleTap({ x: MAP_SOURCE_WIDTH / 2, y: MAP_SOURCE_HEIGHT });
    controller.setLabelMode(true);
    controller.handleTap({ x: MAP_SOURCE_WIDTH / 2, y: MAP_SOURCE_HEIGHT / 2 });

    const source = controller.exportSource();
    expect(source).toContain('"Testlandia"');
    expect(source).toContain("x: 1.0000");
    expect(source).toContain("x: 0.5000");
  });

  it("omits a territory from the export if it never got 3 points or a label", () => {
    const { controller } = makeController();
    controller.selectTerritory("Testlandia");
    controller.handleTap({ x: 0, y: 0 });
    controller.handleTap({ x: 10, y: 10 }); // only 2 points, no label
    const source = controller.exportSource();
    expect(source).not.toContain('"Testlandia"');
  });
});
