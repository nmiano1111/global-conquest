import type { GameRecord } from "../../api/games";

export type GamePlayerSummary = {
  id: string;
  cardCount: number;
  eliminated: boolean;
  isCurrent: boolean;
};

export type GameChatMessage = {
  gameID: string;
  userName: string;
  body: string;
  createdAt: string;
};

export type GameEventMessage = {
  id: string;
  gameID: string;
  actorUserID: string;
  eventType: string;
  body: string;
  createdAt: string;
};

export type DiceRollResult = {
  attacker: number[];
  defender: number[];
  attackerLoss: number;
  defenderLoss: number;
};

type MapTerritoryNode = { x: number; y: number };

export const MAP_PLAYER_COLORS = ["#ef4444", "#3b82f6", "#22c55e", "#f59e0b", "#a855f7", "#06b6d4"];

export const MAP_TERRITORIES: Record<string, MapTerritoryNode> = {
  Alaska: { x: 130, y: 210 },
  "Northwest Territory": { x: 300, y: 200 },
  Greenland: { x: 575, y: 150 },
  Alberta: { x: 265, y: 310 },
  Ontario: { x: 380, y: 315 },
  Quebec: { x: 500, y: 330 },
  "Western United States": { x: 275, y: 440 },
  "Eastern United States": { x: 405, y: 465 },
  "Central America": { x: 300, y: 590 },
  Venezuela: { x: 410, y: 655 },
  Peru: { x: 380, y: 810 },
  Brazil: { x: 540, y: 770 },
  Argentina: { x: 445, y: 990 },
  Iceland: { x: 720, y: 270 },
  Scandinavia: { x: 855, y: 260 },
  Ukraine: { x: 990, y: 350 },
  "Great Britain": { x: 700, y: 400 },
  "Northern Europe": { x: 840, y: 415 },
  "Western Europe": { x: 740, y: 545 },
  "Southern Europe": { x: 865, y: 535 },
  "North Africa": { x: 790, y: 740 },
  Egypt: { x: 915, y: 690 },
  "East Africa": { x: 1010, y: 810 },
  Congo: { x: 910, y: 885 },
  "South Africa": { x: 930, y: 1055 },
  Madagascar: { x: 1070, y: 1020 },
  Ural: { x: 1170, y: 300 },
  Siberia: { x: 1260, y: 200 },
  Yakutsk: { x: 1405, y: 160 },
  Kamchatka: { x: 1520, y: 200 },
  Irkutsk: { x: 1365, y: 320 },
  Mongolia: { x: 1400, y: 430 },
  Japan: { x: 1580, y: 400 },
  Afghanistan: { x: 1140, y: 460 },
  "Middle East": { x: 1045, y: 630 },
  India: { x: 1220, y: 620 },
  Siam: { x: 1360, y: 685 },
  China: { x: 1325, y: 545 },
  Indonesia: { x: 1370, y: 850 },
  "New Guinea": { x: 1530, y: 800 },
  "Western Australia": { x: 1455, y: 1015 },
  "Eastern Australia": { x: 1600, y: 1000 },
};

