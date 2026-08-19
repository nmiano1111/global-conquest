import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getGameBootstrap, getGameReplayEvents, type GameBootstrap, type GameReplayEntry } from "../../api/games";
import { GameMap } from "../../map/GameMap";
import { buttonGhostClass, buttonPrimaryClass } from "./styles";
import { MAP_PLAYER_COLORS, PHASE_BADGE_CLASS, PHASE_LABELS, type GameEventMessage } from "./gameShared";
import { applyGameStateUpdate, type GameStateUpdateResult } from "./applyGameStateUpdate";
import { MobileGameReplayView } from "./MobileGameReplayView";

// Playback speed presets, in ms between steps. "Instant" (0) still steps
// one action at a time rather than jumping straight to the end, so the
// sequence of visual change is preserved even at the fastest setting.
const SPEEDS: Array<{ label: string; ms: number }> = [
  { label: "0.5x", ms: 1500 },
  { label: "1x", ms: 700 },
  { label: "2x", ms: 300 },
  { label: "Instant", ms: 0 },
];

/**
 * Watches a game's action history play out, one committed action at a
 * time, using the exact same applyGameStateUpdate logic GamePage's live
 * socket handler uses — so a bot's attack, a capture, a card trade, all
 * look and animate here exactly as a live spectator would have seen them.
 *
 * This page intentionally never imports useSocket and never touches the
 * live `game` state GamePage owns: it fetches a static history snapshot
 * once, holds its own local state, and has no game-action submission UI
 * at all. There is no code path here that could submit a live game
 * action from this rewound state — leaving this page (navigating back to
 * /app/game/$gameID) re-mounts GamePage fresh, which re-bootstraps and
 * re-subscribes on its own.
 */
