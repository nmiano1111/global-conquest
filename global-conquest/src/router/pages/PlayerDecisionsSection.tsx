// Attack-decision analytics for the admin player stats page: how a player
// picks attacks compared with the attacks available at the time. Data comes
// from attack_decisions, extracted in the background from each completed
// game's move history (backend service.DecisionExtractor).
import { useEffect, useState } from "react";
import type { ApiError } from "../../api/client";
import { getStatsPlayerDecisions, type PlayerDecisionStats, type StatsFilters } from "../../api/stats";
import { BarChart, LineChart, Panel, StatTile } from "./statsShared";
import { SERIES, fmtDate, fmtNum, fmtPct, type XMode } from "./statsFormat";

const th = "py-1.5 pr-4 text-xs font-semibold uppercase tracking-wide text-gc-muted";
const td = "py-1.5 pr-4 tabular-nums";

const fmtSigned = (v: number | null | undefined) => (v == null ? "—" : `${v >= 0 ? "+" : "−"}${Math.abs(v).toFixed(1)}%`);
const fmtPref = (v: number | null | undefined) => (v == null ? "—" : `${v.toFixed(2)}×`);
/** Preference cell: highlights clear over/under-selection (≥1.25× / ≤0.8×). */
function Pref({ v }: { v: number | null }) {
  const cls = v == null ? "text-gc-muted" : v >= 1.25 ? "font-semibold text-gc-text" : v <= 0.8 ? "text-gc-muted" : "text-gc-text";
  return <span className={cls}>{fmtPref(v)}</span>;
}

