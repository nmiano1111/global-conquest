import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { getBotTuningConfig, updateBotTuningConfig, type BotPacingConfig, type BotTuningConfig } from "../../api/botTuning";
import { createMap, deleteMap, getMap, listMaps, type MapDetail, type MapSummary } from "../../api/maps";
import {
  listAdminUsers,
  revokeUserSessions,
  updateUserAccess,
  updateUserSandbox,
  type AdminUserRecord,
} from "../../api/users";
import { useAuth } from "../../auth";
import { buttonGhostClass, buttonPrimaryClass, inputClass } from "./styles";

// One row per PacingConfig category (backend/internal/bot/pacing.go) --
// min/max bound the random delay applied after that kind of committed bot
// action in a live (human-involved) game.
const PACING_ROWS: { label: string; minKey: keyof BotPacingConfig; maxKey: keyof BotPacingConfig }[] = [
  { label: "Turn start", minKey: "turnStartMinMs", maxKey: "turnStartMaxMs" },
  { label: "Card turn-in", minKey: "cardTurnInMinMs", maxKey: "cardTurnInMaxMs" },
  { label: "Reinforcement", minKey: "reinforcementMinMs", maxKey: "reinforcementMaxMs" },
  { label: "First attack", minKey: "firstAttackMinMs", maxKey: "firstAttackMaxMs" },
  { label: "Repeat attack", minKey: "repeatAttackMinMs", maxKey: "repeatAttackMaxMs" },
  { label: "Capture", minKey: "captureMinMs", maxKey: "captureMaxMs" },
  { label: "Occupation", minKey: "occupationMinMs", maxKey: "occupationMaxMs" },
  { label: "Fortify", minKey: "fortifyMinMs", maxKey: "fortifyMaxMs" },
  { label: "End attack", minKey: "endAttackMinMs", maxKey: "endAttackMaxMs" },
  { label: "Dramatic (elimination/game over)", minKey: "dramaticMinMs", maxKey: "dramaticMaxMs" },
];
import { MAP_PLAYER_COLORS } from "./gameShared";

type ContinentRow = { name: string; bonus: number; territoryCount: number };
type BorderRow = { a: string; b: string; crossings: number };

function continentColor(continents: string[], name: string): string {
  const idx = continents.indexOf(name);
  return MAP_PLAYER_COLORS[idx % MAP_PLAYER_COLORS.length];
}

function MapPreview({ detail }: { detail: MapDetail }) {
  const continentNames = Object.keys(detail.continents);
  const territoryColor = (territory: string): string => {
    for (const name of continentNames) {
      if (detail.continents[name]?.territories.includes(territory)) {
        return continentColor(continentNames, name);
      }
    }
    return "#888";
  };
  return (
    <svg viewBox="0 0 100 100" className="h-32 w-32 shrink-0 rounded-lg border border-gc-border bg-gc-surface-2">
      {detail.edges.map(([a, b]) => {
        const pa = detail.territories[a];
        const pb = detail.territories[b];
        if (!pa || !pb) return null;
        return (
          <line
            key={`${a}-${b}`}
            x1={pa.x * 100}
            y1={pa.y * 100}
            x2={pb.x * 100}
            y2={pb.y * 100}
            stroke="currentColor"
            strokeWidth={0.5}
            className="text-gc-border"
          />
        );
      })}
      {detail.order.map((t) => {
        const p = detail.territories[t];
        if (!p) return null;
        return <circle key={t} cx={p.x * 100} cy={p.y * 100} r={2.5} fill={territoryColor(t)} />;
      })}
    </svg>
  );
}

