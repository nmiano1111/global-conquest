import {
  Application,
  Assets,
  Container,
  type FederatedPointerEvent,
  Graphics,
  Rectangle,
  Sprite,
  Texture,
} from "pixi.js";
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

// The world renders at this fixed scale regardless of viewport size.
// viewport (resizes) → worldContainer (fixed scale, pans) → map + territories
const WORLD_SCALE = 1024 / MAP_VIEWBOX_WIDTH; // world = 1024 × 683 px

export class MapScene {
  private readonly app: Application;
  /** Fixed-scale world container — position changes when the user pans. */
  private readonly worldContainer: Container;
  /** Plain grouping layer for edges and territory nodes. */
  private readonly overlayContainer: Container;
  private readonly nodes: Map<string, TerritoryNode> = new Map();

  private constructor(app: Application) {
    this.app = app;
    this.worldContainer = new Container();
    this.overlayContainer = new Container();
  }

  get canvas(): HTMLCanvasElement {
    return this.app.canvas;
  }

  static async create(
    width: number,
    height: number,
    onTerritoryClick: (name: string) => void,
  ): Promise<MapScene> {
    const app = new Application();
    await app.init({
      width,
      height,
      background: 0x0f172a,
      antialias: true,
      resolution: window.devicePixelRatio || 1,
      autoDensity: true,
    });

    const scene = new MapScene(app);
    await scene.buildScene(onTerritoryClick);
    return scene;
  }

  private async buildScene(onTerritoryClick: (name: string) => void) {
    // --- Background ---
    const texture = await Assets.load<Texture>(riskBoardImage);
    const bg = new Sprite(texture);
    bg.width = MAP_VIEWBOX_WIDTH;
    bg.height = MAP_VIEWBOX_HEIGHT;
    this.worldContainer.addChild(bg);

    // --- Overlay layer — mirrors the SVG <g> transform ---
    // SVG: translate(cx+ox, cy+oy) scale(s) translate(-cx, -cy)
    // Pixi: pivot at (cx, cy), position at (cx+ox, cy+oy), scale s — identical result
    this.overlayContainer.pivot.set(MAP_CENTER_X, MAP_CENTER_Y);
    this.overlayContainer.position.set(
      MAP_CENTER_X + MAP_OVERLAY_OFFSET_X,
      MAP_CENTER_Y + MAP_OVERLAY_OFFSET_Y,
    );
    this.overlayContainer.scale.set(MAP_OVERLAY_SCALE);
    this.worldContainer.addChild(this.overlayContainer);

    // --- Edges ---
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

    // Fix the world scale — it never changes after this point.
    this.worldContainer.scale.set(WORLD_SCALE);

    // Center the world in the initial viewport.
    this.centerWorld();

    // Wire up drag-to-pan.
    this.setupPan();
  }

  // ---------------------------------------------------------------------------
  // Viewport helpers
  // ---------------------------------------------------------------------------

  private get vw() { return this.app.screen.width; }
  private get vh() { return this.app.screen.height; }
  private get worldW() { return MAP_VIEWBOX_WIDTH * WORLD_SCALE; }
  private get worldH() { return MAP_VIEWBOX_HEIGHT * WORLD_SCALE; }

  /** Place the world centered in the current viewport, then clamp. */
  private centerWorld() {
    this.worldContainer.x = Math.round((this.vw - this.worldW) / 2);
    this.worldContainer.y = Math.round((this.vh - this.worldH) / 2);
    this.clampWorld();
  }

  /**
   * Prevent the world from being dragged so far that the viewport shows only
   * empty space. If the world is larger than the viewport in a given axis,
   * constrain so both edges of the world remain reachable but neither goes
   * beyond the viewport edge. If the world fits, keep it centered.
   */
  private clampWorld() {
    const { vw, vh, worldW, worldH } = this;

    if (worldW >= vw) {
      // World wider than viewport — allow panning across the full world width.
      this.worldContainer.x = Math.max(vw - worldW, Math.min(0, this.worldContainer.x));
    } else {
      // World fits horizontally — center it.
      this.worldContainer.x = Math.round((vw - worldW) / 2);
    }

    if (worldH >= vh) {
      this.worldContainer.y = Math.max(vh - worldH, Math.min(0, this.worldContainer.y));
    } else {
      this.worldContainer.y = Math.round((vh - worldH) / 2);
    }
  }

  // ---------------------------------------------------------------------------
  // Drag-to-pan
  // ---------------------------------------------------------------------------

  private setupPan() {
    let dragging = false;
    let dragStart = { x: 0, y: 0 };
    let worldStart = { x: 0, y: 0 };

    const stage = this.app.stage;
    stage.eventMode = "static";
    // Ensure the stage captures pointer events across the full canvas area.
    stage.hitArea = new Rectangle(0, 0, 100_000, 100_000);

    stage.on("pointerdown", (e: FederatedPointerEvent) => {
      dragging = true;
      dragStart = { x: e.globalX, y: e.globalY };
      worldStart = { x: this.worldContainer.x, y: this.worldContainer.y };
      this.app.canvas.style.cursor = "grabbing";
    });

    stage.on("pointermove", (e: FederatedPointerEvent) => {
      if (!dragging) return;
      this.worldContainer.x = worldStart.x + (e.globalX - dragStart.x);
      this.worldContainer.y = worldStart.y + (e.globalY - dragStart.y);
      this.clampWorld();
    });

    const stopDrag = () => {
      dragging = false;
      this.app.canvas.style.cursor = "grab";
    };
    stage.on("pointerup", stopDrag);
    stage.on("pointerupoutside", stopDrag);

    // Default cursor when not dragging.
    this.app.canvas.style.cursor = "grab";
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Called by ResizeObserver when the viewport div changes size.
   * Only the canvas is resized — world scale and position are preserved,
   * then re-clamped so the world stays in a valid position.
   */
  resize(width: number, height: number) {
    this.app.renderer.resize(width, height);
    this.clampWorld();
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
    this.app.destroy(true);
  }
}
