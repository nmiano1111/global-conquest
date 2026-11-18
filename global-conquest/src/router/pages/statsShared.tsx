// Shared components for the admin stats pages (AdminStatsPage,
// AdminPlayerStatsPage): filter bar, stat tiles, and small dependency-free
// SVG charts. Formatters and series colors live in statsFormat.ts.
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { StatsFilters } from "../../api/stats";
import { SERIES } from "./statsFormat";
import { inputClass } from "./styles";

const selectClass = `${inputClass} w-auto py-1.5`;

export function FilterBar({ value, onChange }: { value: StatsFilters; onChange: (f: StatsFilters) => void }) {
  const set = (patch: Partial<StatsFilters>) => onChange({ ...value, ...patch });
  const label = "flex flex-col gap-1 text-xs font-medium text-gc-muted";
  return (
    <div className="mb-5 flex flex-wrap items-end gap-3 rounded-xl border border-gc-border bg-gc-surface p-3">
      <label className={label}>
        Games
        <select
          className={selectClass}
          value={value.composition ?? "all"}
          onChange={(e) => set({ composition: e.target.value as StatsFilters["composition"] })}
        >
          <option value="all">All games</option>
          <option value="human_only">Humans only</option>
          <option value="mixed">Humans + bots</option>
          <option value="bot_only">Bots only</option>
        </select>
      </label>
      <label className={label}>
        Map
        <select className={selectClass} value={value.map ?? "all"} onChange={(e) => set({ map: e.target.value as StatsFilters["map"] })}>
          <option value="all">All maps</option>
          <option value="classic">Classic</option>
          <option value="custom">Custom</option>
        </select>
      </label>
      <label className={label}>
        Players
        <select
          className={selectClass}
          value={value.player_count ?? 0}
          onChange={(e) => set({ player_count: Number(e.target.value) || undefined })}
        >
          <option value={0}>Any</option>
          {[2, 3, 4, 5, 6].map((n) => (
            <option key={n} value={n}>{n} players</option>
          ))}
        </select>
      </label>
      <label className={label}>
        From
        <input type="date" className={selectClass} value={value.from ?? ""} onChange={(e) => set({ from: e.target.value || undefined })} />
      </label>
      <label className={label}>
        To
        <input type="date" className={selectClass} value={value.to ?? ""} onChange={(e) => set({ to: e.target.value || undefined })} />
      </label>
      <label className="flex items-center gap-2 pb-2 text-xs font-medium text-gc-muted">
        <input
          type="checkbox"
          checked={!!value.include_sandbox}
          onChange={(e) => set({ include_sandbox: e.target.checked || undefined })}
        />
        Include sandbox games
      </label>
      {Object.keys(value).length > 0 ? (
        <button type="button" className="pb-2 text-xs font-medium text-gc-accent hover:underline" onClick={() => onChange({})}>
          Reset
        </button>
      ) : null}
    </div>
  );
}

// ---------- layout bits ----------

export function StatTile({ label, value, sub }: { label: string; value: ReactNode; sub?: ReactNode }) {
  return (
    <div className="rounded-xl border border-gc-border bg-gc-surface px-4 py-3">
      <div className="text-xs font-medium uppercase tracking-wide text-gc-muted">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums text-gc-text">{value}</div>
      {sub ? <div className="mt-0.5 text-xs text-gc-muted">{sub}</div> : null}
    </div>
  );
}

export function Panel({ title, subtitle, children, right }: { title: string; subtitle?: string; children: ReactNode; right?: ReactNode }) {
  return (
    <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold text-gc-text">{title}</h3>
          {subtitle ? <p className="mt-0.5 text-xs text-gc-muted">{subtitle}</p> : null}
        </div>
        {right}
      </div>
      {children}
    </section>
  );
}

export function RecentForm({ recent }: { recent: boolean[] }) {
  return (
    <span className="inline-flex gap-0.5" aria-label={recent.map((w) => (w ? "W" : "L")).join(" ")}>
      {recent.map((w, i) => (
        <span
          key={i}
          title={w ? "Win" : "Loss"}
          className={`inline-block h-3 w-1.5 rounded-sm ${w ? "bg-gc-success" : "bg-gc-danger/50"}`}
        />
      ))}
    </span>
  );
}