const MAP_ADJACENCY: Record<string, string[]> = {
  Alaska: ["Northwest Territory", "Alberta", "Kamchatka"],
  "Northwest Territory": ["Alaska", "Alberta", "Ontario", "Greenland"],
  Greenland: ["Northwest Territory", "Ontario", "Quebec", "Iceland"],
  Alberta: ["Alaska", "Northwest Territory", "Ontario", "Western United States"],
  Ontario: ["Northwest Territory", "Greenland", "Quebec", "Eastern United States", "Western United States", "Alberta"],
  Quebec: ["Ontario", "Greenland", "Eastern United States"],
  "Western United States": ["Alberta", "Ontario", "Eastern United States", "Central America"],
  "Eastern United States": ["Western United States", "Ontario", "Quebec", "Central America"],
  "Central America": ["Western United States", "Eastern United States", "Venezuela"],
  Venezuela: ["Central America", "Brazil", "Peru"],
  Peru: ["Venezuela", "Brazil", "Argentina"],
  Brazil: ["Venezuela", "Peru", "Argentina", "North Africa"],
  Argentina: ["Peru", "Brazil"],
  Iceland: ["Greenland", "Great Britain", "Scandinavia"],
  Scandinavia: ["Iceland", "Great Britain", "Northern Europe", "Ukraine"],
  Ukraine: ["Scandinavia", "Northern Europe", "Southern Europe", "Middle East", "Afghanistan", "Ural"],
  "Great Britain": ["Iceland", "Scandinavia", "Northern Europe", "Western Europe"],
  "Northern Europe": ["Great Britain", "Scandinavia", "Ukraine", "Southern Europe", "Western Europe"],
  "Western Europe": ["Great Britain", "Northern Europe", "Southern Europe", "North Africa"],
  "Southern Europe": ["Western Europe", "Northern Europe", "Ukraine", "Middle East", "Egypt", "North Africa"],
  "North Africa": ["Brazil", "Western Europe", "Southern Europe", "Egypt", "East Africa", "Congo"],
  Egypt: ["North Africa", "Southern Europe", "Middle East", "East Africa"],
  "East Africa": ["Egypt", "North Africa", "Congo", "South Africa", "Madagascar", "Middle East"],
  Congo: ["North Africa", "East Africa", "South Africa"],
  "South Africa": ["Congo", "East Africa", "Madagascar"],
  Madagascar: ["South Africa", "East Africa"],
  Ural: ["Ukraine", "Siberia", "China", "Afghanistan"],
  Siberia: ["Ural", "Yakutsk", "Irkutsk", "Mongolia", "China"],
  Yakutsk: ["Siberia", "Irkutsk", "Kamchatka"],
  Kamchatka: ["Yakutsk", "Irkutsk", "Mongolia", "Japan", "Alaska"],
  Irkutsk: ["Siberia", "Yakutsk", "Kamchatka", "Mongolia"],
  Mongolia: ["Siberia", "Irkutsk", "Kamchatka", "Japan", "China"],
  Japan: ["Kamchatka", "Mongolia"],
  Afghanistan: ["Ukraine", "Ural", "China", "India", "Middle East"],
  "Middle East": ["Ukraine", "Southern Europe", "Egypt", "East Africa", "India", "Afghanistan"],
  India: ["Middle East", "Afghanistan", "China", "Siam"],
  Siam: ["India", "China", "Indonesia"],
  China: ["Mongolia", "Siberia", "Ural", "Afghanistan", "India", "Siam"],
  Indonesia: ["Siam", "New Guinea", "Western Australia"],
  "New Guinea": ["Indonesia", "Western Australia", "Eastern Australia"],
  "Western Australia": ["Indonesia", "New Guinea", "Eastern Australia"],
  "Eastern Australia": ["New Guinea", "Western Australia"],
};

export const MAP_EDGES: Array<[string, string]> = (() => {
  const out: Array<[string, string]> = [];
  const seen = new Set<string>();
  Object.entries(MAP_ADJACENCY).forEach(([from, tos]) => {
    tos.forEach((to) => {
      const key = from < to ? `${from}|${to}` : `${to}|${from}`;
      if (seen.has(key)) return;
      seen.add(key);
      out.push(from < to ? [from, to] : [to, from]);
    });
  });
  return out;
})();

export const MAP_OVERLAY_SCALE = 1.26;
export const MAP_OVERLAY_OFFSET_X = 200;
export const MAP_OVERLAY_OFFSET_Y = 100;
export const MAP_VIEWBOX_WIDTH = 2048;
export const MAP_VIEWBOX_HEIGHT = 1367;
export const MAP_CENTER_X = MAP_VIEWBOX_WIDTH / 2;
export const MAP_CENTER_Y = MAP_VIEWBOX_HEIGHT / 2;

export function summarizeGamePlayers(game: GameRecord): GamePlayerSummary[] {
  const state = game.state;
  if (state && typeof state === "object") {
    const stateRecord = state as Record<string, unknown>;
    const rawPlayers = stateRecord.players;
    const currentPlayer = typeof stateRecord.current_player === "number" ? stateRecord.current_player : -1;
    if (Array.isArray(rawPlayers)) {
      return rawPlayers
        .filter((p): p is Record<string, unknown> => !!p && typeof p === "object")
        .map((p, idx) => {
          const cards = Array.isArray(p.cards) ? p.cards : [];
          return {
            id: typeof p.id === "string" && p.id !== "" ? p.id : `player-${idx + 1}`,
            cardCount: cards.length,
            eliminated: p.eliminated === true,
            isCurrent: idx === currentPlayer,
          };
        });
    }
  }
  return game.playerIds.map((id) => ({ id, cardCount: 0, eliminated: false, isCurrent: false }));
}
