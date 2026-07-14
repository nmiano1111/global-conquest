import { useEffect, useRef, useState, type RefObject } from "react";
import { createPortal } from "react-dom";
import type { GameBootstrap } from "../../api/games";
import type { GameMapHandle } from "../../map/GameMap";
import { deriveInteractionMode } from "./interactionMode";
import { PHASE_BADGE_CLASS, PHASE_LABELS, type DiceRollResult } from "./gameShared";

type OccupyRequirement = NonNullable<GameBootstrap["occupy"]>;
type Player = GameBootstrap["players"][number];

export interface FullscreenGameMapProps {
  game: GameBootstrap | null;
  onClose: () => void;
  /** Shared GameMap instance's imperative handle, owned by GamePage — see GamePage for why it's shared rather than one-per-view. */
  mapRef: RefObject<GameMapHandle | null>;
  /** Registers this shell's map container as the active portal target for the shared GameMap. */
  mapSlotRef: (node: HTMLDivElement | null) => void;

  phase: string;
  phaseMode: string;
  isMyTurn: boolean;
  isGameOver: boolean;
  wsStatus: string;

  players: Player[];
  playerColors: string[];
  territoryState: Record<string, unknown> | null;

  pendingReinforcements: number;
  mySetupArmies: number;
  occupyRequirement: OccupyRequirement | null;
  diceResult: DiceRollResult | null;

  selectedTerritory: string;
  selectedFrom: string;
  selectedTo: string;
  activeFrom: string;
  activeTo: string;

  clampedArmiesInput: number;
  minArmiesInput: number;
  maxArmiesInput: number;
  clampedAttackerDice: number;
  maxAttackDiceAllowed: number;
  maxDefendDiceAllowed: number;
  canAttackSelection: boolean;

  commitReinforcement: () => void;
  commitFortify: () => void;
  commitOccupy: () => void;
  onRollDice: () => void;
  setArmiesInput: (n: number) => void;
  setAttackerDice: (n: number) => void;
  sendAction: (payload: Record<string, unknown>) => void;
  setSelectedFrom: (s: string) => void;
  setSelectedTo: (s: string) => void;
  setSelectedTerritory: (s: string) => void;
}

function getTerritoryInfo(
  name: string,
  territoryState: Record<string, unknown> | null,
  players: Player[],
  playerColors: string[],
) {
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
}

/** Floating circular icon button used for exit/camera controls over the map. */
function MapIconButton(props: {
  onClick: () => void;
  label: string;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={props.onClick}
      disabled={props.disabled}
      aria-label={props.label}
      title={props.label}
      className="flex h-11 w-11 items-center justify-center rounded-full bg-slate-800/85 text-lg font-semibold text-slate-100 shadow-lg backdrop-blur transition-colors active:bg-slate-700 disabled:opacity-40"
    >
      {props.children}
    </button>
  );
}

/**
 * Drag handle spanning the top of the bottom sheet. Tapping it toggles
 * open/collapsed; dragging it vertically does too, snapping to whichever
 * state the drag direction implies once past a small threshold — dragging
 * down closes, dragging up opens. A tiny movement (a tap, not a drag) just
 * toggles the current state, matching a native bottom-sheet handle.
 */
