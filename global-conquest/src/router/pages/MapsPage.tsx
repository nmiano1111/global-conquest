import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ApiError } from "../../api/client";
import { canModifyMap, deleteMap, getMap, listMaps, type MapDetail, type MapSummary } from "../../api/maps";
import { useAuth } from "../../auth";
import { MAP_PLAYER_COLORS } from "./gameShared";
import { buttonGhostClass, buttonPrimaryClass } from "./styles";

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

// Every signed-in user's map library: their own maps (editable) and
// everyone else's (viewable, playable, and copyable from the editor).
export function MapsPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [maps, setMaps] = useState<MapSummary[]>([]);
  const [mapDetails, setMapDetails] = useState<Record<string, MapDetail>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busyMapID, setBusyMapID] = useState("");

  const loadMaps = useCallback(
    async (cancelled = false) => {
      setError("");
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
        setError(apiErr.message || "Failed to load maps");
      }
    },
    [auth, navigate]
  );

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setLoading(true);
      await loadMaps(cancelled);
      if (!cancelled) setLoading(false);
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [loadMaps]);

  const handleDeleteMap = async (m: MapSummary) => {
    setBusyMapID(m.id);
    setError("");
    try {
      await deleteMap(m.id);
      await loadMaps();
    } catch (err) {
      const apiErr = err as ApiError;
      setError(apiErr.status === 409 ? `"${m.name}" is used by a game, so it can't be deleted.` : apiErr.message || "Failed to delete map");
    } finally {
      setBusyMapID("");
    }
  };

  const mine = maps.filter((m) => m.ownerUserId === auth.user?.id);
  const others = maps.filter((m) => m.ownerUserId !== auth.user?.id);

  const renderMap = (m: MapSummary) => {
    const detail = mapDetails[m.id];
    const isBusy = busyMapID === m.id;
    const editable = canModifyMap(m, auth.user);
    return (
      <div key={m.id} className="flex flex-wrap items-center gap-4 rounded-xl border border-gc-border bg-gc-surface p-4">
        {detail ? <MapPreview detail={detail} /> : <div className="h-32 w-32 shrink-0 rounded-lg bg-gc-surface-2" />}
        <div className="flex-1">
          <p className="font-medium text-gc-text">{m.name}</p>
          <p className="text-sm text-gc-muted">
            {m.continentCount} continents, {m.territoryCount} territories
          </p>
          {m.ownerUserId !== auth.user?.id && m.ownerUserName ? (
            <p className="text-xs text-gc-muted">by {m.ownerUserName}</p>
          ) : null}
          {detail && detail.gameCount > 0 ? (
            <p className="text-xs text-gc-muted">
              Used by {detail.gameCount} game{detail.gameCount === 1 ? "" : "s"}
            </p>
          ) : null}
        </div>
        <Link to="/app/maps/$mapID" params={{ mapID: m.id }} className={buttonGhostClass}>
          {editable ? "Open editor" : "View"}
        </Link>
        {editable ? (
          <button className={buttonGhostClass} type="button" disabled={isBusy} onClick={() => void handleDeleteMap(m)}>
            {isBusy ? "Deleting…" : "Delete"}
          </button>
        ) : null}
      </div>
    );
  };

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-gc-text">Maps</h2>
          <p className="mt-0.5 text-sm text-gc-muted">
            Draw a board by hand, or generate one from continents, territory counts, bonuses and how many crossings
            join each bordering pair. Both open in the map editor, where you can adjust the board before saving. Any
            saved map can be picked when creating a game in the lobby.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button className={buttonGhostClass} type="button" onClick={() => void loadMaps()} disabled={loading || !!busyMapID}>
            Refresh
          </button>
          <Link to="/app/maps/new" search={{ generate: true }} className={buttonGhostClass}>
            Generate a map
          </Link>
          <Link to="/app/maps/new" className={buttonPrimaryClass}>
            Draw a map
          </Link>
        </div>
      </div>

      {loading ? <p className="text-sm text-gc-muted">Loading maps…</p> : null}
      {error ? (
        <p className="mb-4 rounded-lg border border-gc-danger/30 bg-gc-danger/10 px-3 py-2 text-sm text-gc-danger">{error}</p>
      ) : null}

      {!loading ? (
        <>
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-gc-muted">Your maps</h3>
          <section className="flex flex-col gap-3">
            {mine.map(renderMap)}
            {mine.length === 0 ? <p className="text-sm text-gc-muted">You haven't made any maps yet.</p> : null}
          </section>

          <h3 className="mb-3 mt-8 text-xs font-semibold uppercase tracking-wide text-gc-muted">Other players' maps</h3>
          <section className="flex flex-col gap-3">
            {others.map(renderMap)}
            {others.length === 0 ? <p className="text-sm text-gc-muted">No one else has made a map yet.</p> : null}
          </section>
        </>
      ) : null}
    </div>
  );
}
