import { useCallback, useEffect, useMemo, useState, type Dispatch, type ReactNode, type SetStateAction } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ReactFlowProvider, useEdgesState, useNodesState } from "@xyflow/react";
import type { ApiError } from "../../api/client";
import { createMapFromGraph, getMap, updateMap, type MapDetail } from "../../api/maps";
import { useAuth } from "../../auth";
import { MapEditorCanvas } from "../../map-editor/MapEditorCanvas";
import {
  MAX_CONTINENTS,
  editorStateFromMap,
  editorStats,
  emptyEditorState,
  newID,
  nextContinentColor,
  neighborsByNode,
  toGraphRequest,
  validateEditorState,
  type BorderEdge,
  type EditorContinent,
  type EditorState,
  type TerritoryNode,
} from "../../map-editor/editorModel";
import { buttonDangerClass, buttonGhostClass, buttonPrimaryClass, inputClass } from "./styles";

type Tab = "canvas" | "territories";

// Shared by /app/admin/maps/new (no mapID) and /app/admin/maps/$mapID.
export function MapEditorPage() {
  const { mapID } = useParams({ strict: false }) as { mapID?: string };
  const navigate = useNavigate();
  const auth = useAuth();

  const [draft] = useState(emptyEditorState);
  // Tagged with the id it was loaded for, so a stale load never renders
  // under a different map's URL (e.g. right after "Create map" navigates).
  const [loaded, setLoaded] = useState<{ id: string; detail: MapDetail; state: EditorState } | null>(null);
  const [loadError, setLoadError] = useState<{ id: string; message: string } | null>(null);

  useEffect(() => {
    if (!mapID) return;
    let cancelled = false;
    void (async () => {
      try {
        const out = await getMap(mapID);
        if (cancelled) return;
        setLoaded({ id: mapID, detail: out, state: editorStateFromMap(out) });
      } catch (err) {
        if (cancelled) return;
        const apiErr = err as ApiError;
        if (apiErr.status === 401) {
          auth.clearSession();
          await navigate({ to: "/login" });
          return;
        }
        if (apiErr.status === 403) {
          await navigate({ to: "/app/lobby" });
          return;
        }
        setLoadError({ id: mapID, message: apiErr.status === 404 ? "Map not found." : apiErr.message || "Failed to load map" });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [mapID, auth, navigate]);

  if (mapID && loadError?.id === mapID) {
    return (
      <div className="mx-auto max-w-md py-16 text-center">
        <p className="text-3xl text-gc-muted">⊘</p>
        <h2 className="mt-2 text-lg font-semibold text-gc-text">{loadError.message}</h2>
        <Link to="/app/admin" className={`${buttonGhostClass} mt-4`}>
          Back to admin
        </Link>
      </div>
    );
  }
  if (mapID && loaded?.id !== mapID) return <p className="py-16 text-center text-sm text-gc-muted">Loading map…</p>;

  return (
    <ReactFlowProvider>
      {/* Remount when switching maps so editor state never leaks between them. */}
      <MapEditor
        key={mapID ?? "new"}
        mapID={mapID}
        detail={mapID ? loaded!.detail : null}
        initial={mapID ? loaded!.state : draft}
        onSaved={(detail) => setLoaded((prev) => (prev ? { ...prev, detail } : prev))}
      />
    </ReactFlowProvider>
  );
}

type MapEditorProps = {
  mapID?: string;
  detail: MapDetail | null;
  initial: EditorState;
  onSaved: (detail: MapDetail) => void;
};

function MapEditor({ mapID, detail, initial, onSaved }: MapEditorProps) {
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>("canvas");
  const [name, setName] = useState(detail?.name ?? "");
  const [continents, setContinents] = useState<EditorContinent[]>(initial.continents);
  const [nodes, setNodes, onNodesChange] = useNodesState<TerritoryNode>(initial.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState<BorderEdge>(initial.edges);
  const [activeContinentID, setActiveContinentID] = useState(initial.continents[0]?.id ?? "");
  const [dirty, setDirty] = useState(!mapID);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");

  const locked = (detail?.gameCount ?? 0) > 0;
  const state = useMemo<EditorState>(() => ({ continents, nodes, edges }), [continents, nodes, edges]);
  const issues = useMemo(() => validateEditorState(name, state), [name, state]);
  const errors = issues.filter((i) => i.level === "error");
  const stats = useMemo(() => editorStats(state), [state]);

  // React Flow reports selection and measurement through the same change
  // stream as edits; only count changes that alter the map itself.
  const handleNodesChange: typeof onNodesChange = useCallback(
    (changes) => {
      if (changes.some((c) => c.type === "remove" || c.type === "add" || (c.type === "position" && c.dragging))) setDirty(true);
      onNodesChange(changes);
    },
    [onNodesChange]
  );
  const handleEdgesChange: typeof onEdgesChange = useCallback(
    (changes) => {
      if (changes.some((c) => c.type === "remove" || c.type === "add")) setDirty(true);
      onEdgesChange(changes);
    },
    [onEdgesChange]
  );
  const setNodesDirty: typeof setNodes = useCallback(
    (v) => {
      setDirty(true);
      setNodes(v);
    },
    [setNodes]
  );
  const setEdgesDirty: typeof setEdges = useCallback(
    (v) => {
      setDirty(true);
      setEdges(v);
    },
    [setEdges]
  );
  const setContinentsDirty: typeof setContinents = useCallback((v) => {
    setDirty(true);
    setContinents(v);
  }, []);

  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const save = async (asNew: boolean) => {
    setSaveError("");
    if (errors.length > 0) {
      setSaveError("Fix the errors listed above before saving.");
      return;
    }
    setSaving(true);
    try {
      const req = toGraphRequest(asNew && mapID ? `${name.trim()} (copy)` : name, state);
      if (mapID && !asNew) {
        const out = await updateMap(mapID, req);
        onSaved({ ...out, gameCount: 0 });
        setDirty(false);
      } else {
        const out = await createMapFromGraph(req);
        setDirty(false);
        await navigate({ to: "/app/admin/maps/$mapID", params: { mapID: out.id } });
      }
    } catch (err) {
      const apiErr = err as ApiError;
      setSaveError(
        apiErr.status === 409
          ? "This map is now used by a game and can't be changed. Save a copy instead."
          : apiErr.message || "Failed to save map"
      );
    } finally {
      setSaving(false);
    }
  };

  const status = locked
    ? { label: `Locked · ${detail?.gameCount} game${detail?.gameCount === 1 ? "" : "s"}`, cls: "border-gc-warning/40 bg-gc-warning/10 text-gc-warning" }
    : dirty
      ? { label: mapID ? "Unsaved changes" : "Draft", cls: "border-gc-accent/40 bg-gc-accent/10 text-gc-accent" }
      : { label: "Saved", cls: "border-gc-success/40 bg-gc-success/10 text-gc-success" };

  return (
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gc-border pb-4">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <Link to="/app/admin" className="text-lg text-gc-muted transition-colors hover:text-gc-text" title="Back to admin">
            ←
          </Link>
          <input
            className="min-w-0 flex-1 rounded-md border border-transparent bg-transparent px-2 py-1 font-display text-xl font-semibold text-gc-text outline-none transition hover:border-gc-border focus:border-gc-accent sm:max-w-md"
            value={name}
            placeholder="Untitled map"
            onChange={(e) => {
              setName(e.target.value);
              setDirty(true);
            }}
          />
          <span className={`shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-semibold ${status.cls}`}>{status.label}</span>
        </div>
        <div className="flex items-center gap-2">
          {mapID ? (
            <button className={buttonGhostClass} type="button" disabled={saving} onClick={() => void save(true)}>
              Save as new
            </button>
          ) : null}
          <button
            className={buttonPrimaryClass}
            type="button"
            disabled={saving || locked || (!dirty && !!mapID)}
            onClick={() => void save(false)}
          >
            {saving ? "Saving…" : mapID ? "Save" : "Create map"}
          </button>
        </div>
      </div>

      {/* Metadata strip */}
      <div className="grid grid-cols-2 gap-4 border-b border-gc-border bg-gc-surface/40 px-1 py-4 sm:grid-cols-4 lg:grid-cols-6">
        <MetaField label="Territories" value={String(stats.territories)} mono />
        <MetaField label="Continents" value={`${stats.continents} / ${MAX_CONTINENTS}`} mono />
        <MetaField label="Borders" value={String(stats.borders)} mono />
        <MetaField label="Cross-continent" value={String(stats.crossings)} mono />
        <MetaField label="Total bonus" value={String(stats.totalBonus)} mono />
        <MetaField label="Created" value={detail?.createdAt ? new Date(detail.createdAt).toLocaleString() : "—"} />
      </div>

      {/* Locked / issues / save error */}
      {locked ? (
        <Section>
          <p className="text-sm text-gc-warning">
            {detail?.gameCount} game{detail?.gameCount === 1 ? " uses" : "s use"} this map, so it can't be changed. You
            can still edit here and use <span className="font-semibold">Save as new</span> to keep a copy.
          </p>
        </Section>
      ) : null}
      {issues.length > 0 ? (
        <Section label={`Issues (${issues.length})`} danger={errors.length > 0}>
          <ul className="grid gap-1 text-sm">
            {issues.map((issue, i) => (
              <li key={i} className={issue.level === "error" ? "text-gc-danger" : "text-gc-warning"}>
                {issue.level === "error" ? "✗" : "!"} {issue.message}
              </li>
            ))}
          </ul>
        </Section>
      ) : null}
      {saveError ? (
        <Section>
          <p className="text-sm text-gc-danger">{saveError}</p>
        </Section>
      ) : null}

      {/* Tab bar */}
      <div className="flex items-center border-b border-gc-border">
        <TabButton active={tab === "canvas"} onClick={() => setTab("canvas")} label="Canvas" />
        <TabButton active={tab === "territories"} onClick={() => setTab("territories")} label={`Territories (${nodes.length})`} />
      </div>

      {tab === "canvas" ? (
        <div className="my-4 flex flex-col gap-4 lg:flex-row">
          <MapEditorCanvas
            nodes={nodes}
            edges={edges}
            continents={continents}
            onNodesChange={handleNodesChange}
            onEdgesChange={handleEdgesChange}
            setNodes={setNodesDirty}
            setEdges={setEdgesDirty}
            activeContinentID={activeContinentID}
            onActiveContinentChange={setActiveContinentID}
            className="h-[560px] min-w-0 flex-1 overflow-hidden rounded-xl border border-gc-border bg-gc-surface-2 lg:h-[680px]"
          />
          <Inspector
            continents={continents}
            nodes={nodes}
            edges={edges}
            setContinents={setContinentsDirty}
            setNodes={setNodesDirty}
            setEdges={setEdgesDirty}
            onActiveContinentChange={setActiveContinentID}
          />
        </div>
      ) : (
        <TerritoryTable continents={continents} nodes={nodes} edges={edges} />
      )}
    </div>
  );
}

// ── Inspector ─────────────────────────────────────────────────────────────────

type InspectorProps = {
  continents: EditorContinent[];
  nodes: TerritoryNode[];
  edges: BorderEdge[];
  setContinents: Dispatch<SetStateAction<EditorContinent[]>>;
  setNodes: Dispatch<SetStateAction<TerritoryNode[]>>;
  setEdges: Dispatch<SetStateAction<BorderEdge[]>>;
  onActiveContinentChange: (id: string) => void;
};

function Inspector({ continents, nodes, edges, setContinents, setNodes, setEdges, onActiveContinentChange }: InspectorProps) {
  const selectedNodes = nodes.filter((n) => n.selected);
  const selectedEdges = edges.filter((e) => e.selected);
  const nameOf = (id: string) => nodes.find((n) => n.id === id)?.data.name || "(unnamed)";
  const countByContinent = new Map<string, number>();
  for (const n of nodes) countByContinent.set(n.data.continentId, (countByContinent.get(n.data.continentId) ?? 0) + 1);

  const updateNode = (id: string, patch: Partial<TerritoryNode["data"]>) =>
    setNodes((ns) => ns.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...patch } } : n)));
  const removeNodes = (ids: string[]) => {
    const drop = new Set(ids);
    setNodes((ns) => ns.filter((n) => !drop.has(n.id)));
    setEdges((es) => es.filter((e) => !drop.has(e.source) && !drop.has(e.target)));
  };
  const removeEdges = (ids: string[]) => {
    const drop = new Set(ids);
    setEdges((es) => es.filter((e) => !drop.has(e.id)));
  };
  const updateContinent = (id: string, patch: Partial<EditorContinent>) =>
    setContinents((cs) => cs.map((c) => (c.id === id ? { ...c, ...patch } : c)));

  let selection: ReactNode = (
    <p className="text-xs text-gc-muted">Select a territory or border to edit it. Ctrl/⌘-click or drag a box to select several.</p>
  );
  if (selectedNodes.length === 1 && selectedEdges.length === 0) {
    const node = selectedNodes[0];
    const neighbors = neighborsByNode(edges).get(node.id) ?? [];
    selection = (
      <div className="grid gap-3">
        <label className="grid gap-1 text-xs font-medium text-gc-muted">
          Name
          <input className={inputClass} value={node.data.name} onChange={(e) => updateNode(node.id, { name: e.target.value })} />
        </label>
        <label className="grid gap-1 text-xs font-medium text-gc-muted">
          Continent
          <select className={inputClass} value={node.data.continentId} onChange={(e) => updateNode(node.id, { continentId: e.target.value })}>
            {continents.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name.trim() || "(unnamed)"}
              </option>
            ))}
          </select>
        </label>
        <div>
          <p className="mb-1 text-xs font-medium text-gc-muted">Borders ({neighbors.length})</p>
          {neighbors.length === 0 ? <p className="text-xs text-gc-danger">None yet — drag from a dot to another territory.</p> : null}
          <ul className="grid gap-1">
            {neighbors.map((other) => (
              <li key={other} className="flex items-center justify-between gap-2 rounded bg-gc-surface-2 px-2 py-1 text-xs text-gc-text">
                <span className="truncate">{nameOf(other)}</span>
                <button
                  type="button"
                  className="text-gc-muted hover:text-gc-danger"
                  title="Remove border"
                  onClick={() => removeEdges(edges.filter((e) => (e.source === node.id && e.target === other) || (e.source === other && e.target === node.id)).map((e) => e.id))}
                >
                  ✕
                </button>
              </li>
            ))}
          </ul>
        </div>
        <button type="button" className={buttonDangerClass} onClick={() => removeNodes([node.id])}>
          Delete territory
        </button>
      </div>
    );
  } else if (selectedEdges.length === 1 && selectedNodes.length === 0) {
    const edge = selectedEdges[0];
    selection = (
      <div className="grid gap-3">
        <p className="text-sm text-gc-text">
          {nameOf(edge.source)} <span className="text-gc-muted">⇄</span> {nameOf(edge.target)}
        </p>
        <button type="button" className={buttonDangerClass} onClick={() => removeEdges([edge.id])}>
          Remove border
        </button>
      </div>
    );
  } else if (selectedNodes.length + selectedEdges.length > 1) {
    selection = (
      <div className="grid gap-3">
        <p className="text-xs text-gc-muted">
          {selectedNodes.length} territories, {selectedEdges.length} borders selected.
        </p>
        {selectedNodes.length > 0 ? (
          <label className="grid gap-1 text-xs font-medium text-gc-muted">
            Move territories to
            <select
              className={inputClass}
              value=""
              onChange={(e) => {
                const ids = new Set(selectedNodes.map((n) => n.id));
                setNodes((ns) => ns.map((n) => (ids.has(n.id) ? { ...n, data: { ...n.data, continentId: e.target.value } } : n)));
              }}
            >
              <option value="" disabled>
                Choose continent…
              </option>
              {continents.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name.trim() || "(unnamed)"}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <button
          type="button"
          className={buttonDangerClass}
          onClick={() => {
            removeEdges(selectedEdges.map((e) => e.id));
            removeNodes(selectedNodes.map((n) => n.id));
          }}
        >
          Delete selection
        </button>
      </div>
    );
  }

  return (
    <aside className="flex w-full shrink-0 flex-col gap-4 lg:w-80">
      <div className="rounded-xl border border-gc-border bg-gc-surface p-3">
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gc-muted">Selection</p>
        {selection}
      </div>

      <div className="rounded-xl border border-gc-border bg-gc-surface p-3">
        <div className="mb-2 flex items-center justify-between">
          <p className="text-xs font-semibold uppercase tracking-wide text-gc-muted">Continents</p>
          <button
            type="button"
            className="rounded border border-gc-border px-2 py-0.5 text-xs text-gc-text hover:bg-gc-surface-2 disabled:opacity-50"
            disabled={continents.length >= MAX_CONTINENTS}
            onClick={() => {
              const c: EditorContinent = { id: newID("c"), name: `Continent ${continents.length + 1}`, bonus: 2, color: nextContinentColor(continents) };
              setContinents((cs) => [...cs, c]);
              onActiveContinentChange(c.id);
            }}
          >
            + Continent
          </button>
        </div>
        <ul className="grid gap-2">
          {continents.map((c) => {
            const count = countByContinent.get(c.id) ?? 0;
            return (
              <li key={c.id} className="flex items-center gap-2">
                <span className="h-3 w-3 shrink-0 rounded-full" style={{ background: c.color }} />
                <input
                  className={`${inputClass} !px-2 !py-1`}
                  value={c.name}
                  placeholder="Name"
                  onChange={(e) => updateContinent(c.id, { name: e.target.value })}
                />
                <input
                  className={`${inputClass} !w-14 !px-2 !py-1`}
                  type="number"
                  min={0}
                  title="Bonus"
                  value={c.bonus}
                  onChange={(e) => updateContinent(c.id, { bonus: Math.max(0, Math.floor(Number(e.target.value) || 0)) })}
                />
                <span className="w-6 shrink-0 text-right font-mono text-xs text-gc-muted" title="Territories">
                  {count}
                </span>
                <button
                  type="button"
                  className="text-gc-muted hover:text-gc-danger disabled:opacity-30 disabled:hover:text-gc-muted"
                  disabled={count > 0}
                  title={count > 0 ? "Move or delete its territories first" : "Delete continent"}
                  onClick={() => setContinents((cs) => cs.filter((x) => x.id !== c.id))}
                >
                  ✕
                </button>
              </li>
            );
          })}
        </ul>
        <p className="mt-2 text-[10px] text-gc-muted">Name · bonus · territory count</p>
      </div>
    </aside>
  );
}

// ── Territories tab ───────────────────────────────────────────────────────────

function TerritoryTable({ continents, nodes, edges }: { continents: EditorContinent[]; nodes: TerritoryNode[]; edges: BorderEdge[] }) {
  const neighbors = neighborsByNode(edges);
  const byID = new Map(nodes.map((n) => [n.id, n]));
  return (
    <div className="my-4 grid gap-4">
      {continents.map((c) => {
        const members = nodes.filter((n) => n.data.continentId === c.id);
        return (
          <section key={c.id} className="overflow-hidden rounded-xl border border-gc-border bg-gc-surface">
            <header className="flex items-center gap-2 border-b border-gc-border bg-gc-surface-2 px-4 py-2">
              <span className="h-2.5 w-2.5 rounded-full" style={{ background: c.color }} />
              <span className="text-sm font-semibold text-gc-text">{c.name.trim() || "(unnamed)"}</span>
              <span className="text-xs text-gc-muted">
                +{c.bonus} bonus · {members.length} territories
              </span>
            </header>
            {members.length === 0 ? <p className="px-4 py-3 text-xs text-gc-muted">No territories.</p> : null}
            <ul className="divide-y divide-gc-border/60">
              {members.map((n) => {
                const ns = (neighbors.get(n.id) ?? []).map((id) => byID.get(id)).filter((x): x is TerritoryNode => !!x);
                return (
                  <li key={n.id} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-2">
                    <span className="w-44 shrink-0 truncate text-sm text-gc-text">{n.data.name || "(unnamed)"}</span>
                    <span className="flex flex-1 flex-wrap gap-1">
                      {ns.length === 0 ? <span className="text-xs text-gc-danger">no borders</span> : null}
                      {ns.map((o) => {
                        const cross = o.data.continentId !== c.id;
                        return (
                          <span
                            key={o.id}
                            className={`rounded px-1.5 py-0.5 text-[11px] ${cross ? "border border-dashed border-gc-border text-gc-muted" : "bg-gc-surface-2 text-gc-text"}`}
                          >
                            {o.data.name || "(unnamed)"}
                          </span>
                        );
                      })}
                    </span>
                  </li>
                );
              })}
            </ul>
          </section>
        );
      })}
    </div>
  );
}

// ── Small pieces ──────────────────────────────────────────────────────────────

function MetaField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <p className="mb-0.5 text-[11px] font-semibold uppercase tracking-wide text-gc-muted">{label}</p>
      <p className={`truncate text-sm text-gc-text ${mono ? "font-mono" : ""}`} title={value}>
        {value}
      </p>
    </div>
  );
}

function Section({ label, danger, children }: { label?: string; danger?: boolean; children: ReactNode }) {
  return (
    <div className="border-b border-gc-border px-1 py-3">
      {label ? (
        <p className={`mb-1.5 text-[11px] font-semibold uppercase tracking-wide ${danger ? "text-gc-danger" : "text-gc-warning"}`}>{label}</p>
      ) : null}
      {children}
    </div>
  );
}

function TabButton({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`-mb-px border-b-2 px-4 py-3 text-sm transition-colors ${
        active ? "border-gc-accent text-gc-accent" : "border-transparent text-gc-muted hover:text-gc-text"
      }`}
    >
      {label}
    </button>
  );
}