function SheetHandle(props: { collapsed: boolean; onToggle: (collapsed: boolean) => void }) {
  const dragStartYRef = useRef<number | null>(null);

  return (
    <button
      type="button"
      aria-expanded={!props.collapsed}
      aria-label={props.collapsed ? "Expand action panel" : "Collapse action panel"}
      className="flex w-full touch-none items-center justify-center py-2.5"
      onPointerDown={(e) => {
        dragStartYRef.current = e.clientY;
        e.currentTarget.setPointerCapture?.(e.pointerId);
      }}
      onPointerUp={(e) => {
        const startY = dragStartYRef.current;
        dragStartYRef.current = null;
        if (startY === null) return;
        const deltaY = e.clientY - startY;
        if (Math.abs(deltaY) < 8) {
          props.onToggle(!props.collapsed);
        } else if (deltaY > 24) {
          props.onToggle(true);
        } else if (deltaY < -24) {
          props.onToggle(false);
        }
      }}
      onClick={(e) => {
        // Keyboard activation (Enter/Space) dispatches a click with no
        // preceding pointer events, so the pointerup handling above never
        // runs for it — this is the only path that toggles for keyboard
        // users. Real mouse/touch clicks are already handled by pointerup
        // and browsers synthesize a click after them too (with detail >=
        // 1); ignoring those here avoids toggling twice and canceling out.
        if (e.detail === 0) props.onToggle(!props.collapsed);
      }}
    >
      <span className="h-1.5 w-10 rounded-full bg-slate-500/70" />
    </button>
  );
}

/**
 * Shared translucent shell for the bottom sheet, with a drag/tap handle
 * and a smooth slide-open/slide-shut transition. Uses the CSS grid
 * "animate to auto height" technique (grid-template-rows: 1fr ↔ 0fr) so it
 * doesn't need to measure the (per-phase, variable) content height in JS.
 */
function CollapsibleSheet(props: {
  collapsed: boolean;
  onToggleCollapsed: (collapsed: boolean) => void;
  variant: "action" | "status";
  testId: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={`relative z-10 shrink-0 rounded-t-2xl border-t border-slate-700 shadow-[0_-8px_24px_rgba(0,0,0,0.4)] backdrop-blur-md ${
        props.variant === "action" ? "bg-slate-900/70" : "bg-slate-900/60"
      }`}
      style={{ paddingBottom: "env(safe-area-inset-bottom)" }}
      data-testid={props.testId}
    >
      <SheetHandle collapsed={props.collapsed} onToggle={props.onToggleCollapsed} />
      <div
        className="grid transition-[grid-template-rows] duration-300 ease-out motion-reduce:transition-none"
        style={{ gridTemplateRows: props.collapsed ? "0fr" : "1fr" }}
      >
        <div className="overflow-hidden">{props.children}</div>
      </div>
    </div>
  );
}

