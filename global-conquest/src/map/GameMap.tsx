import { useEffect, useRef } from "react";
import type { GameBootstrap } from "../api/games";
import { MapScene } from "./MapScene";

interface GameMapProps {
  game: GameBootstrap | null;
  selectedTerritory: string;
  activeFrom: string;
  activeTo: string;
  playerColors: string[];
  onTerritoryClick: (name: string) => void;
}

export function GameMap({
  game,
  selectedTerritory,
  activeFrom,
  activeTo,
  playerColors,
  onTerritoryClick,
}: GameMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const sceneRef = useRef<MapScene | null>(null);

  // Always-current snapshot of props so the async init handler can apply
  // initial game state without needing to redeclare the init effect.
  const stateRef = useRef({ game, selectedTerritory, activeFrom, activeTo, playerColors });
  stateRef.current = { game, selectedTerritory, activeFrom, activeTo, playerColors };

  // Keep click handler in a ref so Pixi never captures a stale closure.
  const onClickRef = useRef(onTerritoryClick);
  onClickRef.current = onTerritoryClick;

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

    MapScene.create(w, h, (name) => onClickRef.current(name))
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

        // Apply current game state immediately — the state effect may have
        // already fired before init completed and found sceneRef still null.
        const s = stateRef.current;
        scene.updateTerritories(
          s.game?.territories ?? null,
          s.selectedTerritory,
          s.activeFrom,
          s.activeTo,
          s.playerColors,
        );
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
  }, []); // eslint-disable-line react-hooks/exhaustive-deps — intentionally mount-once

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

  // --- State sync: update territory visuals whenever game/selection changes ---
  useEffect(() => {
    sceneRef.current?.updateTerritories(
      game?.territories ?? null,
      selectedTerritory,
      activeFrom,
      activeTo,
      playerColors,
    );
  }, [game, selectedTerritory, activeFrom, activeTo, playerColors]);

  return (
    <div
      ref={containerRef}
      className="relative aspect-[2048/1367] w-full overflow-hidden rounded-xl border border-slate-200 bg-slate-900"
    />
  );
}
