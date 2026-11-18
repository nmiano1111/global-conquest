import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getStatsOverview, listStatsPlayers, type PlayerSummary, type StatsFilters, type StatsOverview } from "../../api/stats";
import { useAuth } from "../../auth";
import { BarChart, FilterBar, LineChart, Panel, RecentForm, StatTile } from "./statsShared";
import { SERIES, fmtDate, fmtDuration, fmtNum, fmtPct, fmtStreak } from "./statsFormat";

type SortKey = "name" | "games" | "win_pct" | "wl_ratio" | "avg_placement" | "kd_ratio" | "conquests_per_game" | "last_played";

const COLUMNS: { key: SortKey; label: string; title?: string }[] = [
  { key: "name", label: "Player" },
  { key: "games", label: "Games" },
  { key: "win_pct", label: "Win %", title: "Win rate, with the even-odds baseline (1 ÷ players) for the same games beneath" },
  { key: "wl_ratio", label: "W/L" },
  { key: "avg_placement", label: "Avg place", title: "Average finishing position (1 = won)" },
  { key: "kd_ratio", label: "K/D", title: "Enemy armies destroyed ÷ own armies lost" },
  { key: "conquests_per_game", label: "Conq/game", title: "Territories conquered per game" },
  { key: "last_played", label: "Last played" },
];

function sortValue(p: PlayerSummary, key: SortKey): number | string {
  switch (key) {
    case "name":
      return p.name.toLowerCase();
    case "last_played":
      return p.last_played;
    case "wl_ratio":
      // No losses: rank above every finite ratio.
      return p.wl_ratio ?? (p.wins > 0 ? Infinity : -Infinity);
    default:
      return p[key] ?? -Infinity;
  }
}