export function FullscreenGameMap(props: FullscreenGameMapProps) {
  const {
    game, onClose, mapRef, mapSlotRef, phase, phaseMode, isMyTurn, isGameOver, wsStatus,
    players, playerColors, territoryState,
    pendingReinforcements, mySetupArmies, occupyRequirement, diceResult,
    selectedTerritory, selectedFrom, selectedTo, activeFrom, activeTo,
    clampedArmiesInput, minArmiesInput, maxArmiesInput,
    clampedAttackerDice, maxAttackDiceAllowed, maxDefendDiceAllowed, canAttackSelection,
    commitReinforcement, commitFortify, commitOccupy, onRollDice,
    setArmiesInput, setAttackerDice, sendAction, setSelectedFrom, setSelectedTo, setSelectedTerritory,
  } = props;

  const [portalEl] = useState(() => document.createElement("div"));
  const [sheetCollapsed, setSheetCollapsed] = useState(false);

  // Root-level fixed layer: mount a dedicated element on document.body so
  // this renders above the entire app shell (including its sticky, z-30
  // header) regardless of where in the component tree it's used from.
  useEffect(() => {
    document.body.appendChild(portalEl);
    return () => {
      document.body.removeChild(portalEl);
    };
  }, [portalEl]);

  // Lock page scroll / touch gestures while fullscreen is open, restoring
  // whatever was there before on close. Also mark the rest of the app
  // `inert` so it's unreachable by keyboard/assistive-tech navigation
  // while this dialog-like layer is open — the underlying page (e.g. the
  // embedded map's own zoom buttons) stays mounted behind this portal,
  // and without this it would remain focusable/announceable despite being
  // fully covered on screen.
  useEffect(() => {
    const prevOverflow = document.body.style.overflow;
    const prevTouchAction = document.body.style.touchAction;
    document.body.style.overflow = "hidden";
    document.body.style.touchAction = "none";
    const appRoot = document.getElementById("root");
    appRoot?.setAttribute("inert", "");
    return () => {
      document.body.style.overflow = prevOverflow;
      document.body.style.touchAction = prevTouchAction;
      appRoot?.removeAttribute("inert");
    };
  }, []);

  // Desktop: Escape closes fullscreen mode.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  const currentPlayerIndex = game?.currentPlayer ?? -1;
  const currentPlayer = players[currentPlayerIndex];
  const currentPlayerColor = playerColors[currentPlayerIndex] ?? "#94a3b8";
  const phaseLabel = PHASE_LABELS[phase] ?? phase;
  const phaseBadgeClass = PHASE_BADGE_CLASS[phase] ?? "bg-slate-600";

  const mode = deriveInteractionMode({
    phaseMode,
    isGameOver,
    isMyTurn,
    pendingReinforcements,
    mySetupArmies,
    selectedTerritory,
    selectedFrom,
    selectedTo,
    occupyRequirement,
  });

  const fromInfo = getTerritoryInfo(activeFrom, territoryState, players, playerColors);
  const toInfo = getTerritoryInfo(activeTo, territoryState, players, playerColors);
  const selInfo = getTerritoryInfo(selectedTerritory, territoryState, players, playerColors);

  const armyStepper = (
    <div className="flex items-center justify-center gap-4">
      <button
        type="button"
        onClick={() => setArmiesInput(Math.max(minArmiesInput, clampedArmiesInput - 1))}
        aria-label="Decrease army count"
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
        aria-label="Increase army count"
        className="flex h-11 w-11 items-center justify-center rounded-full bg-slate-700 text-2xl font-bold text-white active:bg-slate-600"
      >
        +
      </button>
      <span className="text-xs text-slate-500">max {maxArmiesInput}</span>
    </div>
  );

  const territoryPair = (fromLabel: string, toLabel: string, icon: string) => (
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

  const clearSelection = () => {
    setSelectedFrom("");
    setSelectedTo("");
    setSelectedTerritory("");
  };

  // ── Bottom sheet content, one branch per interaction mode ──────────────
  let sheet: React.ReactNode;
  switch (mode.kind) {
    case "game-over":
      sheet = (
        <div className="grid gap-1 p-4 text-center">
          <p className="text-lg font-black text-white">Game Over</p>
          <p className="text-xs text-slate-400">Exit fullscreen to see the final standings.</p>
        </div>
      );
      break;

    case "waiting":
      sheet = (
        <div className="flex items-center gap-3 p-4" data-testid="waiting-sheet">
          <span className="text-2xl">⏳</span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-slate-200">
              {currentPlayer?.userName ?? "Opponent"}'s turn
            </p>
            <p className="text-xs text-slate-500 capitalize">{phaseLabel} phase — pan and zoom freely</p>
          </div>
        </div>
      );
      break;

    case "setup-reinforce":
      sheet = (
        <div className="grid gap-2 p-4 text-center" data-testid="setup-reinforce-sheet">
          <p className="text-sm font-semibold text-sky-300">Initial Placement</p>
          <p className="text-xs text-sky-400">
            {mode.armiesRemaining > 0
              ? `Tap one of your territories to place an army. ${mode.armiesRemaining} remaining.`
              : "All your armies placed — waiting for others…"}
          </p>
        </div>
      );
      break;

    case "reinforce":
      sheet = (
        <div className="grid gap-3 p-4" data-testid="reinforce-sheet">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-[10px] uppercase tracking-widest text-slate-500">Armies to Place</p>
              <p className="text-3xl font-black text-emerald-400 tabular-nums">{mode.remaining}</p>
            </div>
            {mode.territory ? (
              <div className="text-right">
                <p className="text-[10px] uppercase tracking-widest text-slate-500">Placing on</p>
                <p className="text-sm font-bold text-white">{mode.territory}</p>
                {selInfo && (
                  <p className="text-xs" style={{ color: selInfo.ownerColor }}>
                    currently {selInfo.armies}
                  </p>
                )}
              </div>
            ) : (
              <p className="text-xs text-slate-500">Tap one of your territories</p>
            )}
          </div>
          {mode.remaining > 0 && (
            <>
              {armyStepper}
              <button
                type="button"
                onClick={commitReinforcement}
                disabled={!mode.territory}
                className="w-full rounded-xl bg-emerald-600 py-3 text-sm font-bold text-white disabled:opacity-40 active:bg-emerald-500"
              >
                Place {clampedArmiesInput} {clampedArmiesInput === 1 ? "Army" : "Armies"}
              </button>
            </>
          )}
        </div>
      );
      break;

    case "attack-select-source":
      sheet = (
        <div className="grid gap-2 p-4 text-center" data-testid="attack-select-source-sheet">
          <p className="text-sm font-semibold text-rose-300">Choose an attacking territory</p>
          <p className="text-xs text-slate-500">Tap one of your territories bordering an enemy.</p>
          <button
            type="button"
            onClick={() => sendAction({ action: "end_attack" })}
            className="mt-1 w-full rounded-xl bg-slate-700 py-2.5 text-xs font-medium text-slate-300 active:bg-slate-600"
          >
            End Attack Phase →
          </button>
        </div>
      );
      break;

    case "attack-select-target":
      sheet = (
        <div className="grid gap-2 p-4" data-testid="attack-select-target-sheet">
          <p className="text-center text-sm font-semibold text-rose-300">
            Attacking from <span className="text-white">{mode.from}</span>
          </p>
          <p className="text-center text-xs text-slate-500">Tap a highlighted enemy territory to target it.</p>
          <button
            type="button"
            onClick={() => setSelectedFrom("")}
            className="w-full rounded-xl bg-slate-700 py-2.5 text-xs font-medium text-slate-300 active:bg-slate-600"
          >
            Change Attacker
          </button>
        </div>
      );
      break;

    case "attack-ready":
      sheet = (
        <div className="grid gap-3 p-4" data-testid="attack-ready-sheet">
          {territoryPair("Attack From", "Target", "⚔")}
          <div className="flex items-center gap-2 rounded-xl bg-slate-700 px-3 py-2.5">
            <span className="text-xs text-slate-400">Dice</span>
            <div className="flex gap-1.5">
              {[1, 2, 3].map((n) => (
                <button
                  key={n}
                  type="button"
                  disabled={n > maxAttackDiceAllowed}
                  onClick={() => setAttackerDice(n)}
                  aria-pressed={clampedAttackerDice === n}
                  className={`flex h-9 w-9 items-center justify-center rounded-lg text-sm font-bold transition-colors ${
                    clampedAttackerDice === n ? "bg-rose-500 text-white" : "bg-slate-600 text-slate-300 disabled:opacity-30"
                  }`}
                >
                  {n}
                </button>
              ))}
            </div>
            <span className="ml-auto text-xs text-slate-400">Def: {maxDefendDiceAllowed}</span>
          </div>
          {diceResult && (
            <div className="rounded-xl bg-slate-700 p-2.5">
              <div className="flex justify-around text-center">
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-slate-400">Attacker</p>
                  <p className="text-base font-black text-white">{diceResult.attacker.join(" · ")}</p>
                  <p className="text-xs text-rose-400">−{diceResult.attackerLoss}</p>
                </div>
                <div className="self-center text-lg text-slate-600">vs</div>
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-slate-400">Defender</p>
                  <p className="text-base font-black text-white">{diceResult.defender.join(" · ")}</p>
                  <p className="text-xs text-emerald-400">−{diceResult.defenderLoss}</p>
                </div>
              </div>
            </div>
          )}
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              onClick={() => setSelectedTo("")}
              className="w-full rounded-xl bg-slate-700 py-3 text-sm font-medium text-slate-300 active:bg-slate-600"
            >
              Change Target
            </button>
            <button
              type="button"
              onClick={onRollDice}
              disabled={!canAttackSelection}
              className="w-full rounded-xl bg-rose-600 py-3 text-sm font-bold text-white disabled:opacity-40 active:bg-rose-500"
            >
              🎲 Roll Dice
            </button>
          </div>
          <button
            type="button"
            onClick={() => {
              sendAction({ action: "end_attack" });
              clearSelection();
            }}
            className="w-full rounded-xl bg-slate-800 py-2.5 text-xs font-medium text-slate-400 active:bg-slate-700"
          >
            End Attack Phase →
          </button>
        </div>
      );
      break;

    case "occupy":
      sheet = (
        <div className="grid gap-3 p-4" data-testid="occupy-sheet">
          <div className="rounded-xl border border-amber-700 bg-amber-900/40 p-3 text-center">
            <p className="text-sm font-bold text-amber-300">Territory Conquered! 🏆</p>
            <p className="mt-1 text-xs text-amber-400">
              Move troops: <strong>{mode.from}</strong> → <strong>{mode.to}</strong>
            </p>
            <p className="mt-0.5 text-xs text-amber-600">
              ({mode.minMove}–{mode.maxMove} armies required)
            </p>
          </div>
          {armyStepper}
          <button
            type="button"
            onClick={commitOccupy}
            className="w-full rounded-xl bg-amber-500 py-3 text-sm font-bold text-white active:bg-amber-400"
          >
            Move {clampedArmiesInput} {clampedArmiesInput === 1 ? "Troop" : "Troops"}
          </button>
        </div>
      );
      break;

    case "fortify-select-source":
      sheet = (
        <div className="grid gap-2 p-4 text-center" data-testid="fortify-select-source-sheet">
          <p className="text-sm font-semibold text-violet-300">Choose a source territory</p>
          <p className="text-xs text-slate-500">Tap one of your territories to move armies from.</p>
          <button
            type="button"
            onClick={() => {
              sendAction({ action: "end_turn" });
              clearSelection();
            }}
            className="mt-1 w-full rounded-xl bg-slate-700 py-2.5 text-xs font-medium text-slate-300 active:bg-slate-600"
          >
            End Turn →
          </button>
        </div>
      );
      break;

    case "fortify-select-destination":
      sheet = (
        <div className="grid gap-2 p-4" data-testid="fortify-select-destination-sheet">
          <p className="text-center text-sm font-semibold text-violet-300">
            Moving from <span className="text-white">{mode.from}</span>
          </p>
          <p className="text-center text-xs text-slate-500">Tap another of your territories as the destination.</p>
          <button
            type="button"
            onClick={() => setSelectedFrom("")}
            className="w-full rounded-xl bg-slate-700 py-2.5 text-xs font-medium text-slate-300 active:bg-slate-600"
          >
            Change Source
          </button>
        </div>
      );
      break;

    case "fortify-ready":
      sheet = (
        <div className="grid gap-3 p-4" data-testid="fortify-ready-sheet">
          {territoryPair("From", "To", "→")}
          {armyStepper}
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              onClick={clearSelection}
              className="w-full rounded-xl bg-slate-700 py-3 text-sm font-medium text-slate-300 active:bg-slate-600"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={commitFortify}
              className="w-full rounded-xl bg-violet-600 py-3 text-sm font-bold text-white active:bg-violet-500"
            >
              Fortify
            </button>
          </div>
          <button
            type="button"
            onClick={() => {
              sendAction({ action: "end_turn" });
              clearSelection();
            }}
            className="w-full rounded-xl bg-slate-800 py-2.5 text-xs font-medium text-slate-400 active:bg-slate-700"
          >
            End Turn →
          </button>
        </div>
      );
      break;
  }

  const showMutationControls = mode.kind !== "waiting" && mode.kind !== "game-over";

  return createPortal(
    <div
      className="fixed inset-0 flex flex-col bg-slate-950"
      style={{ height: "100dvh", width: "100vw", zIndex: 999, touchAction: "none", overscrollBehavior: "none" }}
      role="dialog"
      aria-modal="true"
      aria-label="Fullscreen map"
      data-testid="fullscreen-map-root"
    >
      {/* ── Map layer (fills background) ──
          The actual <GameMap> is owned and rendered by GamePage, portaled
          in here — see the comment by GamePage's embeddedMapSlotEl for why
          embedded and fullscreen share one instance rather than each
          mounting/destroying their own. */}
      <div ref={mapSlotRef} className="absolute inset-0" />

      {/* ── Top bar ── */}
      <header
        className="relative z-10 flex shrink-0 items-center gap-2 bg-slate-900/85 px-3 py-2 shadow-lg backdrop-blur-sm"
        style={{ paddingTop: "calc(env(safe-area-inset-top) + 0.5rem)" }}
      >
        <MapIconButton onClick={onClose} label="Exit fullscreen map">
          ✕
        </MapIconButton>
        <div className="flex min-w-0 flex-1 flex-col items-center">
          <div className="flex items-center gap-2">
            <span className={`rounded-full px-2.5 py-0.5 text-xs font-bold text-white ${phaseBadgeClass}`}>
              {phaseLabel}
            </span>
            <span className="truncate text-xs font-semibold" style={{ color: currentPlayerColor }}>
              {isMyTurn ? "Your Turn" : `${currentPlayer?.userName ?? "?"}'s Turn`}
            </span>
          </div>
        </div>
        <span
          className={`h-2.5 w-2.5 shrink-0 rounded-full ${wsStatus === "connected" ? "bg-emerald-400" : "bg-rose-400"}`}
          title={`Connection: ${wsStatus}`}
          aria-label={`Connection status: ${wsStatus}`}
        />
      </header>

      {/*
        ── Middle spacer: everything between the header and the bottom sheet.
        A real flex child (not absolutely positioned with a guessed offset)
        so the floating camera controls always land in the space actually
        left over above the sheet, however tall the sheet's phase-specific
        content is — never overlapping or intercepting taps meant for it.
        pointer-events-none on the spacer itself lets map gestures (pan/
        pinch/tap) pass straight through everywhere except the button
        cluster, which re-enables pointer events for itself.
      */}
      <div className="relative z-10 flex flex-1 flex-col items-end justify-end gap-2 p-3 pointer-events-none">
        <div className="pointer-events-auto flex flex-col gap-2">
          <MapIconButton onClick={() => mapRef.current?.zoomIn()} label="Zoom in">
            +
          </MapIconButton>
          <MapIconButton onClick={() => mapRef.current?.resetZoom()} label="Recenter map">
            ⌖
          </MapIconButton>
          <MapIconButton onClick={() => mapRef.current?.zoomOut()} label="Zoom out">
            −
          </MapIconButton>
        </div>
      </div>

      {/* ── Bottom sheet: translucent, slide open/shut via the handle ── */}
      {showMutationControls ? (
        <CollapsibleSheet
          collapsed={sheetCollapsed}
          onToggleCollapsed={setSheetCollapsed}
          variant="action"
          testId="action-sheet"
        >
          {sheet}
        </CollapsibleSheet>
      ) : (
        <CollapsibleSheet
          collapsed={sheetCollapsed}
          onToggleCollapsed={setSheetCollapsed}
          variant="status"
          testId="status-overlay"
        >
          {sheet}
        </CollapsibleSheet>
      )}
    </div>,
    portalEl,
  );
}
