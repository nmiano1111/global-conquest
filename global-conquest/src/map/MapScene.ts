import {
  Application,
  Assets,
  Container,
  type FederatedPointerEvent,
  Sprite,
  Texture,
} from "pixi.js";
import riskBoardImage from "../assets/images/risk0.png";
import {
  MAP_CENTER_X,
  MAP_CENTER_Y,
  MAP_OVERLAY_OFFSET_X,
  MAP_OVERLAY_OFFSET_Y,
  MAP_OVERLAY_SCALE,
  MAP_TERRITORIES,
  MAP_VIEWBOX_HEIGHT,
  MAP_VIEWBOX_WIDTH,
} from "../router/pages/gameShared";
import { TerritoryNode } from "./TerritoryNode";
import type { TerritoryDisplayState } from "./types";

const MAX_ZOOM = 4.0;

export class MapScene {
  private readonly app: Application;

  /**
   * The world container lives in map-coordinate space (MAP_VIEWBOX_WIDTH ×
   * MAP_VIEWBOX_HEIGHT). The camera is expressed entirely as this container's
   * position and uniform scale — nothing else moves.
   */
  private readonly worldContainer: Container;

  /**
   * Applies the static overlay transform that aligns territory coordinates
   * (defined in SVG space) onto the background sprite. This is an internal
   * positioning detail; it is not part of the camera.
   */
  private readonly overlayContainer: Container;
  private readonly nodes: Map<string, TerritoryNode> = new Map();

  // Camera state — always kept in sync with worldContainer via applyCamera().
  private camScale = 1;
  private camX = 0;
  private camY = 0;

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
    // --- Background sprite (world space: 0,0 → MAP_VIEWBOX_WIDTH × MAP_VIEWBOX_HEIGHT) ---
    const texture = await Assets.load<Texture>(riskBoardImage);
    const bg = new Sprite(texture);
    bg.width = MAP_VIEWBOX_WIDTH;
    bg.height = MAP_VIEWBOX_HEIGHT;
    this.worldContainer.addChild(bg);

    // --- Territory overlay (static alignment transform, not part of the camera) ---
    // Territory coordinates in MAP_TERRITORIES are in SVG space. This transform
    // maps them onto the correct positions over the background sprite, mirroring
    // the original SVG <g> transform: translate(cx+ox, cy+oy) scale(s) translate(-cx,-cy).
    this.overlayContainer.pivot.set(MAP_CENTER_X, MAP_CENTER_Y);
    this.overlayContainer.position.set(
      MAP_CENTER_X + MAP_OVERLAY_OFFSET_X,
      MAP_CENTER_Y + MAP_OVERLAY_OFFSET_Y,
    );
    this.overlayContainer.scale.set(MAP_OVERLAY_SCALE);
    this.worldContainer.addChild(this.overlayContainer);

    // --- Territory nodes ---
    for (const [name, pos] of Object.entries(MAP_TERRITORIES)) {
      const node = new TerritoryNode(name, pos.x, pos.y, onTerritoryClick);
      this.nodes.set(name, node);
      this.overlayContainer.addChild(node);
    }

    this.app.stage.addChild(this.worldContainer);

    // Fit the entire map into the initial viewport.
    this.fitToViewport();

