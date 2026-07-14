import { forwardRef, useEffect, useImperativeHandle, useLayoutEffect, useRef } from "react";
import type { GameBootstrap } from "../api/games";
import { MapScene } from "./MapScene";

interface GameMapProps {
  game: GameBootstrap | null;
  /** Primary local selection: reinforce target / attack-from / fortify-from. */
  selectedTerritory: string;
  activeFrom: string;
  activeTo: string;
  playerColors: string[];
  onTerritoryClick: (name: string) => void;
  /**
   * Territories that are legal targets for the current selection (e.g.
   * attackable enemy neighbors of the selected attacker). Purely a visual
   * affordance derived from data already public to the client — never used
   * to gate the actual command, which the backend still validates.
   */
  legalTargets?: ReadonlySet<string>;
  /** Territories involved in the most recently resolved combat. */
  recentCombat?: ReadonlySet<string>;
  /** Territory just captured and awaiting an occupation move. */
  recentCapture?: ReadonlySet<string>;
  /**
   * Passive highlight for territories not covered by the local user's own
   * selection — the most recently committed bot action, and other players'
   * live territory presses relayed over the socket. Kept separate from
   * selectedTerritory/activeFrom/activeTo since those also drive form
   * logic (e.g. attack-panel gating); this is purely visual.
   */
  highlightedTerritories?: ReadonlySet<string>;
  /**
   * Fired when the user taps empty map background — not a territory, and
   * not the end of a pan/pinch drag. Used to offer "tap the map to expand
   * to fullscreen" without conflicting with territory selection.
   */
  onBackgroundTap?: () => void;
  /** Overrides the default embedded sizing (aspect-ratio box). Fullscreen mode passes an absolute-fill className instead. */
  className?: string;
  /**
   * Initial/reset zoom fit. "contain" (default) shows the whole board,
   * matching today's embedded/mobile behavior. "cover" fills the viewport
   * edge to edge with no letterboxing, cropping the board's edges — used
   * by fullscreen mode so the map reads as large as possible on open;
   * zooming out is still possible down to the "contain" scale to see the
   * whole board, regardless of this setting.
   */
  initialFit?: "contain" | "cover";
}

const DEFAULT_CLASS_NAME =
  "relative aspect-[2048/1367] w-full overflow-hidden rounded-xl border border-slate-200 bg-slate-900";

export interface GameMapHandle {
  zoomIn: () => void;
  zoomOut: () => void;
  resetZoom: () => void;
}

const EMPTY_HIGHLIGHT: ReadonlySet<string> = new Set();

