import { describe, expect, it } from "vitest";
import { deriveInteractionMode, type DeriveInteractionModeInput } from "./interactionMode";

function base(overrides: Partial<DeriveInteractionModeInput> = {}): DeriveInteractionModeInput {
  return {
    phaseMode: "reinforce",
    isGameOver: false,
    isMyTurn: true,
    pendingReinforcements: 3,
    mySetupArmies: 0,
    selectedTerritory: "",
    selectedFrom: "",
    selectedTo: "",
    occupyRequirement: null,
    ...overrides,
  };
}

describe("deriveInteractionMode", () => {
  it("returns game-over regardless of anything else", () => {
    const mode = deriveInteractionMode(base({ isGameOver: true, isMyTurn: false, phaseMode: "attack" }));
    expect(mode).toEqual({ kind: "game-over" });
  });

  it("returns setup-reinforce during setup even when it isn't nominally the player's turn", () => {
    const mode = deriveInteractionMode(base({ phaseMode: "setup_reinforce", isMyTurn: false, mySetupArmies: 2 }));
    expect(mode).toEqual({ kind: "setup-reinforce", armiesRemaining: 2 });
  });

  it("returns waiting when it isn't the local player's turn outside setup", () => {
    const mode = deriveInteractionMode(base({ phaseMode: "attack", isMyTurn: false }));
    expect(mode).toEqual({ kind: "waiting" });
  });

  it("returns waiting during occupy if somehow not the local player's turn (defensive)", () => {
    const mode = deriveInteractionMode(
      base({
        phaseMode: "occupy",
        isMyTurn: false,
        occupyRequirement: { from: "Alaska", to: "Alberta", minMove: 1, maxMove: 3 },
      }),
    );
    expect(mode).toEqual({ kind: "waiting" });
  });

  it("returns occupy with the authoritative min/max when it is the player's turn", () => {
    const mode = deriveInteractionMode(
      base({
        phaseMode: "occupy",
        occupyRequirement: { from: "Alaska", to: "Alberta", minMove: 1, maxMove: 3 },
      }),
    );
    expect(mode).toEqual({ kind: "occupy", from: "Alaska", to: "Alberta", minMove: 1, maxMove: 3 });
  });

  it("returns reinforce with the currently selected territory and remaining count", () => {
    const mode = deriveInteractionMode(
      base({ phaseMode: "reinforce", selectedTerritory: "Quebec", pendingReinforcements: 4 }),
    );
    expect(mode).toEqual({ kind: "reinforce", territory: "Quebec", remaining: 4 });
  });

  it("returns reinforce with a null territory when nothing is selected yet", () => {
    const mode = deriveInteractionMode(base({ phaseMode: "reinforce", selectedTerritory: "" }));
    expect(mode).toEqual({ kind: "reinforce", territory: null, remaining: 3 });
  });

  it("walks through attack source -> target -> ready as selection fills in", () => {
    expect(deriveInteractionMode(base({ phaseMode: "attack" }))).toEqual({ kind: "attack-select-source" });
    expect(deriveInteractionMode(base({ phaseMode: "attack", selectedFrom: "Alaska" }))).toEqual({
      kind: "attack-select-target",
      from: "Alaska",
    });
    expect(
      deriveInteractionMode(base({ phaseMode: "attack", selectedFrom: "Alaska", selectedTo: "Kamchatka" })),
    ).toEqual({ kind: "attack-ready", from: "Alaska", to: "Kamchatka" });
  });

  it("walks through fortify source -> destination -> ready as selection fills in", () => {
    expect(deriveInteractionMode(base({ phaseMode: "fortify" }))).toEqual({ kind: "fortify-select-source" });
    expect(deriveInteractionMode(base({ phaseMode: "fortify", selectedFrom: "Ontario" }))).toEqual({
      kind: "fortify-select-destination",
      from: "Ontario",
    });
    expect(
      deriveInteractionMode(base({ phaseMode: "fortify", selectedFrom: "Ontario", selectedTo: "Quebec" })),
    ).toEqual({ kind: "fortify-ready", from: "Ontario", to: "Quebec" });
  });

  it("falls back to waiting for an unrecognized phase", () => {
    const mode = deriveInteractionMode(base({ phaseMode: "some_future_phase" }));
    expect(mode).toEqual({ kind: "waiting" });
  });
});
