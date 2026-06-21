import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getLeaderboard, type LeaderboardEntry } from "../../api/leaderboard";
import { useAuth } from "../../auth";

const RANK_MEDALS = ["🥇", "🥈", "🥉"];

export function LeaderboardPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [entries, setEntries] = useState<LeaderboardEntry[]>([]);

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
        <section className="rounded-xl border border-gc-border bg-gc-surface overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gc-border bg-gc-surface-2">
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Rank</th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Player</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">Wins</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">Losses</th>
                <th className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide text-gc-muted">Games</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e, i) => {
                const isTop3 = i < 3;
                return (
                  <tr
                    key={e.user_id}
                    className="border-b border-gc-border/60 last:border-0 transition-colors hover:bg-gc-surface-2"
                  >
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
                  </tr>
                );
              })}
            </tbody>
          </table>
        </section>
      ) : null}
    </div>
  );
}
