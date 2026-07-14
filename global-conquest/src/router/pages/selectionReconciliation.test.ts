import { describe, expect, it } from "vitest";
import { reconcileSelection, type ReconcileSelectionInput } from "./selectionReconciliation";

const territories = {
  Alaska: { owner: 0, armies: 5 },
  Alberta: { owner: 1, armies: 2 },
  Ontario: { owner: 0, armies: 3 },
};

function base(overrides: Partial<ReconcileSelectionInput> = {}): ReconcileSelectionInput {
  return {
    currentPlayer: 0,
    previousCurrentPlayer: 0,
    territoryState: territories,
    meIndex: 0,
    phaseMode: "attack",
    selectedTerritory: "",
    selectedFrom: "",
    selectedTo: "",
    ...overrides,
  };
}

describe("reconcileSelection", () => {
  it("clears everything when the turn changes", () => {
    const result = reconcileSelection(
      base({ previousCurrentPlayer: 0, currentPlayer: 1, selectedFrom: "Alaska", selectedTo: "Alberta", selectedTerritory: "Ontario" }),
    );
    expect(result).toEqual({ selectedTerritory: "", selectedFrom: "", selectedTo: "" });
  });

  it("leaves a fully valid selection untouched when the turn hasn't changed", () => {
    const result = reconcileSelection(base({ selectedFrom: "Alaska", selectedTo: "Alberta" }));
    expect(result).toEqual({ selectedTerritory: "", selectedFrom: "Alaska", selectedTo: "Alberta" });
  });

  it("clears a reinforce-target territory no longer owned by the local player", () => {
    const result = reconcileSelection(
      base({ phaseMode: "reinforce", selectedTerritory: "Alberta" /* owned by player 1 */ }),
    );
    expect(result.selectedTerritory).toBe("");
  });

  it("clears both from and to when the source territory was lost", () => {
    // Alaska now belongs to someone else (was captured away from meIndex=0).
    const state = { ...territories, Alaska: { owner: 1, armies: 4 } };
    const result = reconcileSelection(base({ territoryState: state, selectedFrom: "Alaska", selectedTo: "Alberta" }));
    expect(result.selectedFrom).toBe("");
    expect(result.selectedTo).toBe("");
  });

  it("clears the attack target once the local player has captured it and occupy has resolved", () => {
    // Alberta now belongs to meIndex (0) — the attack succeeded and occupation is done.
    const state = { ...territories, Alberta: { owner: 0, armies: 2 } };
    const result = reconcileSelection(
      base({ phaseMode: "attack", territoryState: state, selectedFrom: "Alaska", selectedTo: "Alberta" }),
    );
    expect(result.selectedFrom).toBe("Alaska");
    expect(result.selectedTo).toBe("");
  });

  it("keeps the attack target while occupy is still in progress even though the player now owns it", () => {
    const state = { ...territories, Alberta: { owner: 0, armies: 2 } };
    const result = reconcileSelection(
      base({ phaseMode: "occupy", territoryState: state, selectedFrom: "Alaska", selectedTo: "Alberta" }),
    );
    expect(result.selectedTo).toBe("Alberta");
  });

  it("preserves selection across repeated attacks against a target that remains legally selectable", () => {
    // Nothing about ownership or turn changed — target still enemy-owned.
    const result = reconcileSelection(base({ phaseMode: "attack", selectedFrom: "Alaska", selectedTo: "Alberta" }));
    expect(result).toEqual({ selectedTerritory: "", selectedFrom: "Alaska", selectedTo: "Alberta" });
  });
});
