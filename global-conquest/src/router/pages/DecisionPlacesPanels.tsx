// Per-continent and per-territory attack choices for the admin player stats
// page (part of PlayerDecisionsSection).
import { useState } from "react";
import type { DecisionPlaceStat, PlayerDecisionStats } from "../../api/stats";
import { Panel } from "./statsShared";
import { fmtPct } from "./statsFormat";

const th = "whitespace-nowrap py-1.5 pr-4 text-xs font-semibold uppercase tracking-wide text-gc-muted";
const td = "whitespace-nowrap py-1.5 pr-4 tabular-nums";

/** "north_america" -> "North America"; custom-map names pass through. */
const prettyPlace = (name: string) => (/^[a-z0-9_]+$/.test(name) ? name.split("_").map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ") : name);

type SortKey = "attacks" | "take" | "preference" | "gap";
const sortValue: Record<SortKey, (p: DecisionPlaceStat) => number> = {
  attacks: (p) => p.attacks,
  take: (p) => p.take_pct,
  preference: (p) => p.preference ?? -1,
  // How much better than usual the odds were when they did attack here.
  gap: (p) => (p.avg_win_prob_chosen != null && p.avg_win_prob_available != null ? p.avg_win_prob_chosen - p.avg_win_prob_available : -999),
};

function Pref({ v }: { v: number | null }) {
  const cls = v == null ? "text-gc-muted" : v >= 1.25 ? "font-semibold text-gc-text" : v <= 0.8 ? "text-gc-muted" : "text-gc-text";
  return <span className={cls}>{v == null ? "—" : `${v.toFixed(2)}×`}</span>;
}

function Odds({ p }: { p: DecisionPlaceStat }) {
  if (p.avg_win_prob_chosen == null) return <span className="text-gc-muted">— / {fmtPct(p.avg_win_prob_available, 0)}</span>;
  return (
    <span>
      <span className="text-gc-text">{fmtPct(p.avg_win_prob_chosen, 0)}</span>
      <span className="text-gc-muted"> / {fmtPct(p.avg_win_prob_available, 0)}</span>
    </span>
  );
}

function PlaceTable({ rows, showContinent }: { rows: DecisionPlaceStat[]; showContinent: boolean }) {
  const [sort, setSort] = useState<SortKey>("attacks");
  const sorted = [...rows].sort((a, b) => sortValue[sort](b) - sortValue[sort](a));
  const head = (key: SortKey, label: string, title: string) => (
    <th className={`${th} text-right`} title={title}>
      <button type="button" onClick={() => setSort(key)} className={`uppercase ${sort === key ? "text-gc-text" : "hover:text-gc-text"}`}>
        {label}
        {sort === key ? " ↓" : ""}
      </button>
    </th>
  );
  return (
    <table className="min-w-full text-sm">
      <thead className="sticky top-0 bg-gc-surface">
        <tr className="text-left">
          <th className={th}>{showContinent ? "Territory" : "Continent"}</th>
          {head("attacks", "Attacked", "Attacks started against it")}
          {head("take", "When possible", "Share of their attack decisions where it was attackable that they went for it")}
          {head("preference", "Preference", "Share of their attacks ÷ share of the attackable options")}
          {head("gap", "Odds: taken / usual", "Average odds when they attacked it vs. whenever it was attackable")}
          <th className={`${th} text-right`}>Captured</th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((p) => (
          <tr key={p.name} className="border-t border-gc-border/60">
            <td className="py-1.5 pr-4">
              <div className="text-gc-text">{showContinent ? p.name : prettyPlace(p.name)}</div>
              {showContinent && p.continent ? <div className="text-xs text-gc-muted">{prettyPlace(p.continent)}</div> : null}
            </td>
            <td className={`${td} text-right text-gc-text`}>{p.attacks}</td>
            <td className={`${td} text-right text-gc-muted`}>
              {fmtPct(p.take_pct, 0)} <span className="text-xs">of {p.available}</span>
            </td>
            <td className={`${td} text-right`}>
              <Pref v={p.preference} />
            </td>
            <td className={`${td} text-right`}>
              <Odds p={p} />
            </td>
            <td className={`${td} text-right text-gc-muted`}>{fmtPct(p.capture_pct, 0)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function DecisionPlacesPanels({ data }: { data: PlayerDecisionStats }) {
  const [picked, setPicked] = useState<string | null>(null);
  if (data.place_attacks === 0 || data.maps.length === 0) return null;
  // Default (and fallback when filters drop the picked map): most-attacked map.
  const map = data.maps.find((m) => m.map_id === picked) ?? data.maps[0];
  const coverage =
    data.place_attacks < data.attacks ? ` Covers ${data.place_attacks} of ${data.attacks} attacks (older, since-deleted games lack it).` : "";
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-xs text-gc-muted">
        Targets on
        {data.maps.length > 1 ? (
          <select className="rounded-lg border border-gc-border bg-gc-surface px-2 py-1 text-xs text-gc-text" value={map.map_id} onChange={(e) => setPicked(e.target.value)}>
            {data.maps.map((m) => (
              <option key={m.map_id} value={m.map_id}>
                {m.map_name} ({m.attacks} attacks)
              </option>
            ))}
          </select>
        ) : (
          <span className="font-medium text-gc-text">{map.map_name}</span>
        )}
        <span>Shares and preferences are within this map.{coverage}</span>
      </div>
      <div className="grid gap-4 2xl:grid-cols-2">
        <Panel title="Targets by continent" subtitle="Where they attack vs. where they could have. Click a column to sort.">
          <div className="max-h-[420px] overflow-auto">
            <PlaceTable rows={map.continents} showContinent={false} />
          </div>
        </Panel>
        <Panel title="Targets by territory" subtitle="Odds shown as their average when attacking it / its average whenever it was attackable.">
          <div className="max-h-[420px] overflow-auto">
            <PlaceTable rows={map.territories} showContinent />
          </div>
        </Panel>
      </div>
    </div>
  );
}
