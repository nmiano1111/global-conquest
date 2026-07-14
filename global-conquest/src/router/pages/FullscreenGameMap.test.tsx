import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { forwardRef, useEffect, useImperativeHandle } from "react";
import type { GameBootstrap } from "../../api/games";
import { FullscreenGameMap, type FullscreenGameMapProps } from "./FullscreenGameMap";

const mountCalls = vi.hoisted(() => ({ current: 0 }));

vi.mock("../../map/GameMap", () => {
  const MockGameMap = forwardRef<unknown, Record<string, unknown>>((props, ref) => {
    useEffect(() => {
      mountCalls.current += 1;
    }, []);
    useImperativeHandle(ref, () => ({ zoomIn: vi.fn(), zoomOut: vi.fn(), resetZoom: vi.fn() }));
    const onTerritoryClick = props.onTerritoryClick as (name: string) => void;
    return (
      <div data-testid="mock-game-map">
        <button type="button" onClick={() => onTerritoryClick("Alaska")}>
          mock-territory-click
        </button>
      </div>
    );
  });
  return { GameMap: MockGameMap };
});

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
    legalAttackTargets: new Set(),
    recentCombatTerritories: new Set(),
    recentCaptureTerritories: new Set(),
    highlightedTerritories: new Set(),
    clampedArmiesInput: 1,
    minArmiesInput: 1,
    maxArmiesInput: 5,
    clampedAttackerDice: 1,
    maxAttackDiceAllowed: 3,
    maxDefendDiceAllowed: 2,
    canAttackSelection: false,
    onMapTerritoryClick: vi.fn(),
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

beforeEach(() => {
  mountCalls.current = 0;
});

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

  it("routes a territory tap through onMapTerritoryClick", () => {
    const onMapTerritoryClick = vi.fn();
    render(<FullscreenGameMap {...baseProps({ onMapTerritoryClick })} />);
    fireEvent.click(screen.getByText("mock-territory-click"));
    expect(onMapTerritoryClick).toHaveBeenCalledWith("Alaska");
  });

  it("does not remount the underlying map when the game prop changes (camera survives updates)", () => {
    const { rerender } = render(<FullscreenGameMap {...baseProps({ game: makeGame({ pendingReinforcements: 0 }) })} />);
    expect(mountCalls.current).toBe(1);
    rerender(<FullscreenGameMap {...baseProps({ game: makeGame({ pendingReinforcements: 2, currentPlayer: 0 }) })} />);
    rerender(<FullscreenGameMap {...baseProps({ game: makeGame({ phase: "fortify", currentPlayer: 0 }) })} />);
    expect(mountCalls.current).toBe(1);
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
});
