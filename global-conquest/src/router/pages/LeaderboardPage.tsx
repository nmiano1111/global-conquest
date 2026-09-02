import { Fragment, useCallback, useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getLeaderboard, type LeaderboardEntry } from "../../api/leaderboard";
import { useAuth } from "../../auth";

const RANK_MEDALS = ["🥇", "🥈", "🥉"];

function PlayerCountBreakdown({ entry }: { entry: LeaderboardEntry }) {
  const records = entry.by_player_count ?? [];
  if (records.length === 0) {
    return <p className="px-4 py-3 text-xs text-gc-muted">No completed games to break down.</p>;
  }
  const maxGames = Math.max(...records.map((r) => r.wins + r.losses));
  return (
    <table className="min-w-full text-xs">
      <thead>
        <tr className="text-left text-gc-muted">
          <th className="px-4 py-1.5 font-semibold uppercase tracking-wide">Players</th>
          <th className="px-4 py-1.5 font-semibold uppercase tracking-wide">Record</th>
          <th className="w-full px-4 py-1.5 font-semibold uppercase tracking-wide">Wins / Losses</th>
        </tr>
      </thead>
      <tbody>
        {records.map((r) => {
          const games = r.wins + r.losses;
          const winPct = maxGames > 0 ? (r.wins / maxGames) * 100 : 0;
          const lossPct = maxGames > 0 ? (r.losses / maxGames) * 100 : 0;
          return (
            <tr key={r.player_count}>
              <td className="whitespace-nowrap px-4 py-1.5 text-gc-text">{r.player_count}-player</td>
              <td className="whitespace-nowrap px-4 py-1.5 text-gc-muted">
                <span className="text-gc-success">{r.wins}W</span>
                {" – "}
                <span className="text-gc-danger">{r.losses}L</span>
                <span className="text-gc-muted/60"> ({games})</span>
              </td>
              <td className="px-4 py-1.5">
                <div className="flex h-2 w-full max-w-xs overflow-hidden rounded-full bg-gc-surface-2">
                  <div className="h-full bg-gc-success" style={{ width: `${winPct}%` }} />
                  <div className="h-full bg-gc-danger" style={{ width: `${lossPct}%` }} />
                </div>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

export function LeaderboardPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [entries, setEntries] = useState<LeaderboardEntry[]>([]);
  const [expandedUserIds, setExpandedUserIds] = useState<Set<string>>(new Set());

  const load = useCallback(async (cancelled: boolean) => {
    setLoading(true);
    setError("");
    try {
      const data = await getLeaderboard();
      if (!cancelled) setEntries(data ?? []);
    } catch (err) {
      if (cancelled) return;
      const apiErr = err as ApiError;
      if (apiErr.status === 401) {
        auth.clearSession();
        await navigate({ to: "/login" });
        return;
      }
      setError(apiErr.message || "Failed to load leaderboard");
    } finally {
      if (!cancelled) setLoading(false);
    }
  }, [auth, navigate]);

  useEffect(() => {
    let cancelled = false;
    void load(cancelled);
    return () => { cancelled = true; };
  }, [load]);

  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="mb-4 text-xl font-semibold text-gc-text">Leaderboard</h2>

      {loading ? (
        <div className="rounded-xl border border-gc-border bg-gc-surface p-6">
          <p className="text-sm text-gc-muted">Loading…</p>
        </div>
      ) : null}

      {error ? (
        <div className="rounded-xl border border-gc-danger/30 bg-gc-danger/10 p-4">
          <p className="text-sm text-gc-danger">{error}</p>
        </div>
      ) : null}

      {!loading && !error && entries.length === 0 ? (
        <div className="rounded-xl border border-gc-border bg-gc-surface p-8 text-center">
          <p className="text-gc-muted">No completed games yet.</p>
          <p className="mt-1 text-xs text-gc-muted/60">Finish a game to appear on the leaderboard.</p>
        </div>
      ) : null}

      {!loading && !error && entries.length > 0 ? (
        <section className="overflow-x-auto rounded-xl border border-gc-border bg-gc-surface">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-gc-border bg-gc-surface-2">
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Rank</th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Player</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">Wins</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">Losses</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">Games</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">By player count</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e, i) => {
                const isTop3 = i < 3;
                const isExpanded = expandedUserIds.has(e.user_id);
                return (
                  <Fragment key={e.user_id}>
                    <tr className="border-b border-gc-border/60 last:border-0 transition-colors hover:bg-gc-surface-2">
                      <td className="px-4 py-3">
                        {isTop3 ? (
                          <span className="text-base leading-none">{RANK_MEDALS[i]}</span>
                        ) : (
                          <span className="font-mono text-gc-muted">{i + 1}</span>
                        )}
                      </td>
                      <td className="px-4 py-3 font-medium text-gc-text">{e.username}</td>
                      <td className="px-4 py-3 text-right font-semibold text-gc-success">{e.wins}</td>
                      <td className="px-4 py-3 text-right text-gc-muted">{e.losses}</td>
                      <td className="px-4 py-3 text-right text-gc-muted">{e.games_played}</td>
                      <td className="px-4 py-3 text-right">
                        <button
                          type="button"
                          onClick={() =>
                            setExpandedUserIds((prev) => {
                              const next = new Set(prev);
                              if (isExpanded) next.delete(e.user_id);
                              else next.add(e.user_id);
                              return next;
                            })
                          }
                          className="text-xs font-medium text-gc-accent hover:underline"
                        >
                          {isExpanded ? "Hide ▲" : "Show ▼"}
                        </button>
                      </td>
                    </tr>
                    {isExpanded ? (
                      <tr className="border-b border-gc-border/60 bg-gc-surface-2/50 last:border-0">
                        <td colSpan={6} className="p-0">
                          <PlayerCountBreakdown entry={e} />
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </section>
      ) : null}
    </div>
  );
}