export function GameReplayPage() {
  const { gameID } = useParams({ from: "/app/game/$gameID/replay" });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [seedGame, setSeedGame] = useState<GameBootstrap | null>(null);
  const [events, setEvents] = useState<GameReplayEntry[]>([]);
  const [stepIndex, setStepIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speedIdx, setSpeedIdx] = useState(1);
  // Shares GamePage's "gc.mobile.ui" preference — a user who's set up
  // mobile view for live play should land in the mobile replay view too,
  // and the toggle here carries back the other way.
  const [mobileUI, setMobileUI] = useState<boolean>(() => {
    const stored = localStorage.getItem("gc.mobile.ui");
    if (stored !== null) return stored === "true";
    return typeof window !== "undefined" && window.innerWidth < 768;
  });
  const toggleMobileUI = () => {
    setMobileUI((prev) => {
      localStorage.setItem("gc.mobile.ui", String(!prev));
      return !prev;
    });
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError("");
      try {
        const [bootstrap, replay] = await Promise.all([getGameBootstrap(gameID), getGameReplayEvents(gameID)]);
        if (cancelled) return;
        setSeedGame(bootstrap);
        setEvents(replay);
        setStepIndex(0);
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        setError(apiErr.message || "Failed to load replay.");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [gameID]);

  // Every step's resulting board state and visual cues, precomputed once
  // per (seedGame, events) pair so scrubbing the timeline is instant —
  // no recomputation on every index change. Each step is applied on top
  // of the previous step's resulting game (or seedGame for the first),
  // exactly the same fold a series of live broadcasts would produce.
  const steps = useMemo<GameStateUpdateResult[]>(() => {
    if (!seedGame) return [];
    const out: GameStateUpdateResult[] = [];
    let prev = seedGame;
    for (const ev of events) {
      const applied = applyGameStateUpdate(prev, ev.payload);
      out.push(applied);
      prev = applied.nextGame;
    }
    return out;
  }, [seedGame, events]);

  const lastIndex = steps.length - 1;
  const clampedIndex = Math.min(stepIndex, Math.max(0, lastIndex));
  const current = steps[clampedIndex];
  const currentEntry = events[clampedIndex];

  // Autoplay: advances one step every SPEEDS[speedIdx].ms while playing,
  // stopping automatically at the last step rather than looping.
  useEffect(() => {
    if (!playing || steps.length === 0) return;
    if (clampedIndex >= lastIndex) {
      setPlaying(false);
      return;
    }
    const ms = SPEEDS[speedIdx].ms;
    const timer = window.setTimeout(() => setStepIndex((i) => Math.min(i + 1, lastIndex)), Math.max(ms, 16));
    return () => window.clearTimeout(timer);
  }, [playing, speedIdx, clampedIndex, lastIndex, steps.length]);

  const players = useMemo(() => current?.nextGame.players ?? seedGame?.players ?? [], [current, seedGame]);
  const playerColors = useMemo(
    () => players.map((p, i) => p.color || MAP_PLAYER_COLORS[i % MAP_PLAYER_COLORS.length]),
    [players]
  );
  const eventColorByActorID = useMemo(() => {
    const out: Record<string, string> = {};
    players.forEach((p, i) => {
      if (!p.userId) return;
      out[p.userId] = p.color || MAP_PLAYER_COLORS[i % MAP_PLAYER_COLORS.length];
    });
    return out;
  }, [players]);

  const highlightedTerritories = useMemo(() => {
    const set = new Set<string>();
    if (!current) return set;
    if (current.lastActionTerritory) set.add(current.lastActionTerritory);
    if (current.lastActionFrom) set.add(current.lastActionFrom);
    if (current.lastActionTo) set.add(current.lastActionTo);
    return set;
  }, [current]);

  const recentCombatTerritories = useMemo(() => {
    const set = new Set<string>();
    if (!current || current.lastActionType !== "attack") return set;
    if (current.lastActionFrom) set.add(current.lastActionFrom);
    if (current.lastActionTo) set.add(current.lastActionTo);
    return set;
  }, [current]);

  const recentCaptureTerritories = useMemo(() => {
    const set = new Set<string>();
    if (current?.nextGame.phase === "occupy" && current.nextGame.occupy) {
      set.add(current.nextGame.occupy.to);
    }
    return set;
  }, [current]);

  // Every step's log line up to and including the current one — scrubbing
  // backward hides later events, matching "watching up to this point."
  const visibleEvents = useMemo<GameEventMessage[]>(() => {
    const out: GameEventMessage[] = [];
    for (let i = 0; i <= clampedIndex && i < steps.length; i++) {
      const ev = steps[i].eventMessage;
      if (ev) out.push(ev);
    }
    return out;
  }, [steps, clampedIndex]);

  const actorName = useMemo(() => {
    if (!currentEntry?.actorPlayerId) return "";
    return players.find((p) => p.userId === currentEntry.actorPlayerId)?.userName || currentEntry.actorPlayerId;
  }, [players, currentEntry]);

  const phase = current?.nextGame.phase ?? "";

  const jumpToStart = () => {
    setPlaying(false);
    setStepIndex(0);
  };
  const stepBack = () => {
    setPlaying(false);
    setStepIndex((i) => Math.max(0, i - 1));
  };
  const togglePlay = () => setPlaying((p) => !p);
  const stepForward = () => {
    setPlaying(false);
    setStepIndex((i) => Math.min(lastIndex, i + 1));
  };
  const jumpToEnd = () => {
    setPlaying(false);
    setStepIndex(lastIndex);
  };
  const scrubTo = (i: number) => {
    setPlaying(false);
    setStepIndex(i);
  };

  if (mobileUI) {
    return (
      <MobileGameReplayView
        gameID={gameID}
        game={current?.nextGame ?? seedGame}
        loading={loading}
        error={error}
        phase={phase}
        stepIndex={clampedIndex}
        totalSteps={steps.length}
        actorName={actorName}
        actionType={currentEntry?.actionType ?? ""}
        diceResult={current?.diceResult ?? null}
        visibleEvents={visibleEvents}
        eventColorByActorID={eventColorByActorID}
        playing={playing}
        speedLabel={SPEEDS[speedIdx].label}
        speedOptions={SPEEDS.map((s, i) => ({ label: s.label, idx: i }))}
        onSetSpeedIdx={setSpeedIdx}
        onTogglePlay={togglePlay}
        onJumpStart={jumpToStart}
        onStepBack={stepBack}
        onStepForward={stepForward}
        onJumpEnd={jumpToEnd}
        onScrub={scrubTo}
        canStepBack={steps.length > 0 && clampedIndex > 0}
        canStepForward={steps.length > 0 && clampedIndex < lastIndex}
        highlightedTerritories={highlightedTerritories}
        recentCombat={recentCombatTerritories}
        recentCapture={recentCaptureTerritories}
        playerColors={playerColors}
        onToggleDesktop={toggleMobileUI}
        currentEntry={currentEntry}
      />
    );
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(280px,1fr)]">
      <section className="grid gap-4">
        <header className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gc-border bg-gc-surface px-4 py-3">
          <div>
            <h2 className="text-base font-semibold text-gc-text">{seedGame?.name || "Replay"}</h2>
            <p className="font-mono text-xs text-gc-muted">{gameID}</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full bg-gc-surface-2 px-2 py-1 text-xs font-semibold uppercase tracking-wide text-gc-muted">
              Watching replay
            </span>
            <button className={buttonGhostClass} type="button" onClick={toggleMobileUI}>
              Mobile View
            </button>
            <Link className={buttonGhostClass} to="/app/game/$gameID" params={{ gameID }}>
              ← Back to game
            </Link>
          </div>
        </header>

        {loading ? <p className="text-sm text-gc-muted">Loading replay…</p> : null}
        {error ? (
          <p className="rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {error}
          </p>
        ) : null}
        {!loading && !error && steps.length === 0 ? (
          <p className="rounded-lg border border-gc-border bg-gc-surface px-3 py-2 text-sm text-gc-muted">
            No replay data is available for this game yet.
          </p>
        ) : null}

        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-gc-text">Map</h3>
            {phase ? (
              <span
                className={`rounded-full px-2 py-1 text-xs font-semibold text-white ${PHASE_BADGE_CLASS[phase] || "bg-gc-surface-2"}`}
              >
                {PHASE_LABELS[phase] || phase}
              </span>
            ) : null}
          </div>
          <div className="relative aspect-[2048/1367] w-full overflow-hidden rounded-xl border border-slate-200 bg-slate-900">
            <GameMap
              game={current?.nextGame ?? seedGame}
              selectedTerritory=""
              activeFrom=""
              activeTo=""
              highlightedTerritories={highlightedTerritories}
              recentCombat={recentCombatTerritories}
              recentCapture={recentCaptureTerritories}
              playerColors={playerColors}
              onTerritoryClick={() => undefined}
              className="absolute inset-0"
            />
          </div>
        </section>

        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-sm text-gc-muted">
            <span>
              Action {steps.length === 0 ? 0 : clampedIndex + 1} of {steps.length}
            </span>
            {currentEntry ? (
              <span className="capitalize">
                {actorName ? `${actorName} — ` : ""}
                {currentEntry.actionType.replaceAll("_", " ")}
              </span>
            ) : null}
          </div>
          <input
            type="range"
            min={0}
            max={Math.max(0, lastIndex)}
            value={clampedIndex}
            disabled={steps.length === 0}
            onChange={(e) => scrubTo(Number(e.target.value))}
            className="w-full"
          />
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <button className={buttonGhostClass} type="button" disabled={steps.length === 0} onClick={jumpToStart}>
              ⏮ Start
            </button>
            <button
              className={buttonGhostClass}
              type="button"
              disabled={steps.length === 0 || clampedIndex === 0}
              onClick={stepBack}
            >
              ◀ Step
            </button>
            <button className={buttonPrimaryClass} type="button" disabled={steps.length === 0} onClick={togglePlay}>
              {playing ? "⏸ Pause" : "▶ Play"}
            </button>
            <button
              className={buttonGhostClass}
              type="button"
              disabled={steps.length === 0 || clampedIndex >= lastIndex}
              onClick={stepForward}
            >
              Step ▶
            </button>
            <button className={buttonGhostClass} type="button" disabled={steps.length === 0} onClick={jumpToEnd}>
              End ⏭
            </button>
            <select
              className="ml-auto rounded-lg border border-gc-border bg-gc-surface px-2 py-1.5 text-sm text-gc-text"
              value={speedIdx}
              onChange={(e) => setSpeedIdx(Number(e.target.value))}
            >
              {SPEEDS.map((s, i) => (
                <option key={s.label} value={i}>
                  {s.label}
                </option>
              ))}
            </select>
          </div>
        </section>
      </section>

      <section className="grid gap-4">
        {current?.diceResult ? (
          <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
            <h3 className="mb-3 text-sm font-semibold text-gc-text">Combat</h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <p className="text-xs uppercase tracking-wide text-gc-muted">Attacker</p>
                <p className="text-lg font-bold text-gc-text">{current.diceResult.attacker.join(" · ")}</p>
                <p className="text-xs text-gc-danger">−{current.diceResult.attackerLoss}</p>
              </div>
              <div>
                <p className="text-xs uppercase tracking-wide text-gc-muted">Defender</p>
                <p className="text-lg font-bold text-gc-text">{current.diceResult.defender.join(" · ")}</p>
                <p className="text-xs text-gc-success">−{current.diceResult.defenderLoss}</p>
              </div>
            </div>
          </section>
        ) : null}

        <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-gc-text">Event Log</h3>
            <span className="text-xs text-gc-muted capitalize">{phase || "—"}</span>
          </div>
          <div className="h-[320px] overflow-y-auto rounded-lg border border-gc-border bg-gc-surface-2 p-3 text-sm text-gc-muted">
            {visibleEvents.length === 0 ? <p>No events yet.</p> : null}
            <ul className="grid gap-2">
              {visibleEvents.map((ev, idx) => {
                const eventColor = eventColorByActorID[ev.actorUserID] ?? "#687a91";
                return (
                  <li key={`${ev.id}-${idx}`} className="rounded-lg bg-gc-surface px-2 py-1.5">
                    <div className="mb-1 flex items-center justify-between gap-2 text-xs" style={{ color: eventColor }}>
                      <span className="font-semibold capitalize">{ev.eventType.replaceAll("_", " ")}</span>
                    </div>
                    <p className="text-sm font-medium" style={{ color: eventColor }}>
                      {ev.body}
                    </p>
                  </li>
                );
              })}
            </ul>
          </div>
        </section>
      </section>
    </div>
  );
}
