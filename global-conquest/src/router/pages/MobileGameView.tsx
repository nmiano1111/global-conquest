import { useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import type { Card, GameBootstrap } from "../../api/games";
import { GameMap, type GameMapHandle } from "../../map/GameMap";
import type { DiceRollResult, GameChatMessage, GameEventMessage } from "./gameShared";

type OccupyRequirement = NonNullable<GameBootstrap["occupy"]>;
type Player = GameBootstrap["players"][number];

export interface MobileGameViewProps {
  game: GameBootstrap | null;
  loading: boolean;
  error: string;
  actionError: string;
  chatMessages: GameChatMessage[];
  eventMessages: GameEventMessage[];
  chatDraft: string;
  chatError: string;
  wsStatus: string;
  gameID: string;

  phase: string;
  phaseMode: string;
  meIndex: number;
  isMyTurn: boolean;
  canEnterAttack: boolean;
  players: Player[];
  playerColors: string[];
  territoryState: Record<string, unknown> | null;
  myCards: Card[];
  selectedCardIndices: number[];
  mySetupArmies: number;
  nextTradeBonus: number;
  pendingReinforcements: number;
  occupyRequirement: OccupyRequirement | null;
  diceResult: DiceRollResult | null;

  selectedTerritory: string;
  activeFrom: string;
  activeTo: string;
  armiesInput: number;
  clampedArmiesInput: number;
  clampedAttackerDice: number;
  minArmiesInput: number;
  maxArmiesInput: number;
  maxAttackDiceAllowed: number;
  maxDefendDiceAllowed: number;
  canAttackSelection: boolean;

  renderEventBody: (body: string) => React.ReactNode;
  onMapTerritoryClick: (name: string) => void;
  commitReinforcement: () => void;
  commitFortify: () => void;
  commitOccupy: () => void;
  commitTradeCards: () => void;
  toggleCardSelection: (idx: number) => void;
  onRollDice: () => void;
  setAttackerDice: (n: number) => void;
  setArmiesInput: (n: number) => void;
  setChatDraft: (s: string) => void;
  onSendChat: (e: React.FormEvent<HTMLFormElement>) => void;
  sendAction: (payload: Record<string, unknown>) => void;
  setSelectedFrom: (s: string) => void;
  setSelectedTo: (s: string) => void;
  setSelectedTerritory: (s: string) => void;
  onRefresh: () => void;
  onToggleDesktop: () => void;
}

type Tab = "actions" | "cards" | "events" | "chat";

const PHASE_LABELS: Record<string, string> = {
  setup_reinforce: "Setup",
  reinforce: "Reinforce",
  attack: "Attack",
  occupy: "Occupy",
  fortify: "Fortify",
};

const PHASE_BADGE: Record<string, string> = {
  setup_reinforce: "bg-sky-600",
  reinforce: "bg-emerald-600",
  attack: "bg-rose-600",
  occupy: "bg-amber-500",
  fortify: "bg-violet-600",
};

function symbolIcon(s: string) {
  if (s === "infantry") return "🪖";
  if (s === "cavalry") return "🐴";
  if (s === "artillery") return "💣";
  return "⭐";
}

export function MobileGameView(props: MobileGameViewProps) {
  const {
    game, loading, error, actionError, chatMessages, eventMessages,
    chatDraft, chatError, wsStatus,
    phase, phaseMode, meIndex, isMyTurn,
    players, playerColors, territoryState, myCards, selectedCardIndices,
    mySetupArmies, nextTradeBonus, pendingReinforcements, occupyRequirement,
    diceResult, selectedTerritory, activeFrom, activeTo,
    clampedArmiesInput, clampedAttackerDice,
    minArmiesInput, maxArmiesInput, maxAttackDiceAllowed, maxDefendDiceAllowed,
    canAttackSelection, renderEventBody, onMapTerritoryClick,
    commitReinforcement, commitFortify, commitOccupy, commitTradeCards,
    toggleCardSelection, onRollDice, setAttackerDice, setArmiesInput,
    setChatDraft, onSendChat, sendAction, setSelectedFrom, setSelectedTo,
    setSelectedTerritory, onToggleDesktop,
  } = props;

  const [activeTab, setActiveTab] = useState<Tab>("actions");
  const chatScrollRef = useRef<HTMLDivElement>(null);
  const eventScrollRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<GameMapHandle>(null);

  const currentPlayerIndex = game?.currentPlayer ?? -1;
  const currentPlayer = players[currentPlayerIndex];
  const currentPlayerColor = playerColors[currentPlayerIndex] ?? "#94a3b8";
  const phaseLabel = PHASE_LABELS[phase] ?? phase;
  const phaseBadgeClass = PHASE_BADGE[phase] ?? "bg-slate-600";

  const getTerritoryInfo = (name: string) => {
    if (!name || !territoryState) return null;
    const raw = territoryState[name];
    const t = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : null;
    if (!t) return null;
    const owner = typeof t.owner === "number" ? t.owner : -1;
    const armies = typeof t.armies === "number" ? t.armies : 0;
    return {
      owner,
      armies,
      ownerName: owner >= 0 ? (players[owner]?.userName ?? `P${owner + 1}`) : "?",
      ownerColor: owner >= 0 ? (playerColors[owner] ?? "#94a3b8") : "#94a3b8",
    };
  };

  const fromInfo = getTerritoryInfo(activeFrom);
  const toInfo = getTerritoryInfo(activeTo);
  const selInfo = getTerritoryInfo(selectedTerritory);

  // Reusable army stepper — rendered inline (not a component) to avoid identity issues
  const renderArmyStepper = () => (
    <div className="flex items-center justify-center gap-4">
      <button
        type="button"
        onClick={() => setArmiesInput(Math.max(minArmiesInput, clampedArmiesInput - 1))}
        className="flex h-11 w-11 items-center justify-center rounded-full bg-slate-700 text-2xl font-bold text-white active:bg-slate-600"
      >
        −
      </button>
      <span className="w-14 text-center text-3xl font-black text-white tabular-nums">
        {clampedArmiesInput}
      </span>
      <button
        type="button"
        onClick={() => setArmiesInput(Math.min(maxArmiesInput, clampedArmiesInput + 1))}
        className="flex h-11 w-11 items-center justify-center rounded-full bg-slate-700 text-2xl font-bold text-white active:bg-slate-600"
      >
        +
      </button>
      <span className="text-xs text-slate-500">max {maxArmiesInput}</span>
    </div>
  );

  // Territory pair display (from → to) used in attack and fortify
  const renderTerritoryPair = (
    fromLabel: string,
    toLabel: string,
    icon: string,
  ) => (
    <div className="grid grid-cols-[1fr_28px_1fr] items-center gap-1">
      <div className={`rounded-xl px-2 py-2 text-center ${activeFrom ? "bg-slate-700" : "border border-dashed border-slate-600 bg-slate-800"}`}>
        <p className="text-[9px] uppercase tracking-widest text-slate-500">{fromLabel}</p>
        {activeFrom ? (
          <>
            <p className="mt-0.5 truncate text-xs font-bold text-white">{activeFrom}</p>
            {fromInfo && (
              <p className="text-[10px]" style={{ color: fromInfo.ownerColor }}>
                {fromInfo.armies} armies
              </p>
            )}
          </>
        ) : (
          <p className="mt-0.5 text-xs text-slate-500">Tap map</p>
        )}
      </div>
      <p className="text-center text-base text-slate-500">{icon}</p>
      <div className={`rounded-xl px-2 py-2 text-center ${activeTo ? "bg-slate-700" : "border border-dashed border-slate-600 bg-slate-800"}`}>
        <p className="text-[9px] uppercase tracking-widest text-slate-500">{toLabel}</p>
        {activeTo ? (
          <>
            <p className="mt-0.5 truncate text-xs font-bold text-white">{activeTo}</p>
            {toInfo && (
              <p className="text-[10px]" style={{ color: toInfo.ownerColor }}>
                {toInfo.armies} armies
              </p>
            )}
          </>
        ) : (
          <p className="mt-0.5 text-xs text-slate-500">Tap map</p>
        )}
      </div>
    </div>
  );

  // ── Actions tab content ─────────────────────────────────────────────────────

  const renderActionsTab = () => {
    if (loading) return <p className="p-4 text-sm text-slate-400">Loading…</p>;
    if (error) return <p className="p-4 text-sm text-rose-400">{error}</p>;
    if (!game) return null;

    if (phaseMode === "setup_reinforce") {
      return (
        <div className="grid gap-3 p-4">
          <div className="rounded-xl border border-sky-700 bg-sky-900/40 p-3">
            <p className="text-sm font-semibold text-sky-300">Initial Placement</p>
            <p className="mt-1 text-xs text-sky-400">
              {mySetupArmies > 0
                ? `Tap one of your territories to place an army. ${mySetupArmies} remaining.`
                : "All your armies placed — waiting for others…"}
            </p>
          </div>
          <div className="grid gap-1.5">
            {players.map((p, i) => (
              <div key={p.userId} className="flex items-center gap-2 text-xs">
                <span
                  className="h-2.5 w-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: playerColors[i] ?? "#94a3b8" }}
                />
                <span className="font-medium" style={{ color: playerColors[i] ?? "#94a3b8" }}>
                  {p.userName}
                </span>
                <span className="ml-auto text-slate-500">{p.setupArmies} left</span>
              </div>
            ))}
          </div>
        </div>
      );
    }

    if (phaseMode === "reinforce") {
      return (
        <div className="grid gap-4 p-4">
          <div className="text-center">
            <p className="text-xs uppercase tracking-widest text-slate-500">Armies to Place</p>
            <p className="text-5xl font-black text-emerald-400 tabular-nums">{pendingReinforcements}</p>
          </div>

          {selectedTerritory ? (
            <div className="rounded-xl bg-slate-700 p-3 text-center">
              <p className="text-[10px] uppercase tracking-widest text-slate-400">Placing on</p>
              <p className="mt-0.5 text-sm font-bold text-white">{selectedTerritory}</p>
              {selInfo && (
                <p className="mt-0.5 text-xs" style={{ color: selInfo.ownerColor }}>
                  currently {selInfo.armies} {selInfo.armies === 1 ? "army" : "armies"}
                </p>
              )}
            </div>
          ) : (
            <p className="text-center text-xs text-slate-500">Tap one of your territories on the map</p>
          )}

          {pendingReinforcements > 0 && (
            <>
              {renderArmyStepper()}
              <button
                type="button"
                onClick={commitReinforcement}
                disabled={!isMyTurn || !selectedTerritory}
                className="w-full rounded-xl bg-emerald-600 py-3.5 text-sm font-bold text-white disabled:opacity-40 active:bg-emerald-500"
              >
                Place {clampedArmiesInput} {clampedArmiesInput === 1 ? "Army" : "Armies"}
              </button>
            </>
          )}
          {pendingReinforcements === 0 && (
            <p className="text-center text-xs text-slate-500">
              All reinforcements placed — transitioning to attack…
            </p>
          )}
        </div>
      );
    }

    if (phaseMode === "attack") {
      return (
        <div className="grid gap-3 p-4">
          {renderTerritoryPair("Attack From", "Target", "⚔")}

          <div className="flex items-center gap-2 rounded-xl bg-slate-700 px-3 py-2.5">
            <span className="text-xs text-slate-400">Dice</span>
            <div className="flex gap-1.5">
              {[1, 2, 3].map((n) => (
                <button
                  key={n}
                  type="button"
                  disabled={n > maxAttackDiceAllowed}
                  onClick={() => setAttackerDice(n)}
                  className={`flex h-9 w-9 items-center justify-center rounded-lg text-sm font-bold transition-colors ${
                    clampedAttackerDice === n
                      ? "bg-rose-500 text-white"
                      : "bg-slate-600 text-slate-300 disabled:opacity-30"
                  }`}
                >
                  {n}
                </button>
              ))}
            </div>
            <span className="ml-auto text-xs text-slate-400">Def: {maxDefendDiceAllowed}</span>
          </div>

          {diceResult && (
            <div className="rounded-xl bg-slate-700 p-3">
              <div className="flex justify-around text-center">
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-slate-400">Attacker</p>
                  <p className="text-lg font-black text-white">{diceResult.attacker.join(" · ")}</p>
                  <p className="text-xs text-rose-400">−{diceResult.attackerLoss}</p>
                </div>
                <div className="text-slate-600 self-center text-xl">vs</div>
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-slate-400">Defender</p>
                  <p className="text-lg font-black text-white">{diceResult.defender.join(" · ")}</p>
                  <p className="text-xs text-emerald-400">−{diceResult.defenderLoss}</p>
                </div>
              </div>
            </div>
          )}

          <button
            type="button"
            onClick={onRollDice}
            disabled={!isMyTurn || !canAttackSelection}
            className="w-full rounded-xl bg-rose-600 py-3.5 text-sm font-bold text-white disabled:opacity-40 active:bg-rose-500"
          >
            🎲 Roll Dice
          </button>
          {!canAttackSelection && (
            <p className="text-center text-xs text-slate-500">
              Select an adjacent attacker + enemy territory
            </p>
          )}
          <button
            type="button"
            onClick={() => {
              sendAction({ action: "end_attack" });
              setSelectedFrom("");
              setSelectedTo("");
              setSelectedTerritory("");
            }}
            disabled={!isMyTurn}
            className="w-full rounded-xl bg-slate-700 py-3 text-sm font-medium text-slate-300 disabled:opacity-40 active:bg-slate-600"
          >
            End Attack Phase →
          </button>
        </div>
      );
    }

    if (phaseMode === "occupy") {
      return (
        <div className="grid gap-4 p-4">
          <div className="rounded-xl border border-amber-700 bg-amber-900/40 p-3 text-center">
            <p className="text-sm font-bold text-amber-300">Territory Conquered! 🏆</p>
            <p className="mt-1 text-xs text-amber-400">
              Move troops: <strong>{occupyRequirement?.from}</strong> → <strong>{occupyRequirement?.to}</strong>
            </p>
            <p className="mt-0.5 text-xs text-amber-600">
              ({occupyRequirement?.minMove}–{occupyRequirement?.maxMove} armies required)
            </p>
          </div>
          {renderArmyStepper()}
          <button
            type="button"
            onClick={commitOccupy}
            disabled={!isMyTurn}
            className="w-full rounded-xl bg-amber-500 py-3.5 text-sm font-bold text-white disabled:opacity-40 active:bg-amber-400"
          >
            Move {clampedArmiesInput} {clampedArmiesInput === 1 ? "Troop" : "Troops"}
          </button>
        </div>
      );
    }

    if (phaseMode === "fortify") {
      return (
        <div className="grid gap-3 p-4">
          {renderTerritoryPair("From", "To", "→")}
          {!activeFrom || !activeTo ? (
            <p className="text-center text-xs text-slate-500">Tap two of your territories on the map</p>
          ) : (
            <>
              {renderArmyStepper()}
              <button
                type="button"
                onClick={commitFortify}
                disabled={!isMyTurn}
                className="w-full rounded-xl bg-violet-600 py-3.5 text-sm font-bold text-white disabled:opacity-40 active:bg-violet-500"
              >
                Fortify
              </button>
            </>
          )}
          <button
            type="button"
            onClick={() => {
              sendAction({ action: "end_turn" });
              setSelectedFrom("");
              setSelectedTo("");
              setSelectedTerritory("");
            }}
            disabled={!isMyTurn}
            className="w-full rounded-xl bg-slate-700 py-3 text-sm font-medium text-slate-300 disabled:opacity-40 active:bg-slate-600"
          >
            End Turn →
          </button>
        </div>
      );
    }

    return <p className="p-4 text-center text-xs text-slate-500">Waiting…</p>;
  };

  // ── Cards tab content ───────────────────────────────────────────────────────

  const renderCardsTab = () => {
    if (meIndex < 0 || game?.status !== "in_progress") {
      return <p className="p-4 text-center text-xs text-slate-500">Not an active player.</p>;
    }
    return (
      <div className="grid gap-3 p-4">
        <div className="flex items-center justify-between text-xs text-slate-400">
          <span>{myCards.length} {myCards.length === 1 ? "card" : "cards"}</span>
          <span>Next trade: <span className="font-bold text-indigo-400">+{nextTradeBonus} armies</span></span>
        </div>

        {myCards.length === 0 ? (
          <p className="text-center text-xs text-slate-500 py-3">
            No cards yet — conquer a territory to earn one.
          </p>
        ) : (
          <div className="grid gap-2">
            {myCards.map((card, idx) => {
              const isSelected = selectedCardIndices.includes(idx);
              return (
                <button
                  key={idx}
                  type="button"
                  onClick={() => toggleCardSelection(idx)}
                  className={`flex items-center gap-3 rounded-xl px-4 py-3 text-left transition-colors ${
                    isSelected
                      ? "bg-indigo-600 text-white"
                      : "bg-slate-700 text-slate-200 active:bg-slate-600"
                  }`}
                >
                  <span className="text-2xl">{symbolIcon(card.symbol)}</span>
                  <div className="min-w-0">
                    <p className="font-bold capitalize">{card.symbol}</p>
                    {card.territory && (
                      <p className="truncate text-xs opacity-60">{card.territory}</p>
                    )}
                  </div>
                  {isSelected && <span className="ml-auto text-indigo-200 text-lg">✓</span>}
                </button>
              );
            })}
          </div>
        )}

        {myCards.length >= 5 && isMyTurn && phaseMode === "reinforce" && (
          <div className="rounded-xl border border-amber-700 bg-amber-900/40 p-3 text-xs text-amber-300">
            {(game?.pendingReinforcements ?? 0) === 0
              ? "You have 5+ cards — trade a set before attacking."
              : "Trade cards before placing reinforcements (5+ held)."}
          </div>
        )}

        {isMyTurn && phaseMode === "reinforce" && myCards.length >= 3 && (
          <div className="grid gap-2">
            <p className="text-center text-xs text-slate-500">{selectedCardIndices.length}/3 selected</p>
            <button
              type="button"
              onClick={commitTradeCards}
              disabled={selectedCardIndices.length !== 3}
              className="w-full rounded-xl bg-indigo-600 py-3.5 text-sm font-bold text-white disabled:opacity-40 active:bg-indigo-500"
            >
              Trade Selected Cards
            </button>
          </div>
        )}
      </div>
    );
  };

  // ── Events tab content ──────────────────────────────────────────────────────

  const renderEventsTab = () => (
    <div ref={eventScrollRef} className="grid gap-2 p-3">
      {eventMessages.length === 0 && (
        <p className="py-4 text-center text-xs text-slate-500">No events yet.</p>
      )}
      {eventMessages.map((ev, idx) => (
        <div key={`${ev.id}-${idx}`} className="rounded-xl bg-slate-700 px-3 py-2.5">
          <div className="mb-1 flex items-center justify-between text-[10px] text-slate-400">
            <span className="font-semibold uppercase tracking-wide">
              {ev.eventType.replaceAll("_", " ")}
            </span>
            <span>
              {new Date(ev.createdAt).toLocaleString(undefined, {
                hour: "numeric",
                minute: "2-digit",
              })}
            </span>
          </div>
          <p className="text-xs text-slate-200">{renderEventBody(ev.body)}</p>
        </div>
      ))}
    </div>
  );

  // ── Chat tab content ────────────────────────────────────────────────────────

  const renderChatTab = () => (
    <div className="flex flex-col">
      <div ref={chatScrollRef} className="grid max-h-52 gap-2 overflow-y-auto p-3">
        {chatMessages.length === 0 && (
          <p className="py-4 text-center text-xs text-slate-500">No messages yet.</p>
        )}
        {chatMessages.map((m, idx) => (
          <div key={`${m.userName}-${m.createdAt}-${idx}`} className="rounded-xl bg-slate-700 px-3 py-2">
            <div className="mb-0.5 flex items-center justify-between text-[10px]">
              <span className="font-semibold text-slate-300">{m.userName}</span>
              <span className="text-slate-500">
                {new Date(m.createdAt).toLocaleString(undefined, {
                  hour: "numeric",
                  minute: "2-digit",
                })}
              </span>
            </div>
            <p className="whitespace-pre-wrap text-xs text-slate-200">{m.body}</p>
          </div>
        ))}
      </div>
      <form className="flex gap-2 border-t border-slate-700 p-3" onSubmit={onSendChat}>
        <input
          className="min-w-0 flex-1 rounded-xl bg-slate-700 px-3 py-2 text-sm text-white placeholder-slate-500 outline-none focus:ring-1 focus:ring-indigo-500"
          value={chatDraft}
          onChange={(e) => setChatDraft(e.target.value)}
          placeholder="Message…"
        />
        <button
          type="submit"
          disabled={chatDraft.trim() === "" || wsStatus !== "connected"}
          className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-40 active:bg-indigo-500"
        >
          Send
        </button>
      </form>
      {chatError && <p className="px-3 pb-2 text-xs text-rose-400">{chatError}</p>}
    </div>
  );

  return (
    <div className="flex flex-col overflow-hidden bg-slate-900 text-white" style={{ height: "100dvh" }}>
      {/* ── Top bar ── */}
      <header className="flex shrink-0 items-center gap-2 border-b border-slate-700 bg-slate-800 px-3 py-2">
        <Link
          to="/app/lobby"
          className="shrink-0 rounded-lg bg-slate-700 px-2.5 py-1.5 text-xs font-medium text-slate-300 active:bg-slate-600"
        >
          ← Lobby
        </Link>

        <div className="flex min-w-0 flex-1 flex-col items-center">
          <div className="flex items-center gap-2">
            <span className={`rounded-full px-2.5 py-0.5 text-xs font-bold text-white ${phaseBadgeClass}`}>
              {phaseLabel}
            </span>
            <span
              className="truncate text-xs font-semibold"
              style={{ color: currentPlayerColor }}
            >
              {isMyTurn ? "Your Turn" : `${currentPlayer?.userName ?? "?"}'s Turn`}
            </span>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <span
            className={`h-2 w-2 rounded-full ${wsStatus === "connected" ? "bg-emerald-400" : "bg-rose-400"}`}
            title={wsStatus}
          />
          <button
            type="button"
            onClick={onToggleDesktop}
            className="rounded-lg bg-slate-700 px-2.5 py-1.5 text-xs font-medium text-slate-300 active:bg-slate-600"
          >
            Desktop
          </button>
        </div>
      </header>

      {/* ── Action error banner ── */}
      {actionError && (
        <div className="shrink-0 border-b border-rose-800 bg-rose-900/80 px-4 py-2 text-xs text-rose-200">
          {actionError}
        </div>
      )}

      {/* ── Map ── */}
      <div className="relative min-h-0 flex-1 bg-slate-950">
        <GameMap
          ref={mapRef}
          game={game}
          selectedTerritory={selectedTerritory}
          activeFrom={activeFrom}
          activeTo={activeTo}
          playerColors={playerColors}
          onTerritoryClick={onMapTerritoryClick}
        />
        {/* Zoom controls */}
        <div className="absolute bottom-2 right-2 flex flex-col gap-1">
          <button
            type="button"
            onClick={() => mapRef.current?.zoomIn()}
            className="flex h-9 w-9 items-center justify-center rounded-xl bg-slate-800/80 text-lg font-bold text-white backdrop-blur-sm active:bg-slate-700"
            aria-label="Zoom in"
          >
            +
          </button>
          <button
            type="button"
            onClick={() => mapRef.current?.resetZoom()}
            className="flex h-9 w-9 items-center justify-center rounded-xl bg-slate-800/80 text-sm text-slate-300 backdrop-blur-sm active:bg-slate-700"
            aria-label="Reset zoom"
          >
            ⌖
          </button>
          <button
            type="button"
            onClick={() => mapRef.current?.zoomOut()}
            className="flex h-9 w-9 items-center justify-center rounded-xl bg-slate-800/80 text-lg font-bold text-white backdrop-blur-sm active:bg-slate-700"
            aria-label="Zoom out"
          >
            −
          </button>
        </div>
      </div>

      {/* ── Players strip ── */}
      <div className="flex shrink-0 gap-2 overflow-x-auto border-t border-slate-700 bg-slate-800/80 px-3 py-1.5">
        {players.map((p, i) => {
          const isCurrent = i === currentPlayerIndex;
          return (
            <div
              key={p.userId}
              className={`flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs ${
                isCurrent ? "bg-slate-600" : ""
              }`}
            >
              <span
                className="h-2 w-2 shrink-0 rounded-full"
                style={{ backgroundColor: playerColors[i] ?? "#94a3b8" }}
              />
              <span
                className={`font-medium ${p.eliminated ? "line-through opacity-40" : ""}`}
                style={{ color: playerColors[i] ?? "#94a3b8" }}
              >
                {p.userName}
              </span>
              {isCurrent && <span className="text-[9px] text-slate-400">▶</span>}
            </div>
          );
        })}
      </div>

      {/* ── Bottom sheet ── */}
      <div
        className="flex shrink-0 flex-col border-t border-slate-700 bg-slate-800"
        style={{ maxHeight: "46vh" }}
      >
        {/* Tab bar */}
        <div className="flex shrink-0 border-b border-slate-700">
          {(["actions", "cards", "events", "chat"] as Tab[]).map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setActiveTab(tab)}
              className={`relative flex-1 py-2.5 text-xs font-semibold uppercase tracking-wide transition-colors ${
                activeTab === tab
                  ? "border-b-2 border-indigo-400 text-indigo-300"
                  : "text-slate-500 active:text-slate-300"
              }`}
            >
              {tab}
              {tab === "cards" && myCards.length > 0 && (
                <span className="absolute right-1 top-1.5 rounded-full bg-indigo-500 px-1 text-[9px] text-white">
                  {myCards.length}
                </span>
              )}
            </button>
          ))}
        </div>

        {/* Tab content */}
        <div className="min-h-0 flex-1 overflow-y-auto">
          {activeTab === "actions" && renderActionsTab()}
          {activeTab === "cards" && renderCardsTab()}
          {activeTab === "events" && renderEventsTab()}
          {activeTab === "chat" && renderChatTab()}
        </div>
      </div>
    </div>
  );
}
