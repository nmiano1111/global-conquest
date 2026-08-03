import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type MapSummary = {
  id: string;
  name: string;
  ownerUserId: string;
  continentCount: number;
  territoryCount: number;
  createdAt: string;
};

export type MapContinentInfo = {
  bonus: number;
  territories: string[];
};

export type MapTerritoryPosition = {
  x: number;
  y: number;
};

export type MapDetail = MapSummary & {
  continents: Record<string, MapContinentInfo>;
  territories: Record<string, MapTerritoryPosition>;
  edges: Array<[string, string]>;
  order: string[];
};

export type CreateMapContinentRequest = {
  name: string;
  bonus: number;
  territoryCount: number;
};

export type CreateMapBorderRequest = {
  a: string;
  b: string;
  crossings: number;
};

export type CreateMapRequest = {
  name: string;
  continents: CreateMapContinentRequest[];
  borders: CreateMapBorderRequest[];
};

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== "object") return null;
  return value as UnknownRecord;
}

function readString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function readNumber(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function normalizeMapSummary(value: unknown): MapSummary {
  const record = asRecord(value);
  if (!record) {
    return { id: "", name: "", ownerUserId: "", continentCount: 0, territoryCount: 0, createdAt: "" };
  }
  return {
    id: readString(record.id),
    name: readString(record.name),
    ownerUserId: readString(record.owner_user_id),
    continentCount: readNumber(record.continent_count),
    territoryCount: readNumber(record.territory_count),
    createdAt: readString(record.created_at),
  };
}

function normalizeMapDetail(value: unknown): MapDetail {
  const summary = normalizeMapSummary(value);
  const record = asRecord(value);
  const definition = asRecord(record?.definition);
  const board = asRecord(definition?.board);

  const continentsRaw = asRecord(board?.continents) ?? {};
  const continents: Record<string, MapContinentInfo> = {};
  for (const [name, info] of Object.entries(continentsRaw)) {
    const infoRecord = asRecord(info);
    const territoriesRaw = infoRecord?.territories;
    continents[name] = {
      bonus: readNumber(infoRecord?.bonus),
      territories: Array.isArray(territoriesRaw) ? territoriesRaw.filter((t): t is string => typeof t === "string") : [],
    };
  }

  const orderRaw = board?.order;
  const order = Array.isArray(orderRaw) ? orderRaw.filter((t): t is string => typeof t === "string") : [];

  const layoutRaw = asRecord(definition?.layout) ?? {};
  const territories: Record<string, MapTerritoryPosition> = {};
  for (const [name, pos] of Object.entries(layoutRaw)) {
    const posRecord = asRecord(pos);
    territories[name] = { x: readNumber(posRecord?.x), y: readNumber(posRecord?.y) };
  }

  const adjacentRaw = asRecord(board?.adjacent) ?? {};
  const edges: Array<[string, string]> = [];
  const seen = new Set<string>();
  for (const [from, neighbors] of Object.entries(adjacentRaw)) {
    const neighborsRecord = asRecord(neighbors) ?? {};
    for (const to of Object.keys(neighborsRecord)) {
      const key = from < to ? `${from}|${to}` : `${to}|${from}`;
      if (seen.has(key)) continue;
      seen.add(key);
      edges.push(from < to ? [from, to] : [to, from]);
    }
  }

  return { ...summary, continents, territories, edges, order };
}

export async function listMaps(): Promise<MapSummary[]> {
  const res = await request<unknown>({ method: "GET", url: "/admin/maps" });
  if (!Array.isArray(res)) return [];
  return res.map((item) => normalizeMapSummary(item));
}

export async function getMap(mapID: string): Promise<MapDetail> {
  const res = await request<unknown>({ method: "GET", url: `/admin/maps/${encodeURIComponent(mapID)}` });
  return normalizeMapDetail(res);
}

export async function createMap(input: CreateMapRequest): Promise<MapDetail> {
  const data = {
    name: input.name,
    continents: input.continents.map((c) => ({ name: c.name, bonus: c.bonus, territory_count: c.territoryCount })),
    borders: input.borders.map((b) => ({ a: b.a, b: b.b, crossings: b.crossings })),
  };
  const res = await request<unknown>({ method: "POST", url: "/admin/maps", data });
  return normalizeMapDetail(res);
}

export async function deleteMap(mapID: string): Promise<void> {
  await request<unknown>({ method: "DELETE", url: `/admin/maps/${encodeURIComponent(mapID)}` });
}
