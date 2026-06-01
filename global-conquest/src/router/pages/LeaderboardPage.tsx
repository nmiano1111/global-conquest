import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getLeaderboard, type LeaderboardEntry } from "../../api/leaderboard";
import { useAuth } from "../../auth";

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
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <h2 className="text-2xl font-semibold tracking-tight text-slate-900">Leaderboard</h2>

      {loading ? <p className="mt-3 text-sm text-slate-600">Loading...</p> : null}
      {error ? <p className="mt-3 text-sm text-rose-700">{error}</p> : null}

      {!loading && !error && entries.length === 0 ? (
        <p className="mt-3 text-sm text-slate-500">No completed games yet.</p>
      ) : null}

      {!loading && !error && entries.length > 0 ? (
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm text-left text-slate-700">
            <thead>
              <tr className="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500">
                <th className="pb-2 pr-6">Rank</th>
                <th className="pb-2 pr-6">Player</th>
                <th className="pb-2 pr-6 text-right">Wins</th>
                <th className="pb-2 pr-6 text-right">Losses</th>
                <th className="pb-2 text-right">Games</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e, i) => (
                <tr key={e.user_id} className="border-b border-slate-100 last:border-0">
                  <td className="py-2.5 pr-6 font-mono text-slate-400">{i + 1}</td>
                  <td className="py-2.5 pr-6 font-medium text-slate-900">{e.username}</td>
                  <td className="py-2.5 pr-6 text-right text-emerald-600 font-medium">{e.wins}</td>
                  <td className="py-2.5 pr-6 text-right text-rose-500">{e.losses}</td>
                  <td className="py-2.5 text-right text-slate-500">{e.games_played}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
