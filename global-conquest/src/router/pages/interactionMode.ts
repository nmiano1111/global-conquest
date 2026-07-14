// src/router/pages/interactionMode.ts
//
// A pure derivation of "what should the fullscreen map's bottom sheet show
// right now" from authoritative game state + local selection. This is a UI
// state machine, not a rules engine: every branch below mirrors gating that
// already exists elsewhere in GamePage (phaseMode, isMyTurn, selection), it
// just names the combination so components don't each re-derive it with
// scattered booleans. The backend remains the sole legality authority —
// this never decides whether a command will succeed, only which controls
// to display.

export type InteractionMode =
  | { kind: "game-over" }
  | { kind: "waiting" }
  | { kind: "setup-reinforce"; armiesRemaining: number }
  | { kind: "reinforce"; territory: string | null; remaining: number }
  | { kind: "attack-select-source" }
  | { kind: "attack-select-target"; from: string }
  | { kind: "attack-ready"; from: string; to: string }
  | { kind: "occupy"; from: string; to: string; minMove: number; maxMove: number }
  | { kind: "fortify-select-source" }
  | { kind: "fortify-select-destination"; from: string }
  | { kind: "fortify-ready"; from: string; to: string };

export interface DeriveInteractionModeInput {
  phaseMode: string;
  isGameOver: boolean;
  isMyTurn: boolean;
  pendingReinforcements: number;
  mySetupArmies: number;
  selectedTerritory: string;
  selectedFrom: string;
  selectedTo: string;
  occupyRequirement: { from: string; to: string; minMove: number; maxMove: number } | null;
}

export function deriveInteractionMode(input: DeriveInteractionModeInput): InteractionMode {
  const {
    phaseMode,
    isGameOver,
    isMyTurn,
    pendingReinforcements,
    mySetupArmies,
    selectedTerritory,
    selectedFrom,
    selectedTo,
    occupyRequirement,
  } = input;

  if (isGameOver) return { kind: "game-over" };

  // Setup placement is per-player, not turn-gated — every player places
  // simultaneously whenever they have armies left, independent of whose
  // "turn" it nominally is.
  if (phaseMode === "setup_reinforce") {
    return { kind: "setup-reinforce", armiesRemaining: mySetupArmies };
  }

  if (!isMyTurn) return { kind: "waiting" };

  if (phaseMode === "occupy" && occupyRequirement) {
    return {
      kind: "occupy",
      from: occupyRequirement.from,
      to: occupyRequirement.to,
      minMove: occupyRequirement.minMove,
      maxMove: occupyRequirement.maxMove,
    };
  }

  if (phaseMode === "reinforce") {
    return { kind: "reinforce", territory: selectedTerritory || null, remaining: pendingReinforcements };
  }

  if (phaseMode === "attack") {
    if (!selectedFrom) return { kind: "attack-select-source" };
    if (!selectedTo) return { kind: "attack-select-target", from: selectedFrom };
    return { kind: "attack-ready", from: selectedFrom, to: selectedTo };
  }

  if (phaseMode === "fortify") {
    if (!selectedFrom) return { kind: "fortify-select-source" };
    if (!selectedTo) return { kind: "fortify-select-destination", from: selectedFrom };
    return { kind: "fortify-ready", from: selectedFrom, to: selectedTo };
  }

  return { kind: "waiting" };
}
