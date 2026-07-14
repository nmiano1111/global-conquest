// src/router/pages/selectionReconciliation.ts
//
// Pure decision logic for reconciling local territory selection against a
// freshly arrived authoritative game state. Extracted out of GamePage so
// the rules ("clear X when Y") are directly unit-testable without mounting
// the full page (auth/socket/router context). GamePage calls this and
// applies the resulting setState calls itself.

export interface ReconcileSelectionInput {
  currentPlayer: number;
  previousCurrentPlayer: number | undefined;
  territoryState: Record<string, unknown> | null;
  meIndex: number;
  phaseMode: string;
  selectedTerritory: string;
  selectedFrom: string;
  selectedTo: string;
}

export interface ReconcileSelectionResult {
  selectedTerritory: string;
  selectedFrom: string;
  selectedTo: string;
}

function ownerOf(name: string, territoryState: Record<string, unknown> | null): number {
  if (!name || !territoryState) return -1;
  const raw = territoryState[name];
  const t = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : null;
  return typeof t?.owner === "number" ? t.owner : -1;
}

/**
 * Given the previous local selection and a freshly arrived authoritative
 * state, returns the selection that should now be in effect:
 *  - Turn changed → every phase-specific selection starts fresh.
 *  - A selected source/reinforce-target territory no longer owned by the
 *    local player is cleared (lost it to attack, or otherwise changed).
 *  - A selected attack target the local player now owns (just captured
 *    it) is cleared once occupy has resolved, so the next attack starts
 *    clean — while still occupying, the occupy UI takes over via its own
 *    from/to fields, so this only fires after that.
 * Never touches camera/viewport state — this is selection only.
 */
export function reconcileSelection(input: ReconcileSelectionInput): ReconcileSelectionResult {
  const { currentPlayer, previousCurrentPlayer, territoryState, meIndex, phaseMode, selectedTerritory, selectedFrom, selectedTo } = input;

  if (previousCurrentPlayer !== currentPlayer) {
    return { selectedTerritory: "", selectedFrom: "", selectedTo: "" };
  }

  let nextTerritory = selectedTerritory;
  let nextFrom = selectedFrom;
  let nextTo = selectedTo;

  if (selectedTerritory && ownerOf(selectedTerritory, territoryState) !== meIndex) {
    nextTerritory = "";
  }

  if (selectedFrom && ownerOf(selectedFrom, territoryState) !== meIndex) {
    nextFrom = "";
    nextTo = "";
  } else if (selectedTo && phaseMode !== "occupy" && ownerOf(selectedTo, territoryState) === meIndex) {
    nextTo = "";
  }

  return { selectedTerritory: nextTerritory, selectedFrom: nextFrom, selectedTo: nextTo };
}