// ---------- charts ----------

function useWidth<T extends HTMLElement>(): [React.RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(600);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => setWidth(Math.max(240, entry.contentRect.width)));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, width];
}

/** ~count "nice" ticks spanning [min, max]. */
function niceTicks(min: number, max: number, count = 4): number[] {
  if (max <= min) max = min + 1;
  const raw = (max - min) / count;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? raw;
  const out: number[] = [];
  // The last tick rounds UP past max so the tallest mark stays inside the axis.
  const end = Math.ceil(max / step - 1e-9) * step;
  for (let v = Math.ceil(min / step) * step; v <= end + 1e-9; v += step) out.push(Number(v.toFixed(10)));
  return out;
}

const M = { top: 12, right: 16, bottom: 26, left: 44 };
const AXIS = "#687a91"; // gc-muted
const GRID = "#263348"; // gc-border

export type LineSeries<R> = {
  label: string;
  color: string;
  value: (r: R) => number | null;
};

export type RefLine = { value: number; label: string };

/**
 * Multi-series line chart over rows sorted by x, with a crosshair tooltip.
 * Up to 3 series; a legend shows for ≥2. Optional dashed reference lines
 * (e.g. an expected-win-rate baseline) are annotations, not series.
 */
export function LineChart<R>({
  rows,
  x,
  xFormat,
  series,
  yFormat = (v) => String(v),
  tooltipFormat,
  yMin,
  yMax,
  refLines = [],
  height = 220,
  tooltipTitle,
  showDots,
}: {
  rows: R[];
  x: (r: R) => number;
  xFormat: (v: number) => string;
  series: LineSeries<R>[];
  yFormat?: (v: number) => string;
  /** Tooltip value formatter; defaults to yFormat (which is tuned for round axis ticks). */
  tooltipFormat?: (v: number) => string;
  yMin?: number;
  yMax?: number;
  refLines?: RefLine[];
  height?: number;
  tooltipTitle?: (r: R) => ReactNode;
  showDots?: boolean;
}) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  if (rows.length === 0) return <EmptyChart height={height} />;

  const xs = rows.map(x);
  let x0 = Math.min(...xs);
  let x1 = Math.max(...xs);
  if (x0 === x1) {
    x0 -= 1;
    x1 += 1;
  }
  const vals = rows.flatMap((r) => series.map((s) => s.value(r))).filter((v): v is number => v != null);
  vals.push(...refLines.map((l) => l.value));
  const y0 = yMin ?? Math.min(0, ...vals);
  const yHi = yMax ?? Math.max(1, ...vals) * 1.08;
  const yTicks = niceTicks(y0, yHi);
  const y1 = Math.max(yHi, yTicks[yTicks.length - 1]);

  const iw = width - M.left - M.right;
  const ih = height - M.top - M.bottom;
  const sx = (v: number) => M.left + ((v - x0) / (x1 - x0)) * iw;
  const sy = (v: number) => M.top + ih - ((v - y0) / (y1 - y0)) * ih;
  const xTickCount = Math.max(2, Math.min(6, Math.floor(iw / 90)));
  const xTicks = Array.from({ length: xTickCount }, (_, i) => x0 + ((x1 - x0) * i) / (xTickCount - 1));

  const path = (s: LineSeries<R>) => {
    let d = "";
    let pen = false;
    rows.forEach((r, i) => {
      const v = s.value(r);
      if (v == null) {
        pen = false;
        return;
      }
      d += `${pen ? "L" : "M"}${sx(xs[i]).toFixed(1)},${sy(v).toFixed(1)}`;
      pen = true;
    });
    return d;
  };

  const onMove = (e: React.PointerEvent<SVGRectElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const px = e.clientX - rect.left + M.left;
    let best = 0;
    for (let i = 1; i < rows.length; i++) if (Math.abs(sx(xs[i]) - px) < Math.abs(sx(xs[best]) - px)) best = i;
    setHover(best);
  };

  const hr = hover != null ? rows[hover] : null;
  const hx = hover != null ? sx(xs[hover]) : 0;
  const dots = showDots ?? rows.length <= 40;

  return (
    <div ref={ref} className="relative w-full">
      {series.length >= 2 ? (
        <div className="mb-2 flex flex-wrap gap-4 text-xs text-gc-muted">
          {series.map((s) => (
            <span key={s.label} className="inline-flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-4 rounded" style={{ background: s.color }} />
              {s.label}
            </span>
          ))}
          {refLines.map((l) => (
            <span key={l.label} className="inline-flex items-center gap-1.5">
              <span className="inline-block w-4 border-t border-dashed" style={{ borderColor: AXIS }} />
              {l.label}
            </span>
          ))}
        </div>
      ) : null}
      <svg width={width} height={height} role="img" className="block overflow-visible">
        {yTicks.map((t) => (
          <g key={t}>
            <line x1={M.left} x2={width - M.right} y1={sy(t)} y2={sy(t)} stroke={GRID} strokeWidth={1} />
            <text x={M.left - 6} y={sy(t)} dy="0.32em" textAnchor="end" fontSize={11} fill={AXIS}>
              {yFormat(t)}
            </text>
          </g>
        ))}
        {xTicks.map((t, i) => (
          <text
            key={i}
            x={sx(t)}
            y={height - 6}
            textAnchor={i === 0 ? "start" : i === xTicks.length - 1 ? "end" : "middle"}
            fontSize={11}
            fill={AXIS}
          >
            {xFormat(t)}
          </text>
        ))}
        {refLines.map((l) => (
          <g key={l.label}>
            <line x1={M.left} x2={width - M.right} y1={sy(l.value)} y2={sy(l.value)} stroke={AXIS} strokeDasharray="4 4" strokeWidth={1.5} />
            {series.length < 2 ? (
              <text x={width - M.right} y={sy(l.value) - 4} textAnchor="end" fontSize={11} fill={AXIS}>
                {l.label}
              </text>
            ) : null}
          </g>
        ))}
        {series.map((s) => (
          <path key={s.label} d={path(s)} fill="none" stroke={s.color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
        ))}
        {dots
          ? series.map((s) =>
              rows.map((r, i) => {
                const v = s.value(r);
                return v == null ? null : (
                  <circle key={`${s.label}-${i}`} cx={sx(xs[i])} cy={sy(v)} r={3} fill={s.color} stroke="#141c28" strokeWidth={1.5} />
                );
              }),
            )
          : null}
        {hr ? (
          <g pointerEvents="none">
            <line x1={hx} x2={hx} y1={M.top} y2={M.top + ih} stroke={AXIS} strokeWidth={1} />
            {series.map((s) => {
              const v = s.value(hr);
              return v == null ? null : (
                <circle key={s.label} cx={hx} cy={sy(v)} r={4.5} fill={s.color} stroke="#141c28" strokeWidth={2} />
              );
            })}
          </g>
        ) : null}
        <rect
          x={M.left}
          y={M.top}
          width={iw}
          height={ih}
          fill="transparent"
          onPointerMove={onMove}
          onPointerLeave={() => setHover(null)}
        />
      </svg>
      {hr ? (
        <Tooltip x={hx} width={width} top={M.top + (series.length >= 2 ? 24 : 0)}>
          <div className="mb-1 font-medium text-gc-text">{tooltipTitle ? tooltipTitle(hr) : xFormat(xs[hover!])}</div>
          {series.map((s) => (
            <div key={s.label} className="flex items-center justify-between gap-4">
              <span className="inline-flex items-center gap-1.5 text-gc-muted">
                <span className="inline-block h-2 w-2 rounded-full" style={{ background: s.color }} />
                {s.label}
              </span>
              <span className="tabular-nums text-gc-text">{s.value(hr) == null ? "—" : (tooltipFormat ?? yFormat)(s.value(hr)!)}</span>
            </div>
          ))}
        </Tooltip>
      ) : null}
    </div>
  );
}

