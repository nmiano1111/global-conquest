import { describe, expect, it } from "vitest";
import type { PlayerSummary } from "../../api/stats";
import { mergeLines, parseCompare, validatePlayerStatsSearch, withCumulativeKD, type Line, type Pt } from "./statsFormat";

const pt = (n: number, date: string, win_pct: number): Pt => ({
  n,
  game_id: `${date}-${n}`,
  completed_at: date,
  won: false,
  placement: null,
  player_count: 4,
  cum_wins: 0,
  cum_losses: n,
  win_pct,
  wl_ratio: 0,
  rolling_win_pct: win_pct,
  game_kd: null,
});
const line = (key: string, points: Pt[]): Line => ({ key, name: key, color: "#000", summary: {} as PlayerSummary, points });

describe("mergeLines", () => {
  const a = line("a", [pt(1, "2026-01-01T00:00:00Z", 10), pt(2, "2026-01-03T00:00:00Z", 20)]);
  const b = line("b", [pt(1, "2026-01-02T00:00:00Z", 50), pt(2, "2026-01-03T00:00:00Z", 60), pt(3, "2026-01-04T00:00:00Z", 70)]);

  it("by date: carries values forward, blank before first and after last game", () => {
    const rows = mergeLines([a, b], "date");
    expect(rows).toHaveLength(4); // shared Jan 3 timestamp collapses into one row
    expect(rows.map((r) => r.at.a?.win_pct)).toEqual([10, 10, 20, undefined]);
    expect(rows.map((r) => r.at.b?.win_pct)).toEqual([undefined, 50, 60, 70]);
  });

  it("by game number: aligns each player's nth game", () => {
    const rows = mergeLines([a, b], "game");
    expect(rows.map((r) => r.x)).toEqual([1, 2, 3]);
    expect(rows.map((r) => r.at.a?.win_pct)).toEqual([10, 20, undefined]);
    expect(rows.map((r) => r.at.b?.win_pct)).toEqual([50, 60, 70]);
  });
});

describe("withCumulativeKD", () => {
  it("sums destroyed and lost over games with combat history", () => {
    const pts = [pt(1, "2026-01-01T00:00:00Z", 0), pt(2, "2026-01-02T00:00:00Z", 0), pt(3, "2026-01-03T00:00:00Z", 0)];
    const combat = new Map([
      [pts[0].game_id, { destroyed: 10, lost: 0 }],
      [pts[1].game_id, { destroyed: null, lost: null }],
      [pts[2].game_id, { destroyed: 20, lost: 10 }],
    ]);
    expect(withCumulativeKD(pts, combat).map((p) => p.cum_kd)).toEqual([null, null, 3]);
  });
});

describe("compare search param", () => {
  it("dedupes, trims, and caps the list", () => {
    expect(parseCompare(" bot:x, bot:x ,u1,u2")).toEqual(["bot:x", "u1"]);
    expect(validatePlayerStatsSearch({ compare: "", map: "classic" })).toEqual({ map: "classic" });
  });
});
