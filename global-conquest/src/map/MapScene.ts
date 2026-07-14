import {
  Application,
  Assets,
  Container,
  Graphics,
  type FederatedPointerEvent,
  type FederatedWheelEvent,
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
import {
  clampCameraPosition,
  computeMinZoom,
  computeReleaseVelocity,
  distance,
  exceedsDragThreshold,
  fitToViewport,
  isDoubleTap,
  stepMomentum,
  velocityMagnitude,
  type PanSample,
  type TapRecord,
  type Velocity,
  zoomToward,
} from "./cameraMath";
import { TerritoryNode } from "./TerritoryNode";
import type { TerritoryDisplayState, TerritoryHighlightKind } from "./types";

const MAX_ZOOM = 4.0;
const DOUBLE_TAP_ZOOM_FACTOR = 1.8;
const WHEEL_ZOOM_FACTOR = 1.0015; // per pixel of wheel delta

// Momentum glide (inertial panning) tuning. Friction is a per-~16.67ms-frame
// decay factor — normalized to real elapsed time in stepMomentum, so it
// feels consistent regardless of actual frame rate.
const MOMENTUM_FRICTION = 0.94;
/** Below this speed (px/ms), a glide in progress is considered finished. */
const MOMENTUM_STOP_SPEED = 0.02;
/** A release must be at least this fast (px/ms) to start a glide at all — filters out slow, deliberate drags. */
const MOMENTUM_START_SPEED = 0.15;

export interface TerritoryHighlightInput {
  /** The territory currently selected as the primary source (reinforce target / attack-from / fortify-from). */
  selectedSource?: string;
  /** The territory currently selected as a secondary target (attack-to / fortify-to). */
  selectedTarget?: string;
  /** Territories that are legal targets for the current selection (e.g. attackable enemy neighbors). */
  legalTargets?: ReadonlySet<string>;
  /** Territories involved in the most recently resolved combat. */
  recentCombat?: ReadonlySet<string>;
  /** Territory just captured and awaiting occupation. */
  recentCapture?: ReadonlySet<string>;
  /**
   * Passive highlight for territories not covered by the local user's own
   * selection — the most recently committed bot action, and other players'
   * live territory presses relayed over the socket.
   */
  passive?: ReadonlySet<string>;
}

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
  private readonly connectorGfx: Graphics;

  // Camera state — always kept in sync with worldContainer via applyCamera().
  private camScale = 1;
  private camX = 0;
  private camY = 0;

  /**
   * Set true whenever the active gesture has moved past the drag threshold.
   * TerritoryNode click callbacks are wrapped to no-op while this is true,
   * so panning/pinching never accidentally selects a territory. Reset at
   * the start of every fresh (zero-pointer) gesture.
   */
  private dragSuppressesClick = false;

  private lastTap: TapRecord | null = null;

  /** Trailing history of recent single-finger pan positions, used to estimate release velocity for momentum glide. */
  private panSamples: PanSample[] = [];
  /** Non-null while a momentum glide is animating; consumed frame-by-frame in onTick. */
  private momentumVelocity: Velocity | null = null;

  private readonly reducedMotion: boolean;
  private pulsePhase = 0;
  private readonly initialFit: "contain" | "cover";

  private constructor(app: Application, initialFit: "contain" | "cover") {
    this.app = app;
    this.initialFit = initialFit;
    this.worldContainer = new Container();
    this.overlayContainer = new Container();
    this.connectorGfx = new Graphics();
    this.reducedMotion =
      typeof window !== "undefined" &&
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  get canvas(): HTMLCanvasElement {
    return this.app.canvas;
  }

  static async create(
    width: number,
    height: number,
    onTerritoryClick: (name: string) => void,
    initialFit: "contain" | "cover" = "contain",
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

    const scene = new MapScene(app, initialFit);
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

    // --- Connecting line between selected source/target (drawn beneath nodes) ---
    this.overlayContainer.addChild(this.connectorGfx);

    // --- Territory nodes ---
    for (const [name, pos] of Object.entries(MAP_TERRITORIES)) {
      const node = new TerritoryNode(name, pos.x, pos.y, (n) => {
        if (this.dragSuppressesClick) return;
        onTerritoryClick(n);
      });
      this.nodes.set(name, node);
      this.overlayContainer.addChild(node);
    }

    this.app.stage.addChild(this.worldContainer);

    // Fit the entire map into the initial viewport.
    this.fitToViewport();

    // Wire up pan, pinch, wheel, and double-tap.
    this.setupInteraction();

    if (!this.reducedMotion) {
      this.app.ticker.add(this.onTick);
    }
  }

  private readonly onTick = () => {
    this.pulsePhase += 0.06;
    for (const node of this.nodes.values()) {
      node.tickPulse(this.pulsePhase);
    }

    if (this.momentumVelocity) {
      const result = stepMomentum(
        { scale: this.camScale, x: this.camX, y: this.camY },
        this.momentumVelocity,
        this.app.ticker.deltaMS,
        MOMENTUM_FRICTION,
        { width: this.app.screen.width, height: this.app.screen.height },
        { width: MAP_VIEWBOX_WIDTH, height: MAP_VIEWBOX_HEIGHT },
      );
      this.camX = result.camera.x;
      this.camY = result.camera.y;
      this.applyCamera();
      this.momentumVelocity =
        velocityMagnitude(result.velocity) < MOMENTUM_STOP_SPEED ? null : result.velocity;
    }
  };

  // ---------------------------------------------------------------------------
  // Camera helpers
  // ---------------------------------------------------------------------------

  /** Scale at which the full map exactly fits inside the current viewport. */
  private get minZoom(): number {
    return computeMinZoom(
      { width: this.app.screen.width, height: this.app.screen.height },
      { width: MAP_VIEWBOX_WIDTH, height: MAP_VIEWBOX_HEIGHT },
    );
  }

  private applyCamera() {
    this.worldContainer.scale.set(this.camScale);
    this.worldContainer.position.set(this.camX, this.camY);
  }

  private clampCamera() {
    const next = clampCameraPosition(
      { scale: this.camScale, x: this.camX, y: this.camY },
      { width: this.app.screen.width, height: this.app.screen.height },
      { width: MAP_VIEWBOX_WIDTH, height: MAP_VIEWBOX_HEIGHT },
    );
    this.camX = next.x;
    this.camY = next.y;
  }

  /**
   * Zoom to newScale while keeping the screen-space point (pivotX, pivotY)
   * fixed over the same world point — i.e., zoom toward the cursor / pinch
   * midpoint / double-tap point.
   */
  private zoomToward(newScale: number, pivotX: number, pivotY: number) {
    this.momentumVelocity = null; // a deliberate zoom overrides any in-flight glide
    const next = zoomToward(
      { scale: this.camScale, x: this.camX, y: this.camY },
      newScale,
      pivotX,
      pivotY,
      this.minZoom,
      MAX_ZOOM,
      { width: this.app.screen.width, height: this.app.screen.height },
      { width: MAP_VIEWBOX_WIDTH, height: MAP_VIEWBOX_HEIGHT },
    );
    this.camScale = next.scale;
    this.camX = next.x;
    this.camY = next.y;
    this.applyCamera();
  }

  /**
   * Scale and center the world to fit the current viewport in the given
   * mode ("contain" shows the whole board; "cover" fills the viewport
   * edge to edge, cropping — the user can always zoom out to minZoom,
   * which is always the "contain" scale regardless of mode, to see the
   * whole board).
   */
  private applyFit(mode: "contain" | "cover") {
    this.momentumVelocity = null; // recentering overrides any in-flight glide
    const next = fitToViewport(
      { width: this.app.screen.width, height: this.app.screen.height },
      { width: MAP_VIEWBOX_WIDTH, height: MAP_VIEWBOX_HEIGHT },
      mode,
    );
    this.camScale = Math.min(next.scale, MAX_ZOOM);
    this.camX = next.x;
    this.camY = next.y;
    // Only matters in the (practically unreachable) case where the cover
    // scale needed MAX_ZOOM clamping above — re-centers x/y for the
    // clamped scale instead of the original unclamped one.
    this.clampCamera();
    this.applyCamera();
  }

  /** Fit using this scene's default mode (set once at construction). Used for the very first fit and for resetZoom(). */
  private fitToViewport() {
    this.applyFit(this.initialFit);
  }

  // ---------------------------------------------------------------------------
  // Interaction (pan, pinch-to-zoom, wheel-zoom, double-tap-zoom, tap detection)
  // ---------------------------------------------------------------------------

  private setupInteraction() {
    const stage = this.app.stage;
    stage.eventMode = "static";
    // app.screen is the live rectangle that tracks the renderer size.
    stage.hitArea = this.app.screen;
    this.canvas.style.touchAction = "none";

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
      this.panSamples = [{ x, y, timeMs: Date.now() }];
      this.canvas.style.cursor = "grabbing";
    };

    stage.on("pointerdown", (e: FederatedPointerEvent) => {
      // Any new touch stops an in-flight glide immediately — a hand landing
      // on a moving map should still it, not fight it.
      this.momentumVelocity = null;

      if (pointers.size === 0) {
        // Fresh gesture: don't let a previous drag suppress this tap.
        this.dragSuppressesClick = false;
      }
      pointers.set(e.pointerId, { x: e.globalX, y: e.globalY });

      if (pointers.size === 2) {
        // Transition from pan to pinch.
        panActive = false;
        const pts = [...pointers.values()];
        pinchMidX = (pts[0].x + pts[1].x) / 2;
        pinchMidY = (pts[0].y + pts[1].y) / 2;
        pinchStartDist = distance(pts[0].x, pts[0].y, pts[1].x, pts[1].y);
        pinchStartScale = this.camScale;
      } else {
        beginPan(e.globalX, e.globalY);
      }
    });

    stage.on("pointermove", (e: FederatedPointerEvent) => {
      if (!pointers.has(e.pointerId)) return;
      pointers.set(e.pointerId, { x: e.globalX, y: e.globalY });

      if (pointers.size === 2) {
        this.dragSuppressesClick = true;
        const pts = [...pointers.values()];
        const dist = distance(pts[0].x, pts[0].y, pts[1].x, pts[1].y);
        this.zoomToward(pinchStartScale * (dist / pinchStartDist), pinchMidX, pinchMidY);
      } else if (panActive) {
        if (exceedsDragThreshold(panStartX, panStartY, e.globalX, e.globalY)) {
          this.dragSuppressesClick = true;
        }
        this.camX = camStartX + (e.globalX - panStartX);
        this.camY = camStartY + (e.globalY - panStartY);
        this.clampCamera();
        this.applyCamera();

        // Trailing sample history for release-velocity estimation. Trimmed
        // to a short window so a slow start followed by a fast flick isn't
        // dragged down by stale, slower early samples.
        const now = Date.now();
        this.panSamples.push({ x: e.globalX, y: e.globalY, timeMs: now });
        this.panSamples = this.panSamples.filter((s) => now - s.timeMs <= 150);
      }
    });

    const onPointerUp = (e: FederatedPointerEvent) => {
      const wasPanning = panActive;
      pointers.delete(e.pointerId);

      if (pointers.size === 0) {
        panActive = false;
        this.canvas.style.cursor = "grab";

        if (wasPanning && !this.reducedMotion) {
          const releaseVelocity = computeReleaseVelocity(this.panSamples);
          if (releaseVelocity && velocityMagnitude(releaseVelocity) >= MOMENTUM_START_SPEED) {
            this.momentumVelocity = releaseVelocity;
          }
        }
        this.panSamples = [];

        // Double-tap-to-zoom detection: only for taps that didn't drag, and
        // whose target is the stage itself (not a territory node, which
        // handles its own pointertap independently). A single background
        // tap is otherwise a no-op — it must never trigger anything else
        // (e.g. entering/exiting fullscreen is only ever done via an
        // explicit button, never a map tap).
        if (!this.dragSuppressesClick && e.target === stage) {
          const tap: TapRecord = { x: e.globalX, y: e.globalY, timeMs: Date.now() };
          if (isDoubleTap(this.lastTap, tap)) {
            this.zoomToward(this.camScale * DOUBLE_TAP_ZOOM_FACTOR, tap.x, tap.y);
            this.lastTap = null;
          } else {
            this.lastTap = tap;
          }
        }
      } else if (pointers.size === 1) {
        // One finger lifted during pinch — resume single-finger pan from here.
        const [remaining] = [...pointers.values()];
        beginPan(remaining.x, remaining.y);
      }
    };

    stage.on("pointerup", onPointerUp);
    stage.on("pointerupoutside", onPointerUp);

    // Desktop wheel-zoom, centered on the cursor.
    stage.on("wheel", (e: FederatedWheelEvent) => {
      e.preventDefault();
      const factor = Math.pow(WHEEL_ZOOM_FACTOR, -e.deltaY);
      this.zoomToward(this.camScale * factor, e.globalX, e.globalY);
    });

    this.canvas.style.cursor = "grab";
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Called once, right after scene creation, with the container's actual
   * current size. MapScene.create() may have been initialized with a
   * stale/fallback size if the container's true dimensions weren't
   * resolved yet at mount (e.g. a freshly-portaled fullscreen root that
   * was still 0×0 when measured, synchronously, before Pixi's async
   * WebGL/texture init even began). Unlike resize(), this redoes the
   * initial fitToViewport() (respecting initialFit) against the corrected
   * size rather than just clamping to a zoom floor — appropriate here
   * because no user gesture could have happened yet, so there's no
   * camera position to preserve.
   */
  applyInitialSize(width: number, height: number) {
    this.app.renderer.resize(width, height);
    this.fitToViewport();
  }

  /**
   * Resizes the renderer to the given size and immediately re-fits to the
   * "cover" scale (fills edge to edge, no letterboxing) in one atomic
   * step. Used when a persistent, shared scene is reparented into the
   * fullscreen shell: the container's new (larger) size needs to be
   * applied *and* the camera needs to auto-fill it, without waiting for
   * the ResizeObserver's async callback (which only clamps, never
   * re-fits, since normally a resize must preserve the user's camera).
   */
  enterFullscreenFit(width: number, height: number) {
    this.app.renderer.resize(width, height);
    this.applyFit("cover");
  }

  /**
   * Called by a ResizeObserver when the viewport container changes size.
   * Resizes the renderer, clamps the camera, and ensures zoom never falls
   * below the new fit scale. Deliberately preserves the current pan/zoom
   * (only clamping, never re-fitting) since by this point the user may
   * already have moved the camera — unlike applyInitialSize, a plain
   * resize (orientation change, mobile browser chrome show/hide) must not
   * reset it.
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
    playerColors: string[],
    highlight: TerritoryHighlightInput = {},
  ) {
    const { selectedSource, selectedTarget, legalTargets, recentCombat, recentCapture, passive } =
      highlight;

    for (const [name, node] of this.nodes) {
      const raw = territoryStates?.[name];
      const t =
        raw && typeof raw === "object" ? (raw as Record<string, unknown>) : null;
      const kind = classifyHighlight(
        name,
        selectedSource,
        selectedTarget,
        legalTargets,
        recentCombat,
        recentCapture,
        passive,
      );
      const state: TerritoryDisplayState = {
        owner: typeof t?.owner === "number" ? t.owner : -1,
        armies: typeof t?.armies === "number" ? t.armies : 0,
        highlight: kind,
      };
      node.update(state, playerColors);
    }

    this.drawConnector(selectedSource, selectedTarget);
  }

  private drawConnector(source?: string, target?: string) {
    this.connectorGfx.clear();
    if (!source || !target) return;
    const from = this.nodes.get(source);
    const to = this.nodes.get(target);
    if (!from || !to) return;
    this.connectorGfx
      .moveTo(from.position.x, from.position.y)
      .lineTo(to.position.x, to.position.y)
      .stroke({ color: 0xfbbf24, width: 3, alpha: 0.55 });
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
    this.app.ticker.remove(this.onTick);
    this.app.destroy(true);
  }
}

function classifyHighlight(
  name: string,
  selectedSource?: string,
  selectedTarget?: string,
  legalTargets?: ReadonlySet<string>,
  recentCombat?: ReadonlySet<string>,
  recentCapture?: ReadonlySet<string>,
  passive?: ReadonlySet<string>,
): TerritoryHighlightKind {
  // Precedence: the local user's own active selection always wins, then an
  // authoritative fresh-capture signal, then passive/legal/combat affordances.
  if (name === selectedSource) return "selected-source";
  if (name === selectedTarget) return "selected-target";
  if (recentCapture?.has(name)) return "recent-capture";
  if (legalTargets?.has(name)) return "legal-target";
  if (recentCombat?.has(name)) return "recent-combat";
  if (passive?.has(name)) return "passive";
  return "none";
}