export function AdminStatsPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const filters = useSearch({ from: "/app/admin/stats" });
  const [overview, setOverview] = useState<StatsOverview | null>(null);
  const [players, setPlayers] = useState<PlayerSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showBots, setShowBots] = useState(true);
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "games", desc: true });

  const load = useCallback(
    async (f: StatsFilters, isCancelled: () => boolean) => {
      setLoading(true);
      setError("");
      try {
        const [ov, ps] = await Promise.all([getStatsOverview(f), listStatsPlayers(f)]);
        if (isCancelled()) return;
        setOverview(ov);
        setPlayers(ps);
      } catch (err) {
        if (isCancelled()) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        setError(apiErr.message || "Failed to load stats");
      } finally {
        if (!isCancelled()) setLoading(false);
      }
    },
    [auth, navigate],
  );

  useEffect(() => {
    let cancelled = false;
    void load(filters, () => cancelled);
    return () => {
      cancelled = true;
    };
  }, [load, filters]);

  const setFilters = (f: StatsFilters) => void navigate({ to: "/app/admin/stats", search: f, replace: true });

  const visiblePlayers = useMemo(() => {
    const list = players.filter((p) => showBots || !p.is_bot);
    const dir = sort.desc ? -1 : 1;
    return [...list].sort((a, b) => {
      const va = sortValue(a, sort.key);
      const vb = sortValue(b, sort.key);
      return va < vb ? -dir : va > vb ? dir : a.key.localeCompare(b.key);
    });
  }, [players, showBots, sort]);

  const series = overview?.series ?? [];
  const bucketLabel = overview?.bucket === "day" ? "day" : "week";
  const t = (iso: string) => new Date(iso).getTime();

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link to="/app/admin" className="text-xs font-medium text-gc-accent hover:underline">
            ← Admin
          </Link>
          <h2 className="text-xl font-semibold text-gc-text">Player Statistics</h2>
          <p className="mt-0.5 text-sm text-gc-muted">Completed games: humans and bots, classic and custom maps.</p>
        </div>
      </div>

      <FilterBar value={filters} onChange={setFilters} />

      {error ? <div className="mb-4 rounded-xl border border-gc-danger/30 bg-gc-danger/10 p-4 text-sm text-gc-danger">{error}</div> : null}

      {overview ? (
        <div className={`space-y-4 transition-opacity ${loading ? "opacity-60" : ""}`}>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <StatTile
              label="Games completed"
              value={overview.games}
              sub={`${overview.human_only_games} human · ${overview.mixed_games} mixed · ${overview.bot_only_games} bot-only`}
            />
            <StatTile label="Human players" value={overview.human_players} />
            <StatTile label="Avg game length" value={fmtNum(overview.avg_turns, 0)} sub="turns" />
            <StatTile label="Avg duration" value={fmtDuration(overview.avg_duration_sec)} sub="first action → game over" />
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Panel title={`Games completed per ${bucketLabel}`}>
              <BarChart
                rows={series}
                label={(b) => fmtDate(b.start)}
                value={(b) => b.games}
                tooltip={(b) => (
                  <>
                    <div className="font-medium text-gc-text">
                      {bucketLabel === "week" ? `Week of ${fmtDate(b.start)}` : fmtDate(b.start)}
                    </div>
                    <div className="text-gc-muted">
                      {b.games} games · {b.active_humans} human players
                    </div>
                  </>
                )}
              />
            </Panel>
            <Panel title={`Active human players per ${bucketLabel}`}>
              <LineChart
                rows={series}
                x={(b) => t(b.start)}
                xFormat={fmtDate}
                series={[{ label: "Players", color: SERIES.blue, value: (b) => b.active_humans }]}
              />
            </Panel>
            <Panel title="Average game length" subtitle={`Turns per completed game, by ${bucketLabel}`}>
              <LineChart
                rows={series}
                x={(b) => t(b.start)}
                xFormat={fmtDate}
                series={[{ label: "Avg turns", color: SERIES.blue, value: (b) => b.avg_turns }]}
                yFormat={(v) => v.toFixed(0)}
                tooltipFormat={(v) => v.toFixed(1)}
              />
            </Panel>
            <Panel title="Humans vs bots" subtitle={`Share of mixed human/bot games won by a human, by ${bucketLabel}`}>
              <LineChart
                rows={series}
                x={(b) => t(b.start)}
                xFormat={fmtDate}
                series={[{ label: "Human win rate", color: SERIES.blue, value: (b) => b.human_win_pct_mixed }]}
                yMin={0}
                yMax={100}
                yFormat={(v) => `${v}%`}
                tooltipFormat={(v) => fmtPct(v, 1)}
                tooltipTitle={(b) => `${fmtDate(b.start)} · ${b.mixed_games} mixed games`}
              />
            </Panel>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Panel title="Games by player count">
              <BarChart
                rows={overview.by_player_count}
                label={(b) => `${b.player_count}p`}
                value={(b) => b.games}
                tooltip={(b) => (
                  <>
                    <div className="font-medium text-gc-text">{b.player_count}-player games</div>
                    <div className="text-gc-muted">
                      {b.games} ({fmtPct((100 * b.games) / Math.max(overview.games, 1), 0)})
                    </div>
                  </>
                )}
                height={180}
              />
            </Panel>

            {overview.bot_strategies.length > 0 ? (
              <Panel title="Bot strategies in mixed games" subtitle="Expected = win rate if every seat were equally strong">
                <div className="overflow-x-auto">
                  <table className="min-w-full text-sm">
                    <thead>
                      <tr className="text-left text-xs uppercase tracking-wide text-gc-muted">
                        <th className="py-1.5 pr-4 font-semibold">Strategy</th>
                        <th className="py-1.5 pr-4 text-right font-semibold">Games</th>
                        <th className="py-1.5 pr-4 text-right font-semibold">Wins</th>
                        <th className="py-1.5 pr-4 text-right font-semibold">Win %</th>
                        <th className="py-1.5 text-right font-semibold">Expected</th>
                      </tr>
                    </thead>
                    <tbody>
                      {overview.bot_strategies.map((s) => (
                        <tr key={s.strategy} className="border-t border-gc-border/60">
                          <td className="py-1.5 pr-4">
                            <Link
                              to="/app/admin/stats/$playerKey"
                              params={{ playerKey: `bot:${s.strategy}` }}
                              search={filters}
                              className="font-medium text-gc-text hover:text-gc-accent"
                            >
                              {s.strategy}
                            </Link>
                          </td>
                          <td className="py-1.5 pr-4 text-right tabular-nums text-gc-muted">{s.games}</td>
                          <td className="py-1.5 pr-4 text-right tabular-nums text-gc-muted">{s.wins}</td>
                          <td className="py-1.5 pr-4 text-right tabular-nums text-gc-text">{fmtPct(s.win_pct, 1)}</td>
                          <td className="py-1.5 text-right tabular-nums text-gc-muted">{fmtPct(s.expected_win_pct, 1)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </Panel>
            ) : (
              <Panel title="Bot strategies in mixed games">
                <p className="py-6 text-center text-sm text-gc-muted">No mixed human/bot games for these filters.</p>
              </Panel>
            )}
          </div>

          <Panel
            title="Players"
            subtitle="Bots aggregate by strategy. Select a player for trends over time."
            right={
              <label className="flex items-center gap-2 text-xs font-medium text-gc-muted">
                <input type="checkbox" checked={showBots} onChange={(e) => setShowBots(e.target.checked)} />
                Show bots
              </label>
            }
          >
            {visiblePlayers.length === 0 ? (
              <p className="py-6 text-center text-sm text-gc-muted">No completed games for these filters.</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full text-sm">
                  <thead>
                    <tr className="text-xs uppercase tracking-wide text-gc-muted">
                      {COLUMNS.map((c) => (
                        <th
                          key={c.key}
                          title={c.title}
                          className={`py-1.5 pr-4 font-semibold ${c.key === "name" ? "text-left" : "text-right"}`}
                        >
                          <button
                            type="button"
                            className="uppercase hover:text-gc-text"
                            onClick={() =>
                              setSort((s) => ({
                                key: c.key,
                                desc: s.key === c.key ? !s.desc : c.key !== "name" && c.key !== "avg_placement",
                              }))
                            }
                          >
                            {c.label}
                            {sort.key === c.key ? (sort.desc ? " ▾" : " ▴") : ""}
                          </button>
                        </th>
                      ))}
                      <th className="py-1.5 pr-4 text-right font-semibold">Streak</th>
                      <th className="py-1.5 text-left font-semibold">Last 10</th>
                    </tr>
                  </thead>
                  <tbody>
                    {visiblePlayers.map((p) => (
                      <tr key={p.key} className="border-t border-gc-border/60 hover:bg-gc-surface-2">
                        <td className="py-2 pr-4">
                          <Link
                            to="/app/admin/stats/$playerKey"
                            params={{ playerKey: p.key }}
                            search={filters}
                            className="font-medium text-gc-text hover:text-gc-accent"
                          >
                            {p.name}
                          </Link>
                          {p.is_bot ? (
                            <span className="ml-2 rounded bg-gc-surface-2 px-1.5 py-0.5 text-[10px] uppercase text-gc-muted">bot</span>
                          ) : null}
                        </td>
                        <td className="py-2 pr-4 text-right tabular-nums text-gc-muted">
                          {p.games}{" "}
                          <span className="text-xs">
                            ({p.wins}–{p.losses})
                          </span>
                        </td>
                        <td className="py-2 pr-4 text-right tabular-nums">
                          <div className="text-gc-text">{fmtPct(p.win_pct, 1)}</div>
                          <div className="text-[11px] text-gc-muted">vs {fmtPct(p.expected_win_pct, 0)}</div>
                        </td>
                        <td className="py-2 pr-4 text-right tabular-nums text-gc-text">
                          {p.wl_ratio != null ? fmtNum(p.wl_ratio) : p.wins > 0 ? "∞" : "0.00"}
                        </td>
                        <td className="py-2 pr-4 text-right tabular-nums text-gc-muted">{fmtNum(p.avg_placement, 1)}</td>
                        <td className="py-2 pr-4 text-right tabular-nums text-gc-muted">{fmtNum(p.kd_ratio)}</td>
                        <td className="py-2 pr-4 text-right tabular-nums text-gc-muted">{fmtNum(p.conquests_per_game, 1)}</td>
                        <td className="whitespace-nowrap py-2 pr-4 text-right text-gc-muted">{fmtDate(p.last_played)}</td>
                        <td className="py-2 pr-4 text-right tabular-nums text-gc-muted">{fmtStreak(p.current_streak)}</td>
                        <td className="py-2">
                          <RecentForm recent={p.recent} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Panel>
        </div>
      ) : loading ? (
        <div className="rounded-xl border border-gc-border bg-gc-surface p-6 text-sm text-gc-muted">Loading…</div>
      ) : null}
    </div>
  );
}