export function PlayerDecisionsSection({
  playerKey,
  playerName,
  filters,
  xMode,
}: {
  playerKey: string;
  playerName: string;
  filters: StatsFilters;
  xMode: XMode;
}) {
  // Results are tagged with the request they answer, so "loading" is just
  // "the latest result is for an older request" (no setState in the effect body).
  const reqKey = JSON.stringify([playerKey, filters]);
  const [result, setResult] = useState<{ key: string; data?: PlayerDecisionStats; error?: string } | null>(null);
  const loading = result?.key !== reqKey;
  const data = result?.data ?? null;
  const error = !loading ? (result?.error ?? "") : "";

  useEffect(() => {
    let cancelled = false;
    getStatsPlayerDecisions(playerKey, filters).then(
      (d) => {
        if (!cancelled) setResult({ key: reqKey, data: d });
      },
      (err: ApiError) => {
        if (!cancelled) setResult({ key: reqKey, error: err.message || "Failed to load decision stats" });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [playerKey, filters, reqKey]);

  const heading = (
    <div className="pt-2">
      <h3 className="text-sm font-semibold uppercase tracking-wide text-gc-muted">Attack decisions · {playerName}</h3>
      <p className="mt-0.5 text-xs text-gc-muted">
        Each attack compared with every attack that was legal at that moment.
        {data && data.games > 0 ? ` From ${data.games} games with move-by-move history.` : ""}
      </p>
    </div>
  );

  if (error) {
    return (
      <>
        {heading}
        <div className="rounded-xl border border-gc-border bg-gc-surface p-6 text-sm text-gc-muted">{error}</div>
      </>
    );
  }
  if (!data) {
    return (
      <>
        {heading}
        <div className="rounded-xl border border-gc-border bg-gc-surface p-6 text-sm text-gc-muted">Loading…</div>
      </>
    );
  }
  if (data.games === 0) {
    return (
      <>
        {heading}
        <div className="rounded-xl border border-gc-border bg-gc-surface p-6 text-sm text-gc-muted">
          No move-by-move history for these games. Finished games are processed within a few minutes; games without a saved move log can't
          be analyzed.
        </div>
      </>
    );
  }

  const series = data.series.map((p, i) => ({ ...p, n: i + 1 }));
  type Row = (typeof series)[number];
  const x = (r: Row) => (xMode === "date" ? new Date(r.completed_at).getTime() : r.n);
  const xFormat = xMode === "date" ? fmtDate : (v: number) => `#${Math.round(v)}`;
  const title = (r: Row) => `${fmtDate(r.completed_at)} · ${r.attacks} attacks started`;

  return (
    <>
      {heading}
      <div className={`space-y-4 transition-opacity ${loading ? "opacity-60" : ""}`}>
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <StatTile
            label="Attacks started"
            value={data.attacks}
            sub={`${data.rolls} rolls · ${fmtNum(data.avg_rolls_per_attack, 1)} per attack`}
          />
          <StatTile
            label="Avg odds when attacking"
            value={fmtPct(data.avg_win_prob, 0)}
            sub={`${fmtPct(data.abandoned_pct, 0)} called off before capturing`}
          />
          <StatTile
            label="Stopped with good odds left"
            value={fmtPct(data.stops_with_good_odds_pct, 0)}
            sub={`of ${data.stops} stops had a ≥75% attack available`}
          />
          <StatTile
            label="Dice luck"
            value={fmtSigned(data.kill_luck_pct)}
            sub={`kills vs expected · losses ${fmtSigned(data.loss_luck_pct)}`}
          />
        </div>

        <div className="grid gap-4 lg:grid-cols-2">
          <Panel
            title="Odds of attacks started"
            subtitle="Odds of taking the target if fought to the end. Preference = share chosen ÷ share available; above 1× means picked more than chance."
          >
            <BarChart
              rows={data.odds_buckets}
              label={(b) => b.label}
              value={(b) => b.chosen_pct}
              format={(v) => `${v}%`}
              height={170}
              tooltip={(b) => (
                <>
                  <div className="font-medium text-gc-text">{b.label} odds</div>
                  <div className="text-gc-muted">
                    {b.attacks} attacks · {fmtPct(b.chosen_pct, 1)} of attacks
                  </div>
                  <div className="text-gc-muted">
                    {fmtPct(b.available_pct, 1)} of options · {fmtPref(b.preference)}
                  </div>
                </>
              )}
            />
            <table className="mt-3 min-w-full text-sm">
              <thead>
                <tr className="text-left">
                  <th className={th}>Odds</th>
                  <th className={`${th} text-right`}>Chosen</th>
                  <th className={`${th} text-right`}>Available</th>
                  <th className={`${th} text-right`}>Preference</th>
                  <th className={`${th} text-right`} title="Captured, vs. average odds of capturing (stopping early counts as not capturing)">
                    Captured
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.odds_buckets.map((b) => (
                  <tr key={b.label} className="border-t border-gc-border/60">
                    <td className={`${td} text-gc-text`}>{b.label}</td>
                    <td className={`${td} text-right text-gc-text`}>{fmtPct(b.chosen_pct, 0)}</td>
                    <td className={`${td} text-right text-gc-muted`}>{fmtPct(b.available_pct, 0)}</td>
                    <td className={`${td} text-right`}>
                      <Pref v={b.preference} />
                    </td>
                    <td className={`${td} text-right text-gc-muted`}>
                      {b.capture_pct == null ? "—" : `${fmtPct(b.capture_pct, 0)} (exp. ${fmtPct(b.expected_capture_pct, 0)})`}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Panel>

          <div className="space-y-4">
            <Panel title="Target choices" subtitle="How often attacks they started had each property, vs. the attacks available.">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="text-left">
                    <th className={th}>Target</th>
                    <th className={`${th} text-right`}>Chosen</th>
                    <th className={`${th} text-right`}>Available</th>
                    <th className={`${th} text-right`}>Preference</th>
                  </tr>
                </thead>
                <tbody>
                  {data.features.map((f) => (
                    <tr key={f.key} className="border-t border-gc-border/60">
                      <td className={`${td} text-gc-text`}>{f.label}</td>
                      <td className={`${td} text-right text-gc-text`}>{fmtPct(f.chosen_pct, 0)}</td>
                      <td className={`${td} text-right text-gc-muted`}>{fmtPct(f.available_pct, 0)}</td>
                      <td className={`${td} text-right`}>
                        <Pref v={f.preference} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <p className="mt-2 text-xs text-gc-muted">
                Strongest/weakest is by territories held. With one opponent left, every attack counts as both.
              </p>
            </Panel>
            <Panel title="When they stop attacking" subtitle="Best odds still available when they ended their attack phase">
              <BarChart
                rows={data.stop_buckets}
                label={(b) => b.label.replace("Best ", "").replace("No attack possible", "None")}
                value={(b) => b.pct}
                format={(v) => `${v}%`}
                height={150}
                tooltip={(b) => (
                  <>
                    <div className="font-medium text-gc-text">{b.label}</div>
                    <div className="text-gc-muted">
                      {b.stops} stops ({fmtPct(b.pct, 1)})
                    </div>
                  </>
                )}
              />
            </Panel>
          </div>
        </div>

        <div className="grid gap-4 lg:grid-cols-2">
          <Panel title="Attack odds per game" subtitle="Average odds of the attacks started in each game">
            <LineChart
              rows={series}
              x={x}
              xFormat={xFormat}
              series={[{ label: "Avg odds", color: SERIES.blue, value: (r) => r.avg_win_prob }]}
              yMin={0}
              yMax={100}
              yFormat={(v) => `${v}%`}
              tooltipFormat={(v) => fmtPct(v, 1)}
              tooltipTitle={title}
            />
          </Panel>
          <Panel title="Dice luck over time" subtitle="Cumulative enemy armies destroyed vs. what their dice should produce on average">
            <LineChart
              rows={series}
              x={x}
              xFormat={xFormat}
              series={[{ label: "Kill luck", color: SERIES.blue, value: (r) => r.cum_kill_luck_pct }]}
              refLines={[{ value: 0, label: "As expected" }]}
              yFormat={(v) => `${v > 0 ? "+" : ""}${v}%`}
              tooltipFormat={fmtSigned}
              tooltipTitle={title}
            />
          </Panel>
        </div>
      </div>
    </>
  );
}
