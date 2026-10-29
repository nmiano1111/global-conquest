import type { Edge, Node } from "@xyflow/react";
import type { MapDetail, MapGraphRequest } from "../api/maps";

// Editor-side model for the admin map editor. Territories and borders live
// in React Flow's own node/edge arrays (so dragging, selection and deletion
// come for free); continents live alongside them. Every entity has a
// stable local id, so renaming a territory or continent never touches the
// edges or memberships that reference it. Names only matter on save.

export type EditorContinent = {
  id: string;
  name: string;
  bonus: number;
  color: string;
};

export type TerritoryData = {
  name: string;
  continentId: string;
};

export type TerritoryNode = Node<TerritoryData, "territory">;
export type BorderEdge = Edge<Record<string, never>, "border">;

export type EditorState = {
  continents: EditorContinent[];
  nodes: TerritoryNode[];
  edges: BorderEdge[];
};

// Stored layouts are normalized to [0,1]; the editor works in a larger
// space so default node sizes and spacing read naturally.
export const CANVAS_SCALE = 1000;

export const CONTINENT_COLORS = [
  "#ef4444",
  "#3b82f6",
  "#22c55e",
  "#f59e0b",
  "#a855f7",
  "#06b6d4",
  "#ec4899",
  "#84cc16",
  "#f97316",
  "#6366f1",
  "#14b8a6",
  "#eab308",
];

// Mirrors mapgen's bounds (backend/internal/mapgen) so limits surface while
// drawing instead of only on save. The backend stays authoritative.
export const MIN_CONTINENTS = 2;
export const MAX_CONTINENTS = 12;
export const MAX_TERRITORIES_PER_CONTINENT = 20;

let idCounter = 0;
export function newID(prefix: string): string {
  idCounter += 1;
  return `${prefix}-${Date.now().toString(36)}-${idCounter}`;
}

export function edgeID(a: string, b: string): string {
  return a < b ? `${a}--${b}` : `${b}--${a}`;
}

export function makeBorder(a: string, b: string): BorderEdge {
  return { id: edgeID(a, b), source: a, target: b, type: "border" };
}

export function makeTerritory(name: string, continentId: string, x: number, y: number): TerritoryNode {
  return { id: newID("t"), type: "territory", position: { x, y }, data: { name, continentId } };
}

export function nextContinentColor(continents: EditorContinent[]): string {
  const used = new Set(continents.map((c) => c.color));
  return CONTINENT_COLORS.find((c) => !used.has(c)) ?? CONTINENT_COLORS[continents.length % CONTINENT_COLORS.length];
}

export function emptyEditorState(): EditorState {
  const a: EditorContinent = { id: newID("c"), name: "Continent A", bonus: 2, color: CONTINENT_COLORS[0] };
  const b: EditorContinent = { id: newID("c"), name: "Continent B", bonus: 2, color: CONTINENT_COLORS[1] };
  const nodes = [
    makeTerritory("Continent A 1", a.id, 200, 300),
    makeTerritory("Continent A 2", a.id, 200, 480),
    makeTerritory("Continent B 1", b.id, 560, 300),
    makeTerritory("Continent B 2", b.id, 560, 480),
  ];
  const edges = [
    makeBorder(nodes[0].id, nodes[1].id),
    makeBorder(nodes[2].id, nodes[3].id),
    makeBorder(nodes[0].id, nodes[2].id),
  ];
  return { continents: [a, b], nodes, edges };
}

export function editorStateFromMap(detail: MapDetail): EditorState {
  const continents: EditorContinent[] = detail.continentOrder.map((name, i) => ({
    id: newID("c"),
    name,
    bonus: detail.continents[name]?.bonus ?? 0,
    color: CONTINENT_COLORS[i % CONTINENT_COLORS.length],
  }));
  const continentIDByTerritory = new Map<string, string>();
  for (const c of continents) {
    for (const t of detail.continents[c.name]?.territories ?? []) continentIDByTerritory.set(t, c.id);
  }

  const nodeIDByName = new Map<string, string>();
  const nodes: TerritoryNode[] = detail.order.map((name) => {
    const pos = detail.territories[name] ?? { x: 0.5, y: 0.5 };
    const node = makeTerritory(name, continentIDByTerritory.get(name) ?? "", pos.x * CANVAS_SCALE, pos.y * CANVAS_SCALE);
    nodeIDByName.set(name, node.id);
    return node;
  });

  const edges: BorderEdge[] = [];
  for (const [a, b] of detail.edges) {
    const ia = nodeIDByName.get(a);
    const ib = nodeIDByName.get(b);
    if (ia && ib) edges.push(makeBorder(ia, ib));
  }
  return { continents, nodes, edges };
}

export function toGraphRequest(name: string, state: EditorState): MapGraphRequest {
  const continentName = new Map(state.continents.map((c) => [c.id, c.name.trim()]));
  const territoryName = new Map(state.nodes.map((n) => [n.id, n.data.name.trim()]));
  return {
    name: name.trim(),
    continents: state.continents.map((c) => ({ name: c.name.trim(), bonus: c.bonus })),
    territories: state.nodes.map((n) => ({
      name: n.data.name.trim(),
      continent: continentName.get(n.data.continentId) ?? "",
      // Save node centers, not React Flow's top-left anchor.
      x: n.position.x + (n.measured?.width ?? 0) / 2,
      y: n.position.y + (n.measured?.height ?? 0) / 2,
    })),
    edges: state.edges.map((e) => [territoryName.get(e.source) ?? "", territoryName.get(e.target) ?? ""]),
  };
}