export function AdminPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [users, setUsers] = useState<AdminUserRecord[]>([]);
  const [busyUserID, setBusyUserID] = useState("");

  const [botTuning, setBotTuning] = useState<BotTuningConfig | null>(null);
  const [botTuningLoading, setBotTuningLoading] = useState(true);
  const [botTuningError, setBotTuningError] = useState("");
  const [botTuningSaving, setBotTuningSaving] = useState(false);
  const [botTuningSaved, setBotTuningSaved] = useState(false);

  const load = useCallback(
    async (cancelled = false) => {
      setError("");
      try {
        const out = await listAdminUsers();
        if (cancelled) return;
        setUsers(out);
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
        setError(apiErr.message || "Failed to load admin users");
      }
    },
    [auth, navigate]
  );

  const loadBotTuning = useCallback(
    async (cancelled = false) => {
      setBotTuningError("");
      try {
        const out = await getBotTuningConfig();
        if (cancelled) return;
        setBotTuning(out);
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
        setBotTuningError(apiErr.message || "Failed to load bot tuning config");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setLoading(true);
      await load(cancelled);
      if (!cancelled) setLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [load]);

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setBotTuningLoading(true);
      await loadBotTuning(cancelled);
      if (!cancelled) setBotTuningLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [loadBotTuning]);

  const onSaveBotTuning = async () => {
    if (!botTuning) return;
    setBotTuningSaving(true);
    setBotTuningSaved(false);
    setBotTuningError("");
    try {
      const saved = await updateBotTuningConfig(botTuning);
      setBotTuning(saved);
      setBotTuningSaved(true);
    } catch (err) {
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
      setBotTuningError(apiErr.message || "Failed to save bot tuning config");
    } finally {
      setBotTuningSaving(false);
    }
  };

  const updateTuning = (
    key: "killAdvantageRatio" | "killClusterMaxSize" | "minAttackWinProbability" | "hogwildArmiesPerTerritory",
    value: number
  ) => {
    setBotTuning((prev) => (prev ? { ...prev, [key]: value } : prev));
  };

  const updatePacing = (key: keyof BotPacingConfig, value: number) => {
    setBotTuning((prev) => (prev ? { ...prev, pacing: { ...prev.pacing, [key]: value } } : prev));
  };

  const [maps, setMaps] = useState<MapSummary[]>([]);
  const [mapDetails, setMapDetails] = useState<Record<string, MapDetail>>({});
  const [mapsLoading, setMapsLoading] = useState(true);
  const [mapsError, setMapsError] = useState("");
  const [busyMapID, setBusyMapID] = useState("");
  const [mapName, setMapName] = useState("");
  const [continentRows, setContinentRows] = useState<ContinentRow[]>([
    { name: "", bonus: 2, territoryCount: 4 },
    { name: "", bonus: 2, territoryCount: 4 },
  ]);
  const [borderRows, setBorderRows] = useState<BorderRow[]>([]);
  const [createMapError, setCreateMapError] = useState("");
  const [creatingMap, setCreatingMap] = useState(false);

  const loadMaps = useCallback(
    async (cancelled = false) => {
      setMapsError("");
      try {
        const out = await listMaps();
        if (cancelled) return;
        setMaps(out);
        const details = await Promise.all(out.map((m) => getMap(m.id)));
        if (cancelled) return;
        const byID: Record<string, MapDetail> = {};
        details.forEach((d) => {
          byID[d.id] = d;
        });
        setMapDetails(byID);
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
        setMapsError(apiErr.message || "Failed to load maps");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setMapsLoading(true);
      await loadMaps(cancelled);
      if (!cancelled) setMapsLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [loadMaps]);

  const addContinentRow = () => {
    setContinentRows((rows) => [...rows, { name: "", bonus: 2, territoryCount: 4 }]);
  };
  const removeContinentRow = (idx: number) => {
    setContinentRows((rows) => rows.filter((_, i) => i !== idx));
  };
  const updateContinentRow = (idx: number, patch: Partial<ContinentRow>) => {
    setContinentRows((rows) => rows.map((row, i) => (i === idx ? { ...row, ...patch } : row)));
  };

  const addBorderRow = () => {
    setBorderRows((rows) => [...rows, { a: "", b: "", crossings: 1 }]);
  };
  const removeBorderRow = (idx: number) => {
    setBorderRows((rows) => rows.filter((_, i) => i !== idx));
  };
  const updateBorderRow = (idx: number, patch: Partial<BorderRow>) => {
    setBorderRows((rows) => rows.map((row, i) => (i === idx ? { ...row, ...patch } : row)));
  };

  const handleCreateMap = async () => {
    setCreateMapError("");
    if (!mapName.trim()) {
      setCreateMapError("Map name is required.");
      return;
    }
    if (continentRows.length < 2 || continentRows.some((c) => !c.name.trim())) {
      setCreateMapError("At least 2 continents are required, each with a name.");
      return;
    }
    if (borderRows.some((b) => !b.a || !b.b)) {
      setCreateMapError("Every border needs both continents selected.");
      return;
    }
    setCreatingMap(true);
    try {
      await createMap({
        name: mapName.trim(),
        continents: continentRows.map((c) => ({ name: c.name.trim(), bonus: c.bonus, territoryCount: c.territoryCount })),
        borders: borderRows.map((b) => ({ a: b.a, b: b.b, crossings: b.crossings })),
      });
      setMapName("");
      setContinentRows([
        { name: "", bonus: 2, territoryCount: 4 },
        { name: "", bonus: 2, territoryCount: 4 },
      ]);
      setBorderRows([]);
      await loadMaps();
    } catch (err) {
      const apiErr = err as ApiError;
      setCreateMapError(apiErr.message || "Failed to create map");
    } finally {
      setCreatingMap(false);
    }
  };

  const handleDeleteMap = async (m: MapSummary) => {
    setBusyMapID(m.id);
    setMapsError("");
    try {
      await deleteMap(m.id);
      await loadMaps();
    } catch (err) {
      const apiErr = err as ApiError;
      setMapsError(apiErr.message || "Failed to delete map");
    } finally {
      setBusyMapID("");
    }
  };

  const patchAccess = async (u: AdminUserRecord, accessStatus: "active" | "blocked") => {
    setBusyUserID(u.id);
    setError("");
    try {
      await updateUserAccess(u.id, accessStatus);
      await load();
    } catch (err) {
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
      setError(apiErr.message || "Failed to update user access");
    } finally {
      setBusyUserID("");
    }
  };

  const onToggleSandbox = async (u: AdminUserRecord) => {
    setBusyUserID(u.id);
    setError("");
    try {
      await updateUserSandbox(u.id, !u.isSandboxed);
      await load();
    } catch (err) {
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
      setError(apiErr.message || "Failed to update sandbox status");
    } finally {
      setBusyUserID("");
    }
  };

  const onRevokeSessions = async (u: AdminUserRecord) => {
    setBusyUserID(u.id);
    setError("");
    try {
      await revokeUserSessions(u.id);
      await load();
    } catch (err) {
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
      setError(apiErr.message || "Failed to revoke sessions");
    } finally {
      setBusyUserID("");
    }
  };

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-gc-text">Admin</h2>
          <p className="mt-0.5 text-sm text-gc-muted">Manage user access and sessions.</p>
        </div>
        <button
          className={buttonGhostClass}
          type="button"
          onClick={() => void load()}
          disabled={loading || !!busyUserID}
        >
          Refresh
        </button>
      </div>

      {loading ? <p className="text-sm text-gc-muted">Loading users…</p> : null}
      {error ? (
        <p className="mb-4 rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
          {error}
        </p>
      ) : null}

      <section className="overflow-x-auto rounded-xl border border-gc-border bg-gc-surface">
        <table className="min-w-full divide-y divide-gc-border text-sm">
          <thead className="bg-gc-surface-2">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">User</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Role</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Status</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Sandbox</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Sessions</th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gc-border/60">
            {users.map((u) => {
              const isBusy = busyUserID === u.id;
              const isBlocked = u.accessStatus === "blocked";
              return (
                <tr key={u.id} className="transition-colors hover:bg-gc-surface-2">
                  <td className="px-4 py-3">
                    <p className="font-medium text-gc-text">{u.username}</p>
                    <p className="font-mono text-[11px] text-gc-muted">{u.id}</p>
                  </td>
                  <td className="px-4 py-3 capitalize text-gc-muted">{u.role}</td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium ${
                        isBlocked
                          ? "border border-gc-danger/40 bg-gc-danger/10 text-gc-danger"
                          : "border border-gc-success/40 bg-gc-success/10 text-gc-success"
                      }`}
                    >
                      {isBlocked ? "Blocked" : "Active"}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    {u.isSandboxed ? (
                      <span className="inline-flex rounded-full border border-gc-accent/40 bg-gc-accent/10 px-2 py-0.5 text-[11px] font-medium text-gc-accent">
                        Sandboxed
                      </span>
                    ) : (
                      <span className="text-gc-muted">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-gc-muted">{u.activeSessions}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      {isBlocked ? (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "active")}
                        >
                          Unblock
                        </button>
                      ) : (
                        <button
                          className={buttonGhostClass}
                          type="button"
                          disabled={isBusy}
                          onClick={() => void patchAccess(u, "blocked")}
                        >
                          Block
                        </button>
                      )}
                      <button
                        className={buttonGhostClass}
                        type="button"
                        disabled={isBusy}
                        onClick={() => void onToggleSandbox(u)}
                      >
                        {u.isSandboxed ? "Unsandbox" : "Sandbox"}
                      </button>
                      <button
                        className={buttonPrimaryClass}
                        type="button"
                        disabled={isBusy}
                        onClick={() => void onRevokeSessions(u)}
                      >
                        Revoke Sessions
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>

      <section className="mt-5 rounded-xl border border-gc-border bg-gc-surface p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold text-gc-text">Bot Tuning</h3>
            <p className="mt-0.5 text-xs text-gc-muted">
              Applies to new bot-inclusive games only — already-created games keep whatever was configured when they
              started.
            </p>
          </div>
          <button
            className={buttonGhostClass}
            type="button"
            onClick={() => void loadBotTuning()}
            disabled={botTuningLoading || botTuningSaving}
          >
            Refresh
          </button>
        </div>

        {botTuningLoading ? <p className="text-sm text-gc-muted">Loading…</p> : null}
        {botTuningError ? (
          <p className="mb-3 rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {botTuningError}
          </p>
        ) : null}

        {botTuning ? (
          <>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <label className="grid gap-1.5 text-xs font-medium text-gc-muted">
                Kill advantage ratio
                <input
                  className={inputClass}
                  type="number"
                  step="0.1"
                  min="0"
                  value={botTuning.killAdvantageRatio}
                  onChange={(e) => updateTuning("killAdvantageRatio", Number(e.target.value))}
                />
              </label>
              <label className="grid gap-1.5 text-xs font-medium text-gc-muted">
                Kill cluster max size
                <input
                  className={inputClass}
                  type="number"
                  step="1"
                  min="1"
                  value={botTuning.killClusterMaxSize}
                  onChange={(e) => updateTuning("killClusterMaxSize", Number(e.target.value))}
                />
              </label>
              <label className="grid gap-1.5 text-xs font-medium text-gc-muted">
                Min attack win probability
                <input
                  className={inputClass}
                  type="number"
                  step="0.05"
                  min="0"
                  max="1"
                  value={botTuning.minAttackWinProbability}
                  onChange={(e) => updateTuning("minAttackWinProbability", Number(e.target.value))}
                />
              </label>
              <label className="grid gap-1.5 text-xs font-medium text-gc-muted">
                Hogwild armies/territory
                <input
                  className={inputClass}
                  type="number"
                  step="1"
                  min="1"
                  value={botTuning.hogwildArmiesPerTerritory}
                  onChange={(e) => updateTuning("hogwildArmiesPerTerritory", Number(e.target.value))}
                />
              </label>
            </div>

            <h4 className="mt-4 text-xs font-semibold uppercase tracking-wide text-gc-muted">
              Pacing (live games only, ms)
            </h4>
            <div className="mt-2 overflow-x-auto rounded-lg border border-gc-border">
              <table className="min-w-full divide-y divide-gc-border text-sm">
                <thead className="bg-gc-surface-2">
                  <tr>
                    <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">
                      Action
                    </th>
                    <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">
                      Min ms
                    </th>
                    <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gc-muted">
                      Max ms
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gc-border/60">
                  {PACING_ROWS.map((row) => (
                    <tr key={row.label}>
                      <td className="px-3 py-2 text-gc-text">{row.label}</td>
                      <td className="px-3 py-2">
                        <input
                          className={`${inputClass} w-24`}
                          type="number"
                          step="50"
                          min="0"
                          value={botTuning.pacing[row.minKey]}
                          onChange={(e) => updatePacing(row.minKey, Number(e.target.value))}
                        />
                      </td>
                      <td className="px-3 py-2">
                        <input
                          className={`${inputClass} w-24`}
                          type="number"
                          step="50"
                          min="0"
                          value={botTuning.pacing[row.maxKey]}
                          onChange={(e) => updatePacing(row.maxKey, Number(e.target.value))}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="mt-4 flex items-center gap-3">
              <button
                className={buttonPrimaryClass}
                type="button"
                onClick={() => void onSaveBotTuning()}
                disabled={botTuningSaving}
              >
                {botTuningSaving ? "Saving…" : "Save"}
              </button>
              {botTuningSaved ? <span className="text-sm text-gc-success">Saved.</span> : null}
            </div>
          </>
        ) : null}
      </section>

      <div className="mb-5 mt-10 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-gc-text">Custom Maps</h2>
          <p className="mt-0.5 text-sm text-gc-muted">
            Draw a board by hand in the map editor, or generate one below from continents, per-continent territory
            counts and army bonuses, and how many crossings connect each bordering pair of continents.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            className={buttonGhostClass}
            type="button"
            onClick={() => void loadMaps()}
            disabled={mapsLoading || !!busyMapID}
          >
            Refresh
          </button>
          <Link to="/app/admin/maps/new" className={buttonPrimaryClass}>
            Draw a map
          </Link>
        </div>
      </div>

      <section className="mb-6 rounded-xl border border-gc-border bg-gc-surface p-4">
        <div className="mb-4">
          <label className="mb-1 block text-xs font-semibold uppercase tracking-wide text-gc-muted">Map name</label>
          <input
            className={inputClass}
            type="text"
            value={mapName}
            onChange={(e) => setMapName(e.target.value)}
            placeholder="e.g. Twin Continents"
          />
        </div>

        <div className="mb-4">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wide text-gc-muted">Continents</span>
            <button className={buttonGhostClass} type="button" onClick={addContinentRow}>
              + Add continent
            </button>
          </div>
          <div className="flex flex-col gap-2">
            {continentRows.map((row, idx) => (
              <div key={idx} className="flex flex-wrap items-center gap-2">
                <input
                  className={`${inputClass} w-40`}
                  type="text"
                  placeholder="Name"
                  value={row.name}
                  onChange={(e) => updateContinentRow(idx, { name: e.target.value })}
                />
                <label className="flex items-center gap-1 text-xs text-gc-muted">
                  Bonus
                  <input
                    className={`${inputClass} w-16`}
                    type="number"
                    min={0}
                    value={row.bonus}
                    onChange={(e) => updateContinentRow(idx, { bonus: Number(e.target.value) })}
                  />
                </label>
                <label className="flex items-center gap-1 text-xs text-gc-muted">
                  Territories
                  <input
                    className={`${inputClass} w-16`}
                    type="number"
                    min={1}
                    value={row.territoryCount}
                    onChange={(e) => updateContinentRow(idx, { territoryCount: Number(e.target.value) })}
                  />
                </label>
                <button
                  className={buttonGhostClass}
                  type="button"
                  onClick={() => removeContinentRow(idx)}
                  disabled={continentRows.length <= 2}
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        </div>

        <div className="mb-4">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wide text-gc-muted">Borders</span>
            <button className={buttonGhostClass} type="button" onClick={addBorderRow}>
              + Add border
            </button>
          </div>
          <div className="flex flex-col gap-2">
            {borderRows.map((row, idx) => (
              <div key={idx} className="flex flex-wrap items-center gap-2">
                <select
                  className={`${inputClass} w-36`}
                  value={row.a}
                  onChange={(e) => updateBorderRow(idx, { a: e.target.value })}
                >
                  <option value="">Continent A</option>
                  {continentRows
                    .filter((c) => c.name.trim())
                    .map((c) => (
                      <option key={c.name} value={c.name}>
                        {c.name}
                      </option>
                    ))}
                </select>
                <select
                  className={`${inputClass} w-36`}
                  value={row.b}
                  onChange={(e) => updateBorderRow(idx, { b: e.target.value })}
                >
                  <option value="">Continent B</option>
                  {continentRows
                    .filter((c) => c.name.trim())
                    .map((c) => (
                      <option key={c.name} value={c.name}>
                        {c.name}
                      </option>
                    ))}
                </select>
                <label className="flex items-center gap-1 text-xs text-gc-muted">
                  Crossings
                  <input
                    className={`${inputClass} w-16`}
                    type="number"
                    min={1}
                    value={row.crossings}
                    onChange={(e) => updateBorderRow(idx, { crossings: Number(e.target.value) })}
                  />
                </label>
                <button className={buttonGhostClass} type="button" onClick={() => removeBorderRow(idx)}>
                  Remove
                </button>
              </div>
            ))}
            {borderRows.length === 0 ? <p className="text-xs text-gc-muted">No borders yet — continents must be connected.</p> : null}
          </div>
        </div>

        {createMapError ? (
          <p className="mb-3 rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
            {createMapError}
          </p>
        ) : null}
        <button className={buttonPrimaryClass} type="button" onClick={() => void handleCreateMap()} disabled={creatingMap}>
          {creatingMap ? "Generating…" : "Generate map"}
        </button>
      </section>

      {mapsLoading ? <p className="text-sm text-gc-muted">Loading maps…</p> : null}
      {mapsError ? (
        <p className="mb-4 rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">
          {mapsError}
        </p>
      ) : null}

      <section className="flex flex-col gap-3">
        {maps.map((m) => {
          const detail = mapDetails[m.id];
          const isBusy = busyMapID === m.id;
          return (
            <div
              key={m.id}
              className="flex flex-wrap items-center gap-4 rounded-xl border border-gc-border bg-gc-surface p-4"
            >
              {detail ? <MapPreview detail={detail} /> : <div className="h-32 w-32 shrink-0 rounded-lg bg-gc-surface-2" />}
              <div className="flex-1">
                <p className="font-medium text-gc-text">{m.name}</p>
                <p className="text-sm text-gc-muted">
                  {m.continentCount} continents, {m.territoryCount} territories
                </p>
                <p className="font-mono text-[11px] text-gc-muted">{m.id}</p>
              </div>
              <Link to="/app/admin/maps/$mapID" params={{ mapID: m.id }} className={buttonGhostClass}>
                Open editor
              </Link>
              <button className={buttonGhostClass} type="button" disabled={isBusy} onClick={() => void handleDeleteMap(m)}>
                {isBusy ? "Deleting…" : "Delete"}
              </button>
            </div>
          );
        })}
        {!mapsLoading && maps.length === 0 ? <p className="text-sm text-gc-muted">No custom maps yet.</p> : null}
      </section>
    </div>
  );
}
