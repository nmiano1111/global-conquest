import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getStatsPlayer, listStatsPlayers, type PlayerDetail, type PlayerSummary, type StatsFilters } from "../../api/stats";
import { useAuth } from "../../auth";
import { BarChart, FilterBar, LineChart, Panel, StatTile } from "./statsShared";
import {
  MAX_COMPARE,
  PLAYER_COLORS,
  SERIES,
  fmtDate,
  fmtDuration,
  fmtNum,
  fmtPct,
  fmtStreak,
  mergeLines,
  parseCompare,
  withCumulativeKD,
  type Line,
  type MergedRow,
  type Pt,
  type XMode,
} from "./statsFormat";

const th = "py-1.5 pr-4 text-xs font-semibold uppercase tracking-wide text-gc-muted";
const td = "py-1.5 pr-4 tabular-nums";

export function AdminPlayerStatsPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const { playerKey } = useParams({ from: "/app/admin/stats/$playerKey" });
  const search = useSearch({ from: "/app/admin/stats/$playerKey" });
  const filters = useMemo<StatsFilters>(() => {
    const f = { ...search };
    delete f.compare;
    return f;
  }, [search]);
  const compareKeys = useMemo(() => parseCompare(search.compare).filter((k) => k !== playerKey), [search.compare, playerKey]);
  const [detail, setDetail] = useState<PlayerDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [xMode, setXMode] = useState<XMode>("date");
  const [rateMode, setRateMode] = useState<"cumulative" | "rolling">("cumulative");
  // Compared players' details by key; null = no games under these filters.
  const [others, setOthers] = useState<Record<string, PlayerDetail | null>>({});
  const [othersLoading, setOthersLoading] = useState(false);
  const [allPlayers, setAllPlayers] = useState<PlayerSummary[]>([]);

  const load = useCallback(
    async (key: string, f: StatsFilters, isCancelled: () => boolean) => {
      setLoading(true);
      setError("");
      try {
        const d = await getStatsPlayer(key, f);
        if (!isCancelled()) setDetail(d);
      } catch (err) {
        if (isCancelled()) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        setDetail(null);
        setError(apiErr.status === 404 ? "No completed games for this player with these filters." : apiErr.message || "Failed to load stats");
      } finally {
        if (!isCancelled()) setLoading(false);
      }
    },
    [auth, navigate],
  );

  useEffect(() => {
    let cancelled = false;
    void load(playerKey, filters, () => cancelled);
    return () => {
      cancelled = true;
    };
  }, [load, playerKey, filters]);

  // Compared players: fetched with the same filters, independently of the main load.
  const compareSig = compareKeys.join(",");
  useEffect(() => {
    let cancelled = false;
    const keys = compareSig ? compareSig.split(",") : [];
    if (keys.length === 0) {
      setOthers({});
      return;
    }
    setOthersLoading(true);
    void Promise.all(
      keys.map((k) =>
        getStatsPlayer(k, filters).then(
          (d) => [k, d] as const,
          (err: ApiError) => {
            if (err.status === 404) return [k, null] as const;
            throw err;
          },
        ),
      ),
    )
      .then((pairs) => {
        if (!cancelled) setOthers(Object.fromEntries(pairs));
      })
      .catch(() => {
        if (!cancelled) setOthers({});
      })
      .finally(() => {
        if (!cancelled) setOthersLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [compareSig, filters]);

  // Candidates for the compare picker.
  useEffect(() => {
    let cancelled = false;
    listStatsPlayers(filters).then(
      (ps) => {
        if (!cancelled) setAllPlayers(ps);
      },
      () => {
        if (!cancelled) setAllPlayers([]);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [filters]);

  const go = (f: StatsFilters, compare: string[]) =>
    void navigate({
      to: "/app/admin/stats/$playerKey",
      params: { playerKey },
      search: { ...f, ...(compare.length > 0 ? { compare: compare.join(",") } : {}) },
      replace: true,
    });
  const setFilters = (f: StatsFilters) => go(f, compareKeys);
  const setCompare = (keys: string[]) => go(filters, keys);

  const s = detail?.summary;
  const series = detail?.series ?? [];
  const indexed: Pt[] = series.map((p, i) => ({ ...p, n: i + 1 }));
  type Row = Pt;
  const x = (r: Row) => (xMode === "date" ? new Date(r.completed_at).getTime() : r.n);
  const xFormat = xMode === "date" ? fmtDate : (v: number) => `#${Math.round(v)}`;
  const title = (r: Row) => `Game ${r.n} · ${fmtDate(r.completed_at)} · ${r.won ? "Won" : "Lost"}${r.placement ? ` (#${r.placement} of ${r.player_count})` : ""}`;
  // Compare mode: the page's player in slot 1, then compared players in URL order.
  const lines: Line[] = [];
  if (detail && s) {
    const pointsOf = (d: PlayerDetail, pts: Pt[]) =>
      withCumulativeKD(pts, new Map(d.games.map((g) => [g.game_id, { destroyed: g.armies_destroyed, lost: g.armies_lost }])));
    lines.push({ key: playerKey, name: s.name, color: PLAYER_COLORS[0], summary: s, points: pointsOf(detail, indexed) });
    compareKeys.forEach((k, i) => {
      const d = others[k];
      if (d) {
        lines.push({
          key: k,
          name: d.summary.name,
          color: PLAYER_COLORS[i + 1],
          summary: d.summary,
          points: pointsOf(
            d,
            d.series.map((p, j) => ({ ...p, n: j + 1 })),
          ),
        });
      }
    });
  }
  const comparing = lines.length > 1;
  const merged = comparing ? mergeLines(lines, xMode) : [];
  const mx = (r: MergedRow) => r.x;
  const mTitle = (r: MergedRow) => (xMode === "date" ? fmtDate(r.x) : `Game #${r.x}`);
  const lineSeries = (value: (p: Pt) => number | null) =>
    lines.map((l) => ({
      label: l.name,
      color: l.color,
      value: (r: MergedRow) => {
        const p = r.at[l.key];
        return p ? value(p) : null;
      },
    }));
  const playerName = (k: string) => others[k]?.summary.name ?? allPlayers.find((p) => p.key === k)?.name ?? k;
  const candidates = allPlayers.filter((p) => p.key !== playerKey && !compareKeys.includes(p.key));
  const pickerGroup = (label: string, bots: boolean) => (
    <optgroup label={label}>
      {candidates
        .filter((p) => p.is_bot === bots)
        .map((p) => (
          <option key={p.key} value={p.key}>
            {p.name} ({p.games})
          </option>
        ))}
    </optgroup>
  );
  const compareBar = (
    <div className="flex flex-wrap items-center gap-2 text-xs text-gc-muted">
      <span className="inline-flex items-center gap-1.5 rounded-full border border-gc-border px-2.5 py-1 text-gc-text">
        <span className="inline-block h-2 w-2 rounded-full" style={{ background: PLAYER_COLORS[0] }} />
        {s?.name ?? playerKey}
      </span>
      {compareKeys.map((k, i) => (
        <span key={k} className="inline-flex items-center gap-1.5 rounded-full border border-gc-border py-1 pl-2.5 pr-1.5 text-gc-text">
          <span className="inline-block h-2 w-2 rounded-full" style={{ background: PLAYER_COLORS[i + 1] }} />
          {playerName(k)}
          {others[k] === null ? <span className="text-gc-muted">(no games with these filters)</span> : null}
          <button
            type="button"
            aria-label={`Stop comparing ${playerName(k)}`}
            className="rounded px-1 text-gc-muted hover:text-gc-text"
            onClick={() => setCompare(compareKeys.filter((c) => c !== k))}
          >
            ×
          </button>
        </span>
      ))}
      {compareKeys.length < MAX_COMPARE ? (
        <select
          className="rounded-lg border border-gc-border bg-gc-surface px-2 py-1 text-xs text-gc-text"
          value=""
          onChange={(e) => e.target.value && setCompare([...compareKeys, e.target.value])}
        >
          <option value="">+ Compare with…</option>
          {pickerGroup("Players", false)}
          {pickerGroup("Bot strategies", true)}
        </select>
      ) : (
        <span>Up to {MAX_COMPARE} comparisons</span>
      )}
      {othersLoading ? <span>Loading…</span> : null}
    </div>
  );
  const xToggle = (
    <div className="inline-flex overflow-hidden rounded-lg border border-gc-border text-xs">
      {(["date", "game"] as XMode[]).map((m) => (
        <button
          key={m}
          type="button"
          onClick={() => setXMode(m)}
          className={`px-2.5 py-1 ${xMode === m ? "bg-gc-surface-2 text-gc-text" : "text-gc-muted hover:text-gc-text"}`}
        >
          {m === "date" ? "By date" : "By game #"}
        </button>
      ))}
    </div>
  );

  return (
    <div>
      <div className="mb-4">
        <Link to="/app/admin/stats" search={filters} className="text-xs font-medium text-gc-accent hover:underline">
          ← Player Statistics
        </Link>
        <h2 className="flex items-center gap-2 text-xl font-semibold text-gc-text">
          {s?.name ?? playerKey}
          {s?.is_bot ? <span className="rounded bg-gc-surface-2 px-1.5 py-0.5 text-xs uppercase text-gc-muted">bot strategy</span> : null}
        </h2>
        {s ? (
          <p className="mt-0.5 text-sm text-gc-muted">
            {fmtDate(s.first_played)} – {fmtDate(s.last_played)}
          </p>
        ) : null}
      </div>

      <FilterBar value={filters} onChange={setFilters} />

      {error ? <div className="mb-4 rounded-xl border border-gc-border bg-gc-surface p-6 text-sm text-gc-muted">{error}</div> : null}
      {!detail && loading ? <div className="rounded-xl border border-gc-border bg-gc-surface p-6 text-sm text-gc-muted">Loading…</div> : null}

      {detail && s ? (
        <div className={`space-y-4 transition-opacity ${loading ? "opacity-60" : ""}`}>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">
            <StatTile label="Record" value={`${s.wins}–${s.losses}`} sub={`${s.games} games`} />
            <StatTile label="Win rate" value={fmtPct(s.win_pct, 1)} sub={`even odds: ${fmtPct(s.expected_win_pct, 1)}`} />
            <StatTile label="W/L ratio" value={s.wl_ratio != null ? fmtNum(s.wl_ratio) : s.wins > 0 ? "∞" : "0.00"} />
            <StatTile label="Avg placement" value={fmtNum(s.avg_placement, 2)} sub="1 = won" />
            <StatTile label="Streak" value={fmtStreak(s.current_streak)} sub={`best: ${s.longest_win_streak} wins`} />
            <StatTile label="K/D" value={fmtNum(s.kd_ratio)} sub={`${s.armies_destroyed} destroyed / ${s.armies_lost} lost`} />
            <StatTile label="Conquests" value={fmtNum(s.conquests_per_game, 1)} sub="territories per game" />
            <StatTile label="Eliminations" value={s.eliminations} sub={`${s.card_trades} card trades`} />
          </div>
          {s.history_games < s.games ? (
            <p className="text-xs text-gc-muted">
              Combat stats (K/D, conquests, eliminations) cover {s.history_games} of {s.games} games. Older games lack the event history.
            </p>
          ) : null}

          <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-gc-muted">Trends</h3>
            <div className="flex items-center gap-2 text-xs text-gc-muted">
              X axis {xToggle}
            </div>
          </div>
          {compareBar}

          {comparing ? (
            <>
              <Panel title="Comparison" subtitle="Same filters for every player. Even odds = average of 1 ÷ players across their games.">
                <div className="overflow-x-auto">
                  <table className="min-w-full text-sm">
                    <thead>
                      <tr className="text-left">
                        <th className={th}>Player</th>
                        <th className={`${th} text-right`}>Games</th>
                        <th className={`${th} text-right`}>Record</th>
                        <th className={`${th} text-right`}>Win %</th>
                        <th className={`${th} text-right`}>Even odds</th>
                        <th className={`${th} text-right`}>W/L</th>
                        <th className={`${th} text-right`}>Avg place</th>
                        <th className={`${th} text-right`}>K/D</th>
                        <th className={`${th} text-right`}>Conq/game</th>
                      </tr>
                    </thead>
                    <tbody>
                      {lines.map((l) => (
                        <tr key={l.key} className="border-t border-gc-border/60">
                          <td className="py-1.5 pr-4 text-gc-text">
                            <span className="inline-flex items-center gap-1.5">
                              <span className="inline-block h-2 w-2 rounded-full" style={{ background: l.color }} />
                              {l.name}
                            </span>
                          </td>
                          <td className={`${td} text-right text-gc-muted`}>{l.summary.games}</td>
                          <td className={`${td} text-right text-gc-muted`}>
                            {l.summary.wins}–{l.summary.losses}
                          </td>
                          <td className={`${td} text-right text-gc-text`}>{fmtPct(l.summary.win_pct, 1)}</td>
                          <td className={`${td} text-right text-gc-muted`}>{fmtPct(l.summary.expected_win_pct, 1)}</td>
                          <td className={`${td} text-right text-gc-muted`}>{fmtNum(l.summary.wl_ratio)}</td>
                          <td className={`${td} text-right text-gc-muted`}>{fmtNum(l.summary.avg_placement)}</td>
                          <td className={`${td} text-right text-gc-muted`}>{fmtNum(l.summary.kd_ratio)}</td>
                          <td className={`${td} text-right text-gc-muted`}>{fmtNum(l.summary.conquests_per_game, 1)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </Panel>
              <div className={`grid gap-4 transition-opacity lg:grid-cols-2 ${othersLoading ? "opacity-60" : ""}`}>
                <Panel
                  title="Win rate over time"
                  subtitle={rateMode === "cumulative" ? "Cumulative, after each game" : "Over each player's last 10 games"}
                  right={
                    <div className="inline-flex overflow-hidden rounded-lg border border-gc-border text-xs">
                      {(["cumulative", "rolling"] as const).map((m) => (
                        <button
                          key={m}
                          type="button"
                          onClick={() => setRateMode(m)}
                          className={`px-2.5 py-1 ${rateMode === m ? "bg-gc-surface-2 text-gc-text" : "text-gc-muted hover:text-gc-text"}`}
                        >
                          {m === "cumulative" ? "Cumulative" : "Last 10"}
                        </button>
                      ))}
                    </div>
                  }
                >
                  <LineChart
                    rows={merged}
                    x={mx}
                    xFormat={xFormat}
                    series={lineSeries((p) => (rateMode === "cumulative" ? p.win_pct : p.rolling_win_pct))}
                    yMin={0}
                    yMax={100}
                    yFormat={(v) => `${v}%`}
                    tooltipFormat={(v) => fmtPct(v, 1)}
                    tooltipTitle={mTitle}
                  />
                </Panel>
                <Panel title="W/L ratio over time" subtitle="Wins ÷ losses after each game (undefined until the first loss)">
                  <LineChart
                    rows={merged}
                    x={mx}
                    xFormat={xFormat}
                    series={lineSeries((p) => p.wl_ratio)}
                    refLines={[{ value: 1, label: "1.0" }]}
                    yFormat={(v) => v.toFixed(1)}
                    tooltipFormat={(v) => v.toFixed(2)}
                    tooltipTitle={mTitle}
                  />
                </Panel>
                <Panel title="Cumulative wins">
                  <LineChart
                    rows={merged}
                    x={mx}
                    xFormat={xFormat}
                    series={lineSeries((p) => p.cum_wins)}
                    yFormat={(v) => v.toFixed(0)}
                    tooltipTitle={mTitle}
                  />
                </Panel>
                <Panel title="K/D over time" subtitle="Cumulative enemy armies destroyed ÷ own armies lost (games with combat history)">
                  <LineChart
                    rows={merged}
                    x={mx}
                    xFormat={xFormat}
                    series={lineSeries((p) => p.cum_kd ?? null)}
                    refLines={[{ value: 1, label: "1.0" }]}
                    yFormat={(v) => v.toFixed(1)}
                    tooltipFormat={(v) => v.toFixed(2)}
                    tooltipTitle={mTitle}
                  />
                </Panel>
              </div>
            </>
          ) : (
            <div className="grid gap-4 lg:grid-cols-2">
            <Panel title="Win rate over time" subtitle="Cumulative and over the last 10 games">
              <LineChart
                rows={indexed}
                x={x}
                xFormat={xFormat}
                series={[
                  { label: "Cumulative", color: SERIES.blue, value: (r) => r.win_pct },
                  { label: "Last 10 games", color: SERIES.orange, value: (r) => r.rolling_win_pct },
                ]}
                refLines={[{ value: s.expected_win_pct, label: "Even odds" }]}
                yMin={0}
                yMax={100}
                yFormat={(v) => `${v}%`}
                tooltipFormat={(v) => fmtPct(v, 1)}
                tooltipTitle={title}
              />
            </Panel>
            <Panel title="W/L ratio over time" subtitle="Wins ÷ losses after each game (undefined until the first loss)">
              <LineChart
                rows={indexed}
                x={x}
                xFormat={xFormat}
                series={[{ label: "W/L ratio", color: SERIES.blue, value: (r) => r.wl_ratio }]}
                refLines={[{ value: 1, label: "1.0" }]}
                yFormat={(v) => v.toFixed(1)}
                tooltipFormat={(v) => v.toFixed(2)}
                tooltipTitle={title}
              />
            </Panel>
            <Panel title="Cumulative record">
              <LineChart
                rows={indexed}
                x={x}
                xFormat={xFormat}
                series={[
                  { label: "Wins", color: SERIES.blue, value: (r) => r.cum_wins },
                  { label: "Losses", color: SERIES.orange, value: (r) => r.cum_losses },
                ]}
                yFormat={(v) => v.toFixed(0)}
                tooltipTitle={title}
              />
            </Panel>
            <Panel title="K/D per game" subtitle="Enemy armies destroyed ÷ own armies lost in each game">
              <LineChart
                rows={indexed}
                x={x}
                xFormat={xFormat}
                series={[{ label: "K/D", color: SERIES.blue, value: (r) => r.game_kd }]}
                refLines={[{ value: 1, label: "1.0" }]}
                yFormat={(v) => v.toFixed(1)}
                tooltipFormat={(v) => v.toFixed(2)}
                tooltipTitle={title}
                showDots
              />
            </Panel>
          </div>
          )}

          <div className="grid gap-4 lg:grid-cols-2">
            <Panel title="Finishing positions">
              <BarChart
                rows={detail.placements}
                label={(p) => (p.placement === 1 ? "1st" : p.placement === 2 ? "2nd" : p.placement === 3 ? "3rd" : `${p.placement}th`)}
                value={(p) => p.count}
                tooltip={(p) => (
                  <>
                    <div className="font-medium text-gc-text">Finished #{p.placement}</div>
                    <div className="text-gc-muted">
                      {p.count} games ({fmtPct((100 * p.count) / s.games, 0)})
                    </div>
                  </>
                )}
                height={180}
              />
            </Panel>
            <Panel title="By player count" subtitle="Even odds = 1 ÷ players">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="text-left">
                    <th className={th}>Players</th>
                    <th className={`${th} text-right`}>Record</th>
                    <th className={`${th} text-right`}>Win %</th>
                    <th className={`${th} text-right`}>Even odds</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.by_player_count.map((b) => (
                    <tr key={b.player_count} className="border-t border-gc-border/60">
                      <td className={`${td} text-gc-text`}>{b.player_count}-player</td>
                      <td className={`${td} text-right text-gc-muted`}>
                        {b.wins}–{b.losses}
                      </td>
                      <td className={`${td} text-right text-gc-text`}>{fmtPct(b.win_pct, 1)}</td>
                      <td className={`${td} text-right text-gc-muted`}>{fmtPct(b.expected_win_pct, 1)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Panel>
          </div>

          <Panel title="Head-to-head" subtitle="Games shared with each opponent. Finished ahead only counts games where both placements are known.">
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="text-left">
                    <th className={th}>Opponent</th>
                    <th className={`${th} text-right`}>Games</th>
                    <th className={`${th} text-right`}>{s.name} won</th>
                    <th className={`${th} text-right`}>They won</th>
                    <th className={`${th} text-right`}>Finished ahead</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.opponents.map((o) => (
                    <tr key={o.key} className="border-t border-gc-border/60">
                      <td className="py-1.5 pr-4">
                        <Link
                          to="/app/admin/stats/$playerKey"
                          params={{ playerKey: o.key }}
                          search={filters}
                          className="font-medium text-gc-text hover:text-gc-accent"
                        >
                          {o.name}
                        </Link>
                        {o.is_bot ? <span className="ml-2 text-[10px] uppercase text-gc-muted">bot</span> : null}
                      </td>
                      <td className={`${td} text-right text-gc-muted`}>{o.games}</td>
                      <td className={`${td} text-right text-gc-text`}>{o.wins}</td>
                      <td className={`${td} text-right text-gc-muted`}>{o.opponent_wins}</td>
                      <td className={`${td} text-right text-gc-muted`}>
                        {o.placement_games > 0 ? `${o.finished_ahead} / ${o.placement_games}` : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Panel>

          <Panel title="Game history" subtitle="Newest first">
            <div className="max-h-[480px] overflow-auto">
              <table className="min-w-full text-sm">
                <thead className="sticky top-0 bg-gc-surface">
                  <tr className="text-left">
                    <th className={th}>Completed</th>
                    <th className={th}>Result</th>
                    <th className={th}>Winner</th>
                    <th className={th}>Opponents</th>
                    <th className={th}>Map</th>
                    <th className={`${th} text-right`}>Turns</th>
                    <th className={`${th} text-right`}>Duration</th>
                    <th className={`${th} text-right`}>Conq</th>
                    <th className={`${th} text-right`}>K/D</th>
                    <th className={th}></th>
                  </tr>
                </thead>
                <tbody>
                  {detail.games.map((g) => (
                    <tr key={g.game_id} className="border-t border-gc-border/60">
                      <td className={`${td} whitespace-nowrap text-gc-muted`}>{fmtDate(g.completed_at)}</td>
                      <td className={`${td} whitespace-nowrap`}>
                        <span className={g.won ? "font-semibold text-gc-success" : "text-gc-muted"}>{g.won ? "Won" : "Lost"}</span>
                        {g.placement ? <span className="text-gc-muted"> · #{g.placement}/{g.player_count}</span> : null}
                      </td>
                      <td className={`${td} text-gc-muted`}>{g.won ? "—" : g.winner_name}</td>
                      <td className="max-w-xs truncate py-1.5 pr-4 text-gc-muted" title={g.opponents.join(", ")}>
                        {g.opponents.join(", ")}
                      </td>
                      <td className={`${td} text-gc-muted`}>{g.map_name || "Classic"}</td>
                      <td className={`${td} text-right text-gc-muted`}>{g.turn_count}</td>
                      <td className={`${td} text-right text-gc-muted`}>{fmtDuration(g.duration_seconds)}</td>
                      <td className={`${td} text-right text-gc-muted`}>{g.conquests ?? "—"}</td>
                      <td className={`${td} text-right text-gc-muted`}>
                        {g.armies_destroyed != null && g.armies_lost ? fmtNum(g.armies_destroyed / g.armies_lost) : "—"}
                      </td>
                      <td className="py-1.5 text-right">
                        <Link to="/app/game/$gameID/replay" params={{ gameID: g.game_id }} className="text-xs text-gc-accent hover:underline">
                          Replay
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      ) : null}
    </div>
  );
}
