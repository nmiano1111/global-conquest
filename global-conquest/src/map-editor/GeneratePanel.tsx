import { useState } from "react";
import type { ApiError } from "../api/client";
import { generateMap, type MapDetail } from "../api/maps";
import { buttonGhostClass, buttonPrimaryClass, inputClass } from "../router/pages/styles";
import {
  CLASSIC_SIZED_SPEC,
  MAX_CONTINENTS,
  MAX_TERRITORIES_PER_CONTINENT,
  MIN_CONTINENTS,
  type GenerateSpec, type SpecBorder, type SpecContinent } from "./editorModel";

type GeneratePanelProps = {
  /** Starting spec; the editor passes one derived from the current canvas. */
  initialSpec: GenerateSpec;
  /** Rebuilds a spec from whatever is on the canvas right now. */
  specFromCanvas: () => GenerateSpec;
  /** How many territories the canvas holds, i.e. what generating replaces. */
  canvasTerritoryCount: number;
  mapName: string;
  onGenerated: (detail: MapDetail) => void;
};

// Spec form for the backend map generator. Generating never saves: the
// result replaces the canvas as an unsaved draft the admin can then edit.
export function GeneratePanel({ initialSpec, specFromCanvas, canvasTerritoryCount, mapName, onGenerated }: GeneratePanelProps) {
  const [continents, setContinents] = useState<SpecContinent[]>(initialSpec.continents);
  const [borders, setBorders] = useState<SpecBorder[]>(initialSpec.borders);
  const [error, setError] = useState("");
  const [generating, setGenerating] = useState(false);

  const names = continents.map((c) => c.name.trim()).filter(Boolean);
  const updateContinent = (idx: number, patch: Partial<SpecContinent>) =>
    setContinents((rows) => rows.map((row, i) => (i === idx ? { ...row, ...patch } : row)));
  const renameContinent = (idx: number, name: string) => {
    const old = continents[idx].name.trim();
    updateContinent(idx, { name });
    // Keep borders pointing at the renamed continent.
    if (old) setBorders((rows) => rows.map((b) => ({ ...b, a: b.a === old ? name.trim() : b.a, b: b.b === old ? name.trim() : b.b })));
  };
  const updateBorder = (idx: number, patch: Partial<SpecBorder>) =>
    setBorders((rows) => rows.map((row, i) => (i === idx ? { ...row, ...patch } : row)));

  const generate = async () => {
    setError("");
    if (continents.length < MIN_CONTINENTS || continents.length > MAX_CONTINENTS) {
      setError(`Maps need between ${MIN_CONTINENTS} and ${MAX_CONTINENTS} continents.`);
      return;
    }
    if (continents.some((c) => !c.name.trim())) {
      setError("Every continent needs a name.");
      return;
    }
    if (new Set(names).size !== names.length) {
      setError("Continent names must be unique.");
      return;
    }
    if (borders.some((b) => !b.a || !b.b || b.a === b.b)) {
      setError("Every border needs two different continents.");
      return;
    }
    setGenerating(true);
    try {
      const out = await generateMap({
        name: mapName.trim(),
        continents: continents.map((c) => ({ ...c, name: c.name.trim() })),
        borders,
      });
      onGenerated(out);
    } catch (err) {
      setError((err as ApiError).message || "Failed to generate map");
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div className="my-4 grid gap-4 lg:grid-cols-2">
      <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
        <div className="mb-3 flex items-center justify-between">
          <p className="text-xs font-semibold uppercase tracking-wide text-gc-muted">Continents</p>
          <button
            className={buttonGhostClass}
            type="button"
            disabled={continents.length >= MAX_CONTINENTS}
            onClick={() => setContinents((rows) => [...rows, { name: `Continent ${rows.length + 1}`, bonus: 2, territoryCount: 4 }])}
          >
            + Continent
          </button>
        </div>
        <div className="grid gap-2">
          <div className="grid grid-cols-[1fr_4rem_5rem_2rem] gap-2 text-[10px] uppercase tracking-wide text-gc-muted">
            <span>Name</span>
            <span>Bonus</span>
            <span>Territories</span>
            <span />
          </div>
          {continents.map((row, idx) => (
            <div key={idx} className="grid grid-cols-[1fr_4rem_5rem_2rem] items-center gap-2">
              <input className={`${inputClass} !py-1`} value={row.name} placeholder="Name" onChange={(e) => renameContinent(idx, e.target.value)} />
              <input
                className={`${inputClass} !px-2 !py-1`}
                type="number"
                min={0}
                value={row.bonus}
                onChange={(e) => updateContinent(idx, { bonus: Math.max(0, Math.floor(Number(e.target.value) || 0)) })}
              />
              <input
                className={`${inputClass} !px-2 !py-1`}
                type="number"
                min={1}
                max={MAX_TERRITORIES_PER_CONTINENT}
                value={row.territoryCount}
                onChange={(e) =>
                  updateContinent(idx, {
                    territoryCount: Math.min(MAX_TERRITORIES_PER_CONTINENT, Math.max(1, Math.floor(Number(e.target.value) || 1))),
                  })
                }
              />
              <button
                type="button"
                className="text-gc-muted hover:text-gc-danger disabled:opacity-30"
                title="Remove continent"
                disabled={continents.length <= MIN_CONTINENTS}
                onClick={() => {
                  const gone = row.name.trim();
                  setContinents((rows) => rows.filter((_, i) => i !== idx));
                  setBorders((rows) => rows.filter((b) => b.a !== gone && b.b !== gone));
                }}
              >
                ✕
              </button>
            </div>
          ))}
        </div>
      </section>

      <section className="rounded-xl border border-gc-border bg-gc-surface p-4">
        <div className="mb-3 flex items-center justify-between">
          <p className="text-xs font-semibold uppercase tracking-wide text-gc-muted">Borders between continents</p>
          <button className={buttonGhostClass} type="button" onClick={() => setBorders((rows) => [...rows, { a: "", b: "", crossings: 1 }])}>
            + Border
          </button>
        </div>
        <div className="grid gap-2">
          {borders.length === 0 ? <p className="text-xs text-gc-muted">No borders yet. Every continent must be reachable.</p> : null}
          {borders.map((row, idx) => (
            <div key={idx} className="grid grid-cols-[1fr_1fr_4rem_2rem] items-center gap-2">
              {(["a", "b"] as const).map((side) => (
                <select key={side} className={`${inputClass} !py-1`} value={row[side]} onChange={(e) => updateBorder(idx, { [side]: e.target.value })}>
                  <option value="">Continent…</option>
                  {names.map((n) => (
                    <option key={n} value={n}>
                      {n}
                    </option>
                  ))}
                </select>
              ))}
              <input
                className={`${inputClass} !px-2 !py-1`}
                type="number"
                min={1}
                title="Crossings: how many territory-to-territory borders join these continents"
                value={row.crossings}
                onChange={(e) => updateBorder(idx, { crossings: Math.max(1, Math.floor(Number(e.target.value) || 1)) })}
              />
              <button type="button" className="text-gc-muted hover:text-gc-danger" title="Remove border" onClick={() => setBorders((rows) => rows.filter((_, i) => i !== idx))}>
                ✕
              </button>
            </div>
          ))}
        </div>
        <p className="mt-2 text-[10px] text-gc-muted">The number is crossings: how many territory pairs border across the two continents.</p>
      </section>

      <div className="flex flex-wrap items-center gap-3 lg:col-span-2">
        <button className={buttonPrimaryClass} type="button" disabled={generating} onClick={() => void generate()}>
          {generating ? "Generating…" : canvasTerritoryCount > 0 ? "Generate onto canvas" : "Generate"}
        </button>
        <button
          className={buttonGhostClass}
          type="button"
          onClick={() => {
            const spec = specFromCanvas();
            setContinents(spec.continents);
            setBorders(spec.borders);
          }}
        >
          Copy from canvas
        </button>
        <button
          className={buttonGhostClass}
          type="button"
          title="6 continents, 42 territories, with the classic board's sizes, bonuses and crossings"
          onClick={() => {
            setContinents(CLASSIC_SIZED_SPEC.continents);
            setBorders(CLASSIC_SIZED_SPEC.borders);
          }}
        >
          Classic size
        </button>
        <p className="text-xs text-gc-muted">
          {canvasTerritoryCount > 0
            ? `Replaces the ${canvasTerritoryCount} territories on the canvas. Nothing is saved until you click Save or Create map.`
            : "Nothing is saved until you click Create map."}{" "}
          Generate again for a different random layout.
        </p>
      </div>
      {error ? (
        <p className="rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger lg:col-span-2">{error}</p>
      ) : null}
    </div>
  );
}
