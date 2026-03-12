import { Application, Assets, Container, Graphics, Sprite, Texture } from "pixi.js";
import riskBoardImage from "../assets/images/risk0.png";
import {
  MAP_CENTER_X,
  MAP_CENTER_Y,
  MAP_EDGES,
  MAP_OVERLAY_OFFSET_X,
  MAP_OVERLAY_OFFSET_Y,
  MAP_OVERLAY_SCALE,
  MAP_TERRITORIES,
  MAP_VIEWBOX_HEIGHT,
  MAP_VIEWBOX_WIDTH,
} from "../router/pages/gameShared";
import { TerritoryNode } from "./TerritoryNode";
import type { TerritoryDisplayState } from "./types";

export class MapScene {
  private readonly app: Application;
  /**
   * Scaled + centered to fit the canvas, holds the full 2048×1367 world.
   * Equivalent to SVG's preserveAspectRatio="xMidYMid meet".
   */
  private readonly worldContainer: Container;
  /**
   * Applies the same static pan/zoom as the SVG <g> transform.
   * pivot=(center), position=(center+offset), scale=MAP_OVERLAY_SCALE
   */
  private readonly overlayContainer: Container;
  private readonly nodes: Map<string, TerritoryNode> = new Map();

  private constructor(app: Application) {
    this.app = app;
    this.worldContainer = new Container();
    this.overlayContainer = new Container();
  }

  /** The PixiJS-owned canvas element. Mount this into the DOM after create(). */
  get canvas(): HTMLCanvasElement {
    return this.app.canvas;
  }

  /**
   * Async factory — PixiJS creates its own canvas element (avoids stale WebGL
   * context issues when React StrictMode remounts effects on the same DOM node).
   * Append scene.canvas to your container after this resolves.
   */
  static async create(
    width: number,
    height: number,
    onTerritoryClick: (name: string) => void,
  ): Promise<MapScene> {
    const app = new Application();
    await app.init({
      // No canvas option — PixiJS creates a fresh one each time.
      // This avoids the corrupted-WebGL-context error on StrictMode remount.
      width,
      height,
      background: 0x0f172a,
      antialias: true,
      resolution: window.devicePixelRatio || 1,
      autoDensity: true,
    });

    const scene = new MapScene(app);
    await scene.buildScene(onTerritoryClick);
    scene.fitWorld(width, height);
    return scene;
  }

  private async buildScene(onTerritoryClick: (name: string) => void) {
    // --- Background sprite ---
    const texture = await Assets.load<Texture>(riskBoardImage);
    const bg = new Sprite(texture);
    bg.width = MAP_VIEWBOX_WIDTH;
    bg.height = MAP_VIEWBOX_HEIGHT;
    this.worldContainer.addChild(bg);

    // --- Overlay container (mirrors the SVG <g> transform) ---
    // SVG: translate(cx+ox, cy+oy) scale(s) translate(-cx, -cy)
    // Pixi: pivot at (cx, cy), position at (cx+ox, cy+oy), scale s — identical result
    this.overlayContainer.pivot.set(MAP_CENTER_X, MAP_CENTER_Y);
    this.overlayContainer.position.set(
      MAP_CENTER_X + MAP_OVERLAY_OFFSET_X,
      MAP_CENTER_Y + MAP_OVERLAY_OFFSET_Y,
    );
    this.overlayContainer.scale.set(MAP_OVERLAY_SCALE);
    this.worldContainer.addChild(this.overlayContainer);

    // --- Edge layer (drawn first, underneath territory nodes) ---
    const edges = new Graphics();
    for (const [a, b] of MAP_EDGES) {
      const from = MAP_TERRITORIES[a];
      const to = MAP_TERRITORIES[b];
      if (!from || !to) continue;
      edges.moveTo(from.x, from.y).lineTo(to.x, to.y);
    }
    edges.stroke({ color: "#0f172a", alpha: 0.35, width: 3 });
    this.overlayContainer.addChild(edges);

    // --- Territory nodes ---
    for (const [name, pos] of Object.entries(MAP_TERRITORIES)) {
      const node = new TerritoryNode(name, pos.x, pos.y, onTerritoryClick);
      this.nodes.set(name, node);
      this.overlayContainer.addChild(node);
    }

    this.app.stage.addChild(this.worldContainer);
  }

  /**
   * Scale and center worldContainer to fit the given canvas dimensions.
   * Equivalent to preserveAspectRatio="xMidYMid meet".
   */
  private fitWorld(width: number, height: number) {
    const scale = Math.min(width / MAP_VIEWBOX_WIDTH, height / MAP_VIEWBOX_HEIGHT);
    this.worldContainer.scale.set(scale);
    this.worldContainer.position.set(
      (width - MAP_VIEWBOX_WIDTH * scale) / 2,
      (height - MAP_VIEWBOX_HEIGHT * scale) / 2,
    );
  }

  resize(width: number, height: number) {
    this.app.renderer.resize(width, height);
    this.fitWorld(width, height);
  }

  updateTerritories(
    territoryStates: Record<string, unknown> | null,
    selectedTerritory: string,
    activeFrom: string,
    activeTo: string,
    playerColors: string[],
  ) {
    for (const [name, node] of this.nodes) {
      const raw = territoryStates?.[name];
      const t =
        raw && typeof raw === "object" ? (raw as Record<string, unknown>) : null;
      const state: TerritoryDisplayState = {
        owner: typeof t?.owner === "number" ? t.owner : -1,
        armies: typeof t?.armies === "number" ? t.armies : 0,
        isSelected:
          name === selectedTerritory ||
          name === activeFrom ||
          name === activeTo,
      };
      node.update(state, playerColors);
    }
  }

  destroy() {
    // true = also destroy the canvas element; the caller already removed it
    // from the DOM, so this just frees GPU resources cleanly.
    this.app.destroy(true);
  }
}
