import { request } from "./client";

/** Admin stats filters; mirrors backend store.GameResultsFilter query params. */
export type StatsFilters = {
  composition?: "all" | "human_only" | "mixed" | "bot_only";
  map?: "all" | "classic" | "custom";
  player_count?: number;
  include_sandbox?: boolean;
  /** YYYY-MM-DD, inclusive */
  from?: string;
  /** YYYY-MM-DD, inclusive */
  to?: string;
};

export type PlayerSummary = {
  key: string;
  name: string;
  is_bot: boolean;
  games: number;
  wins: number;
  losses: number;
  win_pct: number;
  wl_ratio: number | null;
  avg_placement: number | null;
  expected_win_pct: number;
  history_games: number;
  attacks: number;
  conquests: number;
  eliminations: number;
  card_trades: number;
  armies_destroyed: number;
  armies_lost: number;
  kd_ratio: number | null;
  conquests_per_game: number | null;
  current_streak: number;
  longest_win_streak: number;
  first_played: string;
  last_played: string;
  recent: boolean[];
};

export type PlayerSeriesPoint = {
  game_id: string;
  completed_at: string;
  won: boolean;
  placement: number | null;
  player_count: number;
  cum_wins: number;
  cum_losses: number;
  win_pct: number;
  wl_ratio: number | null;
  rolling_win_pct: number;
  game_kd: number | null;
};

export type RecordBreakdown = {
  player_count: number;
  games: number;
  wins: number;
  losses: number;
  win_pct: number;
  expected_win_pct: number;
};

export type HeadToHead = {
  key: string;
  name: string;
  is_bot: boolean;
  games: number;
  wins: number;
  opponent_wins: number;
  finished_ahead: number;
  placement_games: number;
};

export type PlayerGameEntry = {
  game_id: string;
  completed_at: string;
  duration_seconds: number;
  map_name: string;
  player_count: number;
  turn_count: number;
  won: boolean;
  placement: number | null;
  winner_name: string;
  opponents: string[];
  conquests: number | null;
  eliminations: number | null;
  armies_destroyed: number | null;
  armies_lost: number | null;
};

export type PlayerDetail = {
  summary: PlayerSummary;
  series: PlayerSeriesPoint[];
  by_player_count: RecordBreakdown[];
  placements: { placement: number; count: number }[];
  opponents: HeadToHead[];
  games: PlayerGameEntry[];
};

export type OverviewBucket = {
  start: string;
  games: number;
  active_humans: number;
  avg_turns: number | null;
  avg_duration_sec: number | null;
  mixed_games: number;
  human_win_pct_mixed: number | null;
};

export type StatsOverview = {
  games: number;
  human_players: number;
  human_only_games: number;
  mixed_games: number;
  bot_only_games: number;
  avg_turns: number | null;
  avg_duration_sec: number | null;
  bucket: "day" | "week";
  series: OverviewBucket[];
  by_player_count: { player_count: number; games: number }[];
  bot_strategies: { strategy: string; games: number; wins: number; win_pct: number; expected_win_pct: number }[];
};

function params(f: StatsFilters): Record<string, string> {
  const out: Record<string, string> = {};
  if (f.composition && f.composition !== "all") out.composition = f.composition;
  if (f.map && f.map !== "all") out.map = f.map;
  if (f.player_count) out.player_count = String(f.player_count);
  if (f.include_sandbox) out.include_sandbox = "true";
  if (f.from) out.from = f.from;
  if (f.to) out.to = f.to;
  return out;
}

export function getStatsOverview(f: StatsFilters): Promise<StatsOverview> {
  return request<StatsOverview>({ method: "GET", url: "/admin/stats/overview", params: params(f) });
}

export async function listStatsPlayers(f: StatsFilters): Promise<PlayerSummary[]> {
  return (await request<PlayerSummary[] | null>({ method: "GET", url: "/admin/stats/players", params: params(f) })) ?? [];
}

export function getStatsPlayer(key: string, f: StatsFilters): Promise<PlayerDetail> {
  return request<PlayerDetail>({
    method: "GET",
    url: `/admin/stats/players/${encodeURIComponent(key)}`,
    params: params(f),
  });
}

export type DecisionOddsBucket = {
  label: string;
  attacks: number;
  chosen_pct: number;
  available_pct: number;
  preference: number | null;
  capture_pct: number | null;
  expected_capture_pct: number | null;
};

export type DecisionFeature = {
  key: string;
  label: string;
  chosen: number;
  chosen_pct: number;
  available_pct: number;
  preference: number | null;
};

export type PlayerDecisionStats = {
  games: number;
  rolls: number;
  attacks: number;
  stops: number;
  avg_win_prob: number | null;
  avg_rolls_per_attack: number | null;
  abandoned_pct: number | null;
  max_dice_pct: number | null;
  stops_with_good_odds_pct: number | null;
  kill_luck_pct: number | null;
  loss_luck_pct: number | null;
  odds_buckets: DecisionOddsBucket[];
  features: DecisionFeature[];
  stop_buckets: { label: string; stops: number; pct: number }[];
  series: {
    game_id: string;
    completed_at: string;
    attacks: number;
    avg_win_prob: number | null;
    cum_kill_luck_pct: number | null;
  }[];
};

export function getStatsPlayerDecisions(key: string, f: StatsFilters): Promise<PlayerDecisionStats> {
  return request<PlayerDecisionStats>({
    method: "GET",
    url: `/admin/stats/players/${encodeURIComponent(key)}/decisions`,
    params: params(f),
  });
}
