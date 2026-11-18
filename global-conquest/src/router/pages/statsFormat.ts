// Non-component helpers for the admin stats pages (kept apart from
// statsShared.tsx so React fast refresh works). Series colors are the
// dataviz reference palette's dark steps, validated against this app's
// #141c28 surface (adjacent CVD ΔE >= 9.4, contrast >= 3:1).
import type { StatsFilters } from "../../api/stats";

export const SERIES = {
  blue: "#3987e5",
  orange: "#d95926",
  aqua: "#199e70",
} as const;

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
