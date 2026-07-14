import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { RefObject } from "react";
import type { GameBootstrap } from "../../api/games";
import type { GameMapHandle } from "../../map/GameMap";
import { FullscreenGameMap, type FullscreenGameMapProps } from "./FullscreenGameMap";

// FullscreenGameMap no longer renders <GameMap> itself — it's owned and
// portaled in by GamePage (see GamePage's embeddedMapSlotEl/fullscreenMapSlotEl
// comment for why: sharing one Pixi Application between the embedded and
// fullscreen views, rather than creating/destroying a second one, avoids a
// real PixiJS bug where destroying any renderer wipes a shared global
// texture pool out from under any other still-live renderer). These tests
// cover FullscreenGameMap's own responsibilities — the shell, top bar,
// bottom sheet, and mapSlotRef wiring — not GameMap/MapScene itself.

function makePlayer(overrides: Partial<GameBootstrap["players"][number]> = {}): GameBootstrap["players"][number] {
  return {
    userId: "u1",
    userName: "Alice",
    color: "#ef4444",
    cardCount: 0,
    cards: [],
    setupArmies: 0,
    eliminated: false,
    isBot: false,
    ...overrides,
  };
}

function makeGame(overrides: Partial<GameBootstrap> = {}): GameBootstrap {
  return {
    id: "game-1",
    ownerUserId: "u1",
    name: "Test Game",
    status: "in_progress",
    phase: "attack",
    playerCount: 2,
    currentPlayer: 0,
    pendingReinforcements: 0,
    setsTraded: 0,
    occupy: null,
    players: [makePlayer(), makePlayer({ userId: "u2", userName: "Bob", color: "#3b82f6" })],
    territories: {
      Alaska: { owner: 0, armies: 5 },
      Kamchatka: { owner: 1, armies: 2 },
    },
    events: [],
    createdAt: "",
    updatedAt: "",
    ...overrides,
  };
}

function baseProps(overrides: Partial<FullscreenGameMapProps> = {}): FullscreenGameMapProps {
  return {
    game: makeGame(),
    onClose: vi.fn(),
    mapRef: { current: null } as RefObject<GameMapHandle | null>,
    mapSlotRef: vi.fn(),
    phase: "attack",
    phaseMode: "attack",
    isMyTurn: true,
    isGameOver: false,
    wsStatus: "connected",
    players: makeGame().players,
    playerColors: ["#ef4444", "#3b82f6"],
    territoryState: { Alaska: { owner: 0, armies: 5 }, Kamchatka: { owner: 1, armies: 2 } },
    pendingReinforcements: 0,
    mySetupArmies: 0,
    occupyRequirement: null,
    diceResult: null,
    selectedTerritory: "",
    selectedFrom: "",
    selectedTo: "",
    activeFrom: "",
    activeTo: "",
    clampedArmiesInput: 1,
    minArmiesInput: 1,
    maxArmiesInput: 5,
    clampedAttackerDice: 1,
    maxAttackDiceAllowed: 3,
    maxDefendDiceAllowed: 2,
    canAttackSelection: false,
    commitReinforcement: vi.fn(),
    commitFortify: vi.fn(),
    commitOccupy: vi.fn(),
    onRollDice: vi.fn(),
    setArmiesInput: vi.fn(),
    setAttackerDice: vi.fn(),
    sendAction: vi.fn(),
    setSelectedFrom: vi.fn(),
    setSelectedTo: vi.fn(),
    setSelectedTerritory: vi.fn(),
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  document.body.style.overflow = "";
  document.body.style.touchAction = "";
});

