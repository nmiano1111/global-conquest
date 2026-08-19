import { useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import type { GameBootstrap, GameReplayEntry } from "../../api/games";
import { GameMap, type GameMapHandle } from "../../map/GameMap";
import { PHASE_BADGE_CLASS, PHASE_LABELS, type DiceRollResult, type GameEventMessage } from "./gameShared";

export interface MobileGameReplayViewProps {
  gameID: string;
  game: GameBootstrap | null;
  loading: boolean;
  error: string;
  phase: string;
  stepIndex: number;
  totalSteps: number;
  actorName: string;
  actionType: string;
  diceResult: DiceRollResult | null;
  visibleEvents: GameEventMessage[];
  eventColorByActorID: Record<string, string>;
  playing: boolean;
  speedLabel: string;
  speedOptions: Array<{ label: string; idx: number }>;
  onSetSpeedIdx: (idx: number) => void;
  onTogglePlay: () => void;
  onJumpStart: () => void;
  onStepBack: () => void;
  onStepForward: () => void;
  onJumpEnd: () => void;
  onScrub: (index: number) => void;
  canStepBack: boolean;
  canStepForward: boolean;
  highlightedTerritories: Set<string>;
  recentCombat: Set<string>;
  recentCapture: Set<string>;
  playerColors: string[];
  onToggleDesktop: () => void;
  currentEntry?: GameReplayEntry;
}

/**
 * Dedicated mobile replay view — a fixed-fullscreen overlay (same pattern
 * as MobileGameView) rather than the desktop two-column layout squeezed
 * into a narrow viewport. Read-only: map, playback controls, and the
 * event log, no action panel/cards/chat — those don't apply to replay.
 */
export function MobileGameReplayView({
  gameID,
  game,
  loading,
  error,
  phase,
  stepIndex,
  totalSteps,
  actorName,
  actionType,
  diceResult,
  visibleEvents,
  eventColorByActorID,
  playing,
  speedLabel,
  speedOptions,
  onSetSpeedIdx,
  onTogglePlay,
  onJumpStart,
  onStepBack,
  onStepForward,
  onJumpEnd,
  onScrub,
  canStepBack,
  canStepForward,
  highlightedTerritories,
  recentCombat,
  recentCapture,
  playerColors,
  onToggleDesktop,
  currentEntry,
}: MobileGameReplayViewProps) {
  const mapRef = useRef<GameMapHandle>(null);
  const eventScrollRef = useRef<HTMLDivElement>(null);
  const phaseBadgeClass = PHASE_BADGE_CLASS[phase] || "bg-slate-600";
  const phaseLabel = PHASE_LABELS[phase] || phase || "—";

  useEffect(() => {
    const el = eventScrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [visibleEvents]);

  return (
    <div
      className="fixed inset-0 z-40 flex flex-col overflow-hidden bg-slate-900 text-white"
      style={{ height: "100dvh" }}
    >
      {/* ── Top bar ── */}
      <header
        className="flex shrink-0 items-center gap-2 border-b border-slate-700 bg-slate-800 px-3 py-2"
        style={{ paddingTop: "calc(env(safe-area-inset-top) + 0.5rem)" }}
      >
        <Link
          to="/app/game/$gameID"
          params={{ gameID }}
          className="shrink-0 rounded-lg bg-slate-700 px-2.5 py-1.5 text-xs font-medium text-slate-300 active:bg-slate-600"
        >
          ← Back
        </Link>

        <div className="flex min-w-0 flex-1 flex-col items-center">
          <div className="flex items-center gap-2">
            <span className={`rounded-full px-2.5 py-0.5 text-xs font-bold text-white ${phaseBadgeClass}`}>
              {phaseLabel}
            </span>
            <span className="truncate text-xs font-semibold text-amber-300">Watching Replay</span>
          </div>
        </div>

        <button
          type="button"
          onClick={onToggleDesktop}
          className="shrink-0 rounded-lg bg-slate-700 px-2.5 py-1.5 text-xs font-medium text-slate-300 active:bg-slate-600"
        >
          Desktop
        </button>
      </header>

      {error ? (
        <div className="shrink-0 border-b border-rose-800 bg-rose-900/80 px-4 py-2 text-xs text-rose-200">
          {error}
        </div>
      ) : null}
      {loading ? (
        <div className="shrink-0 border-b border-slate-700 bg-slate-800/80 px-4 py-2 text-xs text-slate-300">
          Loading replay…
        </div>
      ) : null}

      {/* ── Map ── */}
      <div className="relative min-h-0 flex-1 overflow-hidden bg-slate-900">
        <GameMap
          ref={mapRef}
          game={game}
          selectedTerritory=""
          activeFrom=""
          activeTo=""
          highlightedTerritories={highlightedTerritories}
          recentCombat={recentCombat}
          recentCapture={recentCapture}
          playerColors={playerColors}
          onTerritoryClick={() => undefined}
          className="absolute inset-0"
        />
        <div className="absolute right-2.5 top-2.5 flex flex-col gap-1.5">
          <button
            type="button"
            onClick={() => mapRef.current?.zoomIn()}
            aria-label="Zoom in"
            className="flex h-9 w-9 items-center justify-center rounded-full bg-slate-800/80 text-base font-bold text-slate-200 shadow-lg backdrop-blur active:bg-slate-700"
          >
            +
          </button>
          <button
            type="button"
            onClick={() => mapRef.current?.resetZoom()}
            aria-label="Reset zoom"
            className="flex h-9 w-9 items-center justify-center rounded-full bg-slate-800/80 text-sm text-slate-200 shadow-lg backdrop-blur active:bg-slate-700"
          >
            ⌖
          </button>
          <button
            type="button"
            onClick={() => mapRef.current?.zoomOut()}
            aria-label="Zoom out"
            className="flex h-9 w-9 items-center justify-center rounded-full bg-slate-800/80 text-base font-bold text-slate-200 shadow-lg backdrop-blur active:bg-slate-700"
          >
            −
          </button>
        </div>
      </div>

      {/* ── Bottom sheet: playback controls + log ── */}
      <div
        className="flex shrink-0 flex-col border-t border-slate-700 bg-slate-800"
        style={{ height: "calc(300px + env(safe-area-inset-bottom))", paddingBottom: "env(safe-area-inset-bottom)" }}
      >
        <div className="shrink-0 border-b border-slate-700 px-3 py-2">
          <div className="mb-1.5 flex items-center justify-between text-xs text-slate-400">
            <span>
              Action {totalSteps === 0 ? 0 : stepIndex + 1} of {totalSteps}
            </span>
            {currentEntry ? (
              <span className="truncate text-right">
                {actorName ? `${actorName} — ` : ""}
                {actionType.replaceAll("_", " ")}
              </span>
            ) : null}
          </div>
          <input
            type="range"
            min={0}
            max={Math.max(0, totalSteps - 1)}
            value={stepIndex}
            disabled={totalSteps === 0}
            onChange={(e) => onScrub(Number(e.target.value))}
            className="w-full"
          />
          <div className="mt-2 flex items-center gap-1.5">
            <button
              type="button"
              onClick={onJumpStart}
              disabled={totalSteps === 0}
              className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-700 text-sm text-slate-200 disabled:opacity-40 active:bg-slate-600"
              aria-label="Jump to start"
            >
              ⏮
            </button>
            <button
              type="button"
              onClick={onStepBack}
              disabled={!canStepBack}
              className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-700 text-sm text-slate-200 disabled:opacity-40 active:bg-slate-600"
              aria-label="Step back"
            >
              ◀
            </button>
            <button
              type="button"
              onClick={onTogglePlay}
              disabled={totalSteps === 0}
              className="flex h-8 flex-1 items-center justify-center rounded-lg bg-indigo-600 text-xs font-bold text-white disabled:opacity-40 active:bg-indigo-500"
            >
              {playing ? "⏸ Pause" : "▶ Play"}
            </button>
            <button
              type="button"
              onClick={onStepForward}
              disabled={!canStepForward}
              className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-700 text-sm text-slate-200 disabled:opacity-40 active:bg-slate-600"
              aria-label="Step forward"
            >
              ▶
            </button>
            <button
              type="button"
              onClick={onJumpEnd}
              disabled={totalSteps === 0}
              className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-700 text-sm text-slate-200 disabled:opacity-40 active:bg-slate-600"
              aria-label="Jump to end"
            >
              ⏭
            </button>
            <select
              value={speedLabel}
              onChange={(e) => {
                const opt = speedOptions.find((o) => o.label === e.target.value);
                if (opt) onSetSpeedIdx(opt.idx);
              }}
              className="h-8 shrink-0 rounded-lg border border-slate-600 bg-slate-700 px-1.5 text-xs text-slate-200"
            >
              {speedOptions.map((o) => (
                <option key={o.label} value={o.label}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {diceResult ? (
          <div className="flex shrink-0 items-center justify-around border-b border-slate-700 px-3 py-2 text-xs">
            <div className="text-center">
              <p className="text-slate-400">Attacker</p>
              <p className="font-bold text-white">{diceResult.attacker.join(" · ")}</p>
              <p className="text-rose-400">−{diceResult.attackerLoss}</p>
            </div>
            <div className="text-center">
              <p className="text-slate-400">Defender</p>
              <p className="font-bold text-white">{diceResult.defender.join(" · ")}</p>
              <p className="text-emerald-400">−{diceResult.defenderLoss}</p>
            </div>
          </div>
        ) : null}

        <div ref={eventScrollRef} className="min-h-0 flex-1 overflow-y-auto px-3 py-2">
          {visibleEvents.length === 0 ? <p className="text-xs text-slate-500">No events yet.</p> : null}
          <ul className="grid gap-1.5">
            {visibleEvents.map((ev, idx) => {
              const color = eventColorByActorID[ev.actorUserID] ?? "#94a3b8";
              return (
                <li key={`${ev.id}-${idx}`} className="rounded-lg bg-slate-700/60 px-2 py-1.5">
                  <p className="text-xs font-medium" style={{ color }}>
                    {ev.body}
                  </p>
                </li>
              );
            })}
          </ul>
        </div>
      </div>
    </div>
  );
}