/** Vertical bar chart with a per-bar hover tooltip. */
export function BarChart<R>({
  rows,
  label,
  value,
  color = SERIES.blue,
  format = (v) => String(v),
  height = 200,
  tooltip,
  maxLabels = 8,
}: {
  rows: R[];
  label: (r: R) => string;
  value: (r: R) => number;
  color?: string;
  format?: (v: number) => string;
  height?: number;
  tooltip?: (r: R) => ReactNode;
  maxLabels?: number;
}) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  if (rows.length === 0) return <EmptyChart height={height} />;

  const vmax = Math.max(1, ...rows.map(value));
  const ticks = niceTicks(0, vmax, 3);
  const y1 = ticks[ticks.length - 1];
  const iw = width - M.left - M.right;
  const ih = height - M.top - M.bottom;
  const band = iw / rows.length;
  const gap = Math.min(8, Math.max(2, band * 0.2));
  // Thin marks: cap bar width so a few categories don't become slabs.
  const bw = Math.max(1, Math.min(band - gap, 48));
  const pad = (band - bw) / 2;
  const sy = (v: number) => M.top + ih - (v / y1) * ih;
  const labelEvery = Math.ceil(rows.length / maxLabels);

  return (
    <div ref={ref} className="relative w-full">
      <svg width={width} height={height} role="img" className="block">
        {ticks.map((t) => (
          <g key={t}>
            <line x1={M.left} x2={width - M.right} y1={sy(t)} y2={sy(t)} stroke={GRID} strokeWidth={1} />
            <text x={M.left - 6} y={sy(t)} dy="0.32em" textAnchor="end" fontSize={11} fill={AXIS}>
              {format(t)}
            </text>
          </g>
        ))}
        {rows.map((r, i) => {
          const v = value(r);
          const x = M.left + i * band + pad;
          const h = Math.max(0, sy(0) - sy(v));
          const rad = Math.min(4, bw / 2, h);
          return (
            <g key={i}>
              {v > 0 ? (
                <path
                  d={`M${x},${sy(0)} V${sy(v) + rad} Q${x},${sy(v)} ${x + rad},${sy(v)} H${x + bw - rad} Q${x + bw},${sy(v)} ${x + bw},${sy(v) + rad} V${sy(0)} Z`}
                  fill={color}
                  opacity={hover == null || hover === i ? 1 : 0.55}
                />
              ) : null}
              {i % labelEvery === 0 ? (
                <text x={x + bw / 2} y={height - 6} textAnchor="middle" fontSize={11} fill={AXIS}>
                  {label(r)}
                </text>
              ) : null}
              <rect
                x={M.left + i * band}
                y={M.top}
                width={band}
                height={ih}
                fill="transparent"
                onPointerEnter={() => setHover(i)}
                onPointerLeave={() => setHover(null)}
              />
            </g>
          );
        })}
      </svg>
      {hover != null ? (
        <Tooltip x={M.left + hover * band + band / 2} width={width} top={M.top}>
          {tooltip ? (
            tooltip(rows[hover])
          ) : (
            <>
              <div className="font-medium text-gc-text">{label(rows[hover])}</div>
              <div className="tabular-nums text-gc-muted">{format(value(rows[hover]))}</div>
            </>
          )}
        </Tooltip>
      ) : null}
    </div>
  );
}

function Tooltip({ x, width, top, children }: { x: number; width: number; top: number; children: ReactNode }) {
  const flip = x > width / 2;
  return (
    <div
      className="pointer-events-none absolute z-10 min-w-36 rounded-lg border border-gc-border bg-gc-surface-2 px-3 py-2 text-xs shadow-lg"
      style={{ top, ...(flip ? { right: width - x + 12 } : { left: x + 12 }) }}
    >
      {children}
    </div>
  );
}

function EmptyChart({ height }: { height: number }) {
  return (
    <div className="flex items-center justify-center text-sm text-gc-muted" style={{ height }}>
      No data for these filters.
    </div>
  );
}