export const GameMap = forwardRef<GameMapHandle, GameMapProps>(function GameMap({
  game,
  selectedTerritory,
  activeFrom,
  activeTo,
  playerColors,
  onTerritoryClick,
  legalTargets = EMPTY_HIGHLIGHT,
  recentCombat = EMPTY_HIGHLIGHT,
  recentCapture = EMPTY_HIGHLIGHT,
  highlightedTerritories = EMPTY_HIGHLIGHT,
  onBackgroundTap,
  className,
  initialFit = "contain",
}, ref) {
  const containerRef = useRef<HTMLDivElement>(null);
  const sceneRef = useRef<MapScene | null>(null);

  // Always-current snapshot of props so the async init handler can apply
  // initial game state without needing to redeclare the init effect. Kept
  // in sync via useLayoutEffect (not a render-body assignment) so it's
  // current before any subsequent effect or async callback reads it.
  const stateRef = useRef({
    game,
    selectedTerritory,
    activeFrom,
    activeTo,
    playerColors,
    legalTargets,
    recentCombat,
    recentCapture,
    highlightedTerritories,
  });
  const onClickRef = useRef(onTerritoryClick);
  const onBackgroundTapRef = useRef(onBackgroundTap);
  useLayoutEffect(() => {
    stateRef.current = {
      game,
      selectedTerritory,
      activeFrom,
      activeTo,
      playerColors,
      legalTargets,
      recentCombat,
      recentCapture,
      highlightedTerritories,
    };
    onClickRef.current = onTerritoryClick;
    onBackgroundTapRef.current = onBackgroundTap;
  });

  useImperativeHandle(ref, () => ({
    zoomIn: () => sceneRef.current?.zoomIn(),
    zoomOut: () => sceneRef.current?.zoomOut(),
    resetZoom: () => sceneRef.current?.resetZoom(),
  }));

  // --- Mount: create Pixi scene ---
  //
  // PixiJS creates its own canvas element here (no canvas prop). This is
  // intentional: React StrictMode mounts → unmounts → remounts each effect.
  // If we reused the same <canvas> ref, the second init would receive a canvas
  // whose WebGL context was already corrupted by destroy(), causing a null
  // shader source crash. By letting Pixi own the canvas, each init gets a
  // brand-new element with a clean context.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let mounted = true;
    const w = container.offsetWidth || 800;
    const h = container.offsetHeight || 534;

    MapScene.create(w, h, (name) => onClickRef.current(name), initialFit)
      .then((scene) => {
        if (!mounted) {
          scene.destroy();
          return;
        }
        // Style the Pixi canvas to fill our container absolutely
        const canvas = scene.canvas;
        canvas.style.position = "absolute";
        canvas.style.inset = "0";
        canvas.style.borderRadius = "inherit";
        container.appendChild(canvas);

        sceneRef.current = scene;
        scene.setOnBackgroundTap(() => onBackgroundTapRef.current?.());

        // Re-measure now rather than trusting the w/h captured before this
        // async init started: Pixi's WebGL context + texture load can take
        // long enough that a container whose size wasn't yet stable at
        // mount (e.g. a freshly-portaled fullscreen root) has since
        // resolved to its real size. The ResizeObserver below normally
        // handles this, but its very first callback can fire before
        // sceneRef.current exists yet (a no-op), and it only fires again
        // on subsequent size *changes* — which never happens if the
        // container was already stable. applyInitialSize (not resize)
        // redoes the initial fit-to-viewport for the corrected size,
        // since resize() alone only clamps to a zoom floor and would
        // otherwise leave the camera fit to the stale fallback dimensions.
        if (container.offsetWidth && container.offsetHeight) {
          scene.applyInitialSize(container.offsetWidth, container.offsetHeight);
        }

        // Apply current game state immediately — the state effect may have
        // already fired before init completed and found sceneRef still null.
        const s = stateRef.current;
        scene.updateTerritories(s.game?.territories ?? null, s.playerColors, {
          selectedSource: s.activeFrom || s.selectedTerritory,
          selectedTarget: s.activeTo,
          legalTargets: s.legalTargets,
          recentCombat: s.recentCombat,
          recentCapture: s.recentCapture,
          passive: s.highlightedTerritories,
        });
      })
      .catch(console.error);

    return () => {
      mounted = false;
      if (sceneRef.current) {
        const canvas = sceneRef.current.canvas;
        // Remove canvas from DOM before destroying; destroy(true) then frees GPU
        if (canvas.parentNode === container) {
          container.removeChild(canvas);
        }
        sceneRef.current.destroy();
        sceneRef.current = null;
      }
    };
    // Intentionally mount-once: onClickRef/onBackgroundTapRef stay current via the
    // layout effect above, and initialFit is a one-time creation setting, not
    // something a change to should tear down and recreate the Pixi scene for.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --- Resize: keep renderer and world scale in sync with container ---
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        sceneRef.current?.resize(width, height);
      }
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  // --- Background-tap callback: keep the scene's handler current ---
  useEffect(() => {
    sceneRef.current?.setOnBackgroundTap(onBackgroundTap ? () => onBackgroundTap() : null);
  }, [onBackgroundTap]);

  // --- State sync: update territory visuals whenever game/selection changes ---
  useEffect(() => {
    sceneRef.current?.updateTerritories(game?.territories ?? null, playerColors, {
      selectedSource: activeFrom || selectedTerritory,
      selectedTarget: activeTo,
      legalTargets,
      recentCombat,
      recentCapture,
      passive: highlightedTerritories,
    });
  }, [
    game,
    selectedTerritory,
    activeFrom,
    activeTo,
    playerColors,
    legalTargets,
    recentCombat,
    recentCapture,
    highlightedTerritories,
  ]);

  return (
    <div
      ref={containerRef}
      className={className ?? DEFAULT_CLASS_NAME}
      style={{ touchAction: "none" }}
    />
  );
});