export function uniqueTerritoryName(base: string, nodes: TerritoryNode[]): string {
  const taken = new Set(nodes.map((n) => n.data.name.trim()));
  for (let i = 1; ; i++) {
    const candidate = `${base} ${i}`;
    if (!taken.has(candidate)) return candidate;
  }
}

export function neighborsByNode(edges: BorderEdge[]): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const e of edges) {
    out.set(e.source, [...(out.get(e.source) ?? []), e.target]);
    out.set(e.target, [...(out.get(e.target) ?? []), e.source]);
  }
  return out;
}

/** Connected components over the given node ids, following only edges between them. */
function components(ids: string[], neighbors: Map<string, string[]>): string[][] {
  const inSet = new Set(ids);
  const seen = new Set<string>();
  const out: string[][] = [];
  for (const start of ids) {
    if (seen.has(start)) continue;
    const comp: string[] = [];
    const queue = [start];
    seen.add(start);
    while (queue.length > 0) {
      const cur = queue.shift()!;
      comp.push(cur);
      for (const next of neighbors.get(cur) ?? []) {
        if (!inSet.has(next) || seen.has(next)) continue;
        seen.add(next);
        queue.push(next);
      }
    }
    out.push(comp);
  }
  return out;
}

export type EditorIssue = { level: "error" | "warning"; message: string };

function listNames(names: string[]): string {
  const shown = names.slice(0, 4).map((n) => n || "(unnamed)");
  return names.length > 4 ? `${shown.join(", ")} +${names.length - 4} more` : shown.join(", ");
}

/**
 * Authoring hints computed while drawing. Errors mirror what mapgen.FromGraph
 * rejects; warnings flag legal-but-odd shapes. The backend re-validates on save.
 */
export function validateEditorState(name: string, state: EditorState): EditorIssue[] {
  const issues: EditorIssue[] = [];
  const err = (message: string) => issues.push({ level: "error", message });
  const warn = (message: string) => issues.push({ level: "warning", message });

  if (!name.trim()) err("Map needs a name.");

  if (state.continents.length < MIN_CONTINENTS || state.continents.length > MAX_CONTINENTS) {
    err(`Maps need between ${MIN_CONTINENTS} and ${MAX_CONTINENTS} continents.`);
  }
  const continentNames = new Map<string, number>();
  for (const c of state.continents) {
    const n = c.name.trim();
    if (!n) err("Every continent needs a name.");
    else continentNames.set(n, (continentNames.get(n) ?? 0) + 1);
    if (c.bonus < 0) err(`${n || "A continent"}'s bonus can't be negative.`);
  }
  for (const [n, count] of continentNames) if (count > 1) err(`Continent name "${n}" is used ${count} times.`);

  const territoryNames = new Map<string, number>();
  for (const node of state.nodes) {
    const n = node.data.name.trim();
    if (!n) err("Every territory needs a name.");
    else territoryNames.set(n, (territoryNames.get(n) ?? 0) + 1);
  }
  for (const [n, count] of territoryNames) if (count > 1) err(`Territory name "${n}" is used ${count} times.`);

  const neighbors = neighborsByNode(state.edges);
  const nameOf = (id: string) => state.nodes.find((n) => n.id === id)?.data.name.trim() ?? "";

  for (const c of state.continents) {
    const members = state.nodes.filter((n) => n.data.continentId === c.id).map((n) => n.id);
    const label = c.name.trim() || "A continent";
    if (members.length === 0) err(`${label} has no territories.`);
    else if (members.length > MAX_TERRITORIES_PER_CONTINENT) {
      err(`${label} has ${members.length} territories (max ${MAX_TERRITORIES_PER_CONTINENT}).`);
    } else if (components(members, neighbors).length > 1) {
      warn(`${label} is split into ${components(members, neighbors).length} disconnected pieces.`);
    }
  }

  if (state.nodes.length === 0) {
    err("Add at least one territory.");
  } else {
    const isolated = state.nodes.filter((n) => (neighbors.get(n.id)?.length ?? 0) === 0);
    if (isolated.length > 0 && state.nodes.length > 1) {
      err(`No borders yet: ${listNames(isolated.map((n) => n.data.name.trim()))}.`);
    }
    const comps = components(
      state.nodes.map((n) => n.id),
      neighbors
    ).filter((c) => c.length > 1);
    if (comps.length > 1) {
      err(
        `The board is split into ${comps.length} separate groups. Connect ${listNames(comps.map((c) => nameOf(c[0])))} so every territory is reachable.`
      );
    }
  }
  return issues;
}

export type EditorStats = {
  territories: number;
  continents: number;
  borders: number;
  crossings: number;
  totalBonus: number;
};

export function editorStats(state: EditorState): EditorStats {
  const continentOf = new Map(state.nodes.map((n) => [n.id, n.data.continentId]));
  return {
    territories: state.nodes.length,
    continents: state.continents.length,
    borders: state.edges.length,
    crossings: state.edges.filter((e) => continentOf.get(e.source) !== continentOf.get(e.target)).length,
    totalBonus: state.continents.reduce((sum, c) => sum + c.bonus, 0),
  };
}