    // Wire up pan and pinch-to-zoom.
    this.setupInteraction();
  }

  // ---------------------------------------------------------------------------
  // Camera helpers
  // ---------------------------------------------------------------------------

  /** Scale at which the full map exactly fits inside the current viewport. */
  private get minZoom(): number {
    return Math.min(
      this.app.screen.width / MAP_VIEWBOX_WIDTH,
      this.app.screen.height / MAP_VIEWBOX_HEIGHT,
    );
  }

  private applyCamera() {
    this.worldContainer.scale.set(this.camScale);
    this.worldContainer.position.set(this.camX, this.camY);
  }

  /**
   * Clamp camX/camY so the world never exposes empty space beyond its edges.
   * When the world fits inside the viewport on an axis, center it instead.
   */
  private clampCamera() {
    const vw = this.app.screen.width;
    const vh = this.app.screen.height;
    const ww = MAP_VIEWBOX_WIDTH * this.camScale;
    const wh = MAP_VIEWBOX_HEIGHT * this.camScale;

    if (ww <= vw) {
      this.camX = (vw - ww) / 2;
    } else {
      this.camX = Math.min(0, Math.max(vw - ww, this.camX));
    }

    if (wh <= vh) {
      this.camY = (vh - wh) / 2;
    } else {
      this.camY = Math.min(0, Math.max(vh - wh, this.camY));
    }
  }

  /**
   * Zoom to newScale while keeping the screen-space point (pivotX, pivotY)
   * fixed over the same world point — i.e., zoom toward the cursor / pinch midpoint.
   */
  private zoomToward(newScale: number, pivotX: number, pivotY: number) {
    const clamped = Math.max(this.minZoom, Math.min(MAX_ZOOM, newScale));
    // World-space point currently under the pivot must stay under it after zoom.
    const worldX = (pivotX - this.camX) / this.camScale;
    const worldY = (pivotY - this.camY) / this.camScale;
    this.camScale = clamped;
    this.camX = pivotX - worldX * this.camScale;
    this.camY = pivotY - worldY * this.camScale;
    this.clampCamera();
    this.applyCamera();
  }

  /** Scale and center the world to fit the current viewport. */
  private fitToViewport() {
    this.camScale = this.minZoom;
    const vw = this.app.screen.width;
    const vh = this.app.screen.height;
    this.camX = (vw - MAP_VIEWBOX_WIDTH * this.camScale) / 2;
    this.camY = (vh - MAP_VIEWBOX_HEIGHT * this.camScale) / 2;
    this.applyCamera();
  }

  // ---------------------------------------------------------------------------
  // Interaction (pan + pinch-to-zoom)
  // ---------------------------------------------------------------------------

  private setupInteraction() {
    const stage = this.app.stage;
    stage.eventMode = "static";
    // app.screen is the live rectangle that tracks the renderer size.
    stage.hitArea = this.app.screen;

    // Active pointers tracked by pointerId → current screen position.
    const pointers = new Map<number, { x: number; y: number }>();

    // Single-finger pan state.
    let panActive = false;
    let panStartX = 0;
    let panStartY = 0;
    let camStartX = 0;
    let camStartY = 0;

    // Two-finger pinch state.
    let pinchStartDist = 0;
    let pinchStartScale = 0;
    let pinchMidX = 0;
    let pinchMidY = 0;

    const beginPan = (x: number, y: number) => {
      panActive = true;
      panStartX = x;
      panStartY = y;
      camStartX = this.camX;
      camStartY = this.camY;
      this.app.canvas.style.cursor = "grabbing";
    };

    stage.on("pointerdown", (e: FederatedPointerEvent) => {
      pointers.set(e.pointerId, { x: e.globalX, y: e.globalY });

      if (pointers.size === 2) {
        // Transition from pan to pinch.
        panActive = false;
        const pts = [...pointers.values()];
        pinchMidX = (pts[0].x + pts[1].x) / 2;
        pinchMidY = (pts[0].y + pts[1].y) / 2;
        pinchStartDist = Math.hypot(pts[1].x - pts[0].x, pts[1].y - pts[0].y);
        pinchStartScale = this.camScale;
      } else {
        beginPan(e.globalX, e.globalY);
      }
    });

    stage.on("pointermove", (e: FederatedPointerEvent) => {
      if (!pointers.has(e.pointerId)) return;
      pointers.set(e.pointerId, { x: e.globalX, y: e.globalY });

      if (pointers.size === 2) {
        const pts = [...pointers.values()];
        const dist = Math.hypot(pts[1].x - pts[0].x, pts[1].y - pts[0].y);
        this.zoomToward(pinchStartScale * (dist / pinchStartDist), pinchMidX, pinchMidY);
      } else if (panActive) {
        this.camX = camStartX + (e.globalX - panStartX);
        this.camY = camStartY + (e.globalY - panStartY);
        this.clampCamera();
        this.applyCamera();
      }
    });

    const onPointerUp = (e: FederatedPointerEvent) => {
      pointers.delete(e.pointerId);

      if (pointers.size === 0) {
        panActive = false;
        this.app.canvas.style.cursor = "grab";
      } else if (pointers.size === 1) {
        // One finger lifted during pinch — resume single-finger pan from here.
        const [remaining] = [...pointers.values()];
        beginPan(remaining.x, remaining.y);
      }
    };

    stage.on("pointerup", onPointerUp);
    stage.on("pointerupoutside", onPointerUp);

    this.app.canvas.style.cursor = "grab";
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Called by a ResizeObserver when the viewport container changes size.
   * Resizes the renderer, clamps the camera, and ensures zoom never falls
   * below the new fit scale.
   */
  resize(width: number, height: number) {
    this.app.renderer.resize(width, height);
    // Enforce min zoom in case the viewport grew and the current scale would
    // now expose empty space beyond the map edges.
    if (this.camScale < this.minZoom) {
      this.camScale = this.minZoom;
    }
    this.clampCamera();
    this.applyCamera();
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

  zoomIn() {
    const cx = this.app.screen.width / 2;
    const cy = this.app.screen.height / 2;
    this.zoomToward(this.camScale * 1.35, cx, cy);
  }

  zoomOut() {
    const cx = this.app.screen.width / 2;
    const cy = this.app.screen.height / 2;
    this.zoomToward(this.camScale / 1.35, cx, cy);
  }

  resetZoom() {
    this.fitToViewport();
  }

  destroy() {
    this.app.destroy(true);
  }
}
