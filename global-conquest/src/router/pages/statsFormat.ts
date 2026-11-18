// Non-component helpers for the admin stats pages (kept apart from
// statsShared.tsx so React fast refresh works). Series colors are the
// dataviz reference palette's dark steps, validated against this app's
// #141c28 surface (adjacent CVD ΔE >= 9.4, contrast >= 3:1).
import type { PlayerSeriesPoint, PlayerSummary, StatsFilters } from "../../api/stats";

export const SERIES = {
  blue: "#3987e5",
  orange: "#d95926",
  aqua: "#199e70",
} as const;

/**
 * Player colors when comparing on the player page: the page's own player is
 * always slot 1. Capped at three because lines cross, so every pair must stay
 * distinguishable; the fourth slot (yellow) fails the all-pairs CVD check
 * against orange.
 */
export const PLAYER_COLORS = [SERIES.blue, SERIES.orange, SERIES.aqua] as const;
export const MAX_COMPARE = PLAYER_COLORS.length - 1;

// ---------- formatting ----------

export const fmtPct = (v: number | null | undefined, digits = 0) =>
  v == null ? "—" : `${v.toFixed(digits)}%`;
export const fmtNum = (v: number | null | undefined, digits = 2) =>
  v == null ? "—" : v.toFixed(digits);
export const fmtDate = (iso: string | number) =>
  new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "2-digit" });
export function fmtDuration(sec: number | null | undefined): string {
  if (sec == null) return "—";
  if (sec < 60) return `${Math.round(sec)}s`;
  const m = Math.round(sec / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h ${m % 60}m`;
  return `${Math.round(h / 24)}d`;
}
export function fmtStreak(n: number): string {
  if (n === 0) return "—";
  return n > 0 ? `W${n}` : `L${-n}`;
}

// ---------- filters (kept in the URL search params) ----------

export function validateStatsSearch(search: Record<string, unknown>): StatsFilters {
  const out: StatsFilters = {};
  const comp = search.composition;
  if (comp === "human_only" || comp === "mixed" || comp === "bot_only") out.composition = comp;
  const map = search.map;
  if (map === "classic" || map === "custom") out.map = map;
  const pc = Number(search.player_count);
  if (Number.isInteger(pc) && pc > 0) out.player_count = pc;
  if (search.include_sandbox === true || search.include_sandbox === "true") out.include_sandbox = true;
  const date = /^\d{4}-\d{2}-\d{2}$/;
  if (typeof search.from === "string" && date.test(search.from)) out.from = search.from;
  if (typeof search.to === "string" && date.test(search.to)) out.to = search.to;
  return out;
}

/** Player page search: the shared filters plus `compare`, a comma-separated list of player keys. */
export type PlayerStatsSearch = StatsFilters & { compare?: string };

export function validatePlayerStatsSearch(search: Record<string, unknown>): PlayerStatsSearch {
  const out: PlayerStatsSearch = validateStatsSearch(search);
  const keys = parseCompare(typeof search.compare === "string" ? search.compare : undefined);
  if (keys.length > 0) out.compare = keys.join(",");
  return out;
}

export function parseCompare(raw: string | undefined): string[] {
  if (!raw) return [];
  return [...new Set(raw.split(",").map((k) => k.trim()).filter(Boolean))].slice(0, MAX_COMPARE);
}

// ---------- player comparison ----------

export type XMode = "date" | "game";
/** A player's series point plus its 1-based game number. */
export type Pt = PlayerSeriesPoint & { n: number; cum_kd?: number | null };

/**
 * Adds each point's career K/D so far (armies destroyed ÷ lost, summed over
 * games with combat history). `combat` maps game_id to that game's totals.
 */
export function withCumulativeKD(points: Pt[], combat: Map<string, { destroyed: number | null; lost: number | null }>): Pt[] {
  let destroyed = 0;
  let lost = 0;
  return points.map((p) => {
    const c = combat.get(p.game_id);
    if (c && c.destroyed != null && c.lost != null) {
      destroyed += c.destroyed;
      lost += c.lost;
    }
    return { ...p, cum_kd: lost > 0 ? destroyed / lost : null };
  });
}
/** One player's line set in compare mode. */
export type Line = { key: string; name: string; color: string; summary: PlayerSummary; points: Pt[] };
/**
 * A merged x position across compared players. `at` holds each player's
 * latest point at or before x (cumulative stats hold their value between
 * games), undefined before their first or after their last game.
 */
export type MergedRow = { x: number; at: Record<string, Pt | undefined> };

/** Aligns several players' series on one x axis: by date, or by each player's own game number. */
export function mergeLines(lines: Line[], mode: XMode): MergedRow[] {
  if (mode === "game") {
    const len = Math.max(0, ...lines.map((l) => l.points.length));
    return Array.from({ length: len }, (_, i) => {
      const at: MergedRow["at"] = {};
      for (const l of lines) at[l.key] = l.points[i];
      return { x: i + 1, at };
    });
  }
  const events = lines
    .flatMap((l) => l.points.map((p) => ({ key: l.key, t: new Date(p.completed_at).getTime(), p })))
    .sort((a, b) => a.t - b.t);
  const lastT: Record<string, number> = {};
  for (const l of lines) lastT[l.key] = l.points.length ? new Date(l.points[l.points.length - 1].completed_at).getTime() : -Infinity;
  const latest: Record<string, Pt> = {};
  const out: MergedRow[] = [];
  for (let i = 0; i < events.length; ) {
    const t = events[i].t;
    for (; i < events.length && events[i].t === t; i++) latest[events[i].key] = events[i].p;
    const at: MergedRow["at"] = {};
    for (const l of lines) at[l.key] = t <= lastT[l.key] ? latest[l.key] : undefined;
    out.push({ x: t, at });
  }
  return out;
}