describe("FullscreenGameMap", () => {
  it("renders into a document.body portal", () => {
    render(<FullscreenGameMap {...baseProps()} />);
    expect(screen.getByTestId("fullscreen-map-root")).toBeInTheDocument();
    expect(document.body.contains(screen.getByTestId("fullscreen-map-root"))).toBe(true);
  });

  it("calls onClose when the exit button is clicked", () => {
    const onClose = vi.fn();
    render(<FullscreenGameMap {...baseProps({ onClose })} />);
    fireEvent.click(screen.getByLabelText("Exit fullscreen map"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose on Escape", () => {
    const onClose = vi.fn();
    render(<FullscreenGameMap {...baseProps({ onClose })} />);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("locks body scroll while mounted and restores it on unmount", () => {
    const { unmount } = render(<FullscreenGameMap {...baseProps()} />);
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("marks the rest of the app inert while open, so background controls aren't reachable, and restores it on close", () => {
    const appRoot = document.createElement("div");
    appRoot.id = "root";
    document.body.appendChild(appRoot);
    try {
      const { unmount } = render(<FullscreenGameMap {...baseProps()} />);
      expect(appRoot.hasAttribute("inert")).toBe(true);
      unmount();
      expect(appRoot.hasAttribute("inert")).toBe(false);
    } finally {
      document.body.removeChild(appRoot);
    }
  });

  it("registers its map container as the portal target on mount, and deregisters it on unmount", () => {
    // React's dev build calls ref-cleanup functions through an internal
    // wrapper (runWithFiberInDEV) that appends extra trailing `undefined`
    // arguments beyond the single value React actually intends to pass —
    // harmless for real callers (state setters only read the first arg),
    // but means asserting the exact call signature (toHaveBeenLastCalledWith)
    // is too brittle here; check the first argument of each call instead.
    const firstArgs: unknown[] = [];
    const mapSlotRef = (node: HTMLDivElement | null) => {
      firstArgs.push(node);
    };
    const { unmount } = render(<FullscreenGameMap {...baseProps({ mapSlotRef })} />);
    expect(firstArgs).toHaveLength(1);
    expect(firstArgs[0]).toBeInstanceOf(HTMLDivElement);

    unmount();
    expect(firstArgs).toHaveLength(2);
    expect(firstArgs[1]).toBeNull();
  });

  it("shows a waiting status and hides mutation controls when it isn't the local player's turn", () => {
    render(<FullscreenGameMap {...baseProps({ isMyTurn: false })} />);
    expect(screen.getByTestId("status-overlay")).toBeInTheDocument();
    expect(screen.queryByTestId("action-sheet")).not.toBeInTheDocument();
    expect(screen.queryByText(/roll dice/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^place/i)).not.toBeInTheDocument();
  });

  it("shows a Place button in reinforce mode once a territory is selected, and commits on click", () => {
    const commitReinforcement = vi.fn();
    render(
      <FullscreenGameMap
        {...baseProps({
          phaseMode: "reinforce",
          selectedTerritory: "Alaska",
          pendingReinforcements: 3,
          commitReinforcement,
        })}
      />,
    );
    const placeButton = screen.getByText(/place 1 army/i);
    fireEvent.click(placeButton);
    expect(commitReinforcement).toHaveBeenCalledTimes(1);
  });

  it("disables the Place button in reinforce mode until a territory is selected", () => {
    render(<FullscreenGameMap {...baseProps({ phaseMode: "reinforce", selectedTerritory: "", pendingReinforcements: 3 })} />);
    expect(screen.getByText(/place \d+ army/i)).toBeDisabled();
  });

  it("gates the Roll Dice button on canAttackSelection and fires onRollDice", () => {
    const onRollDice = vi.fn();
    const { rerender } = render(
      <FullscreenGameMap
        {...baseProps({
          phaseMode: "attack",
          selectedFrom: "Alaska",
          selectedTo: "Kamchatka",
          canAttackSelection: false,
          onRollDice,
        })}
      />,
    );
    expect(screen.getByText(/roll dice/i)).toBeDisabled();

    rerender(
      <FullscreenGameMap
        {...baseProps({
          phaseMode: "attack",
          selectedFrom: "Alaska",
          selectedTo: "Kamchatka",
          canAttackSelection: true,
          onRollDice,
        })}
      />,
    );
    const rollButton = screen.getByText(/roll dice/i);
    expect(rollButton).not.toBeDisabled();
    fireEvent.click(rollButton);
    expect(onRollDice).toHaveBeenCalledTimes(1);
  });

  it("transitions into the occupy sheet and commits the chosen army count", () => {
    const commitOccupy = vi.fn();
    render(
      <FullscreenGameMap
        {...baseProps({
          phaseMode: "occupy",
          occupyRequirement: { from: "Alaska", to: "Kamchatka", minMove: 1, maxMove: 4 },
          clampedArmiesInput: 2,
          commitOccupy,
        })}
      />,
    );
    expect(screen.getByTestId("occupy-sheet")).toBeInTheDocument();
    expect(screen.getByText(/territory conquered/i)).toBeInTheDocument();
    fireEvent.click(screen.getByText(/move 2 troops/i));
    expect(commitOccupy).toHaveBeenCalledTimes(1);
  });

  it("starts with the bottom sheet open, and tapping its handle collapses/expands it", () => {
    render(<FullscreenGameMap {...baseProps({ phaseMode: "reinforce" })} />);
    const handle = screen.getByLabelText("Collapse action panel");
    expect(handle).toHaveAttribute("aria-expanded", "true");

    fireEvent.click(handle);
    const collapsedHandle = screen.getByLabelText("Expand action panel");
    expect(collapsedHandle).toHaveAttribute("aria-expanded", "false");
    // Content stays mounted (so state/selection isn't lost) — only its
    // rendered height collapses via CSS, not removal from the DOM.
    expect(screen.getByText(/tap one of your territories/i)).toBeInTheDocument();

    fireEvent.click(collapsedHandle);
    expect(screen.getByLabelText("Collapse action panel")).toHaveAttribute("aria-expanded", "true");
  });

  it("collapses the sheet when the handle is dragged down, and expands it when dragged up", () => {
    render(<FullscreenGameMap {...baseProps({ phaseMode: "reinforce" })} />);
    const handle = screen.getByLabelText("Collapse action panel");

    fireEvent.pointerDown(handle, { clientY: 100 });
    fireEvent.pointerUp(handle, { clientY: 160 }); // dragged down 60px -> collapse
    expect(screen.getByLabelText("Expand action panel")).toHaveAttribute("aria-expanded", "false");

    const collapsedHandle = screen.getByLabelText("Expand action panel");
    fireEvent.pointerDown(collapsedHandle, { clientY: 160 });
    fireEvent.pointerUp(collapsedHandle, { clientY: 100 }); // dragged up 60px -> expand
    expect(screen.getByLabelText("Collapse action panel")).toHaveAttribute("aria-expanded", "true");
  });

  it("treats a small handle movement as a tap toggle rather than a drag", () => {
    render(<FullscreenGameMap {...baseProps({ phaseMode: "reinforce" })} />);
    const handle = screen.getByLabelText("Collapse action panel");
    fireEvent.pointerDown(handle, { clientY: 100 });
    fireEvent.pointerUp(handle, { clientY: 102 }); // 2px — well under the drag threshold
    expect(screen.getByLabelText("Expand action panel")).toHaveAttribute("aria-expanded", "false");
  });
});
