import { useState } from "react";
import { MAP_TERRITORIES } from "../router/pages/gameShared";
import type { MapEditorSnapshot } from "./mapEditor";
import type { MapScene } from "./MapScene";

const TERRITORY_NAMES = Object.keys(MAP_TERRITORIES);

/**
 * Dev-only control panel for MapScene's polygon tracing tool. Rendered by
 * GameMap only when both import.meta.env.DEV and ?mapEditor=true hold — see
 * MAP_EDITOR.md for the full tracing workflow this panel drives.
 */
export function MapEditorPanel({
  scene,
  snapshot,
}: {
  scene: MapScene | null;
  snapshot: MapEditorSnapshot | null;
}) {
  const [copied, setCopied] = useState(false);

  const handleExport = async () => {
    const source = scene?.editorExport() ?? "";
    try {
      await navigator.clipboard.writeText(source);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard permission can be denied in some dev contexts — fall back
      // to a console dump so the data is never lost.
      console.log(source);
    }
  };

  return (
    <div
      style={{
        position: "absolute",
        top: 8,
        right: 8,
        zIndex: 20,
        width: 260,
        background: "rgba(15, 23, 42, 0.92)",
        color: "#e2e8f0",
        border: "1px solid rgba(148, 163, 184, 0.4)",
        borderRadius: 8,
        padding: 12,
        fontSize: 12,
        fontFamily: "monospace",
        display: "flex",
        flexDirection: "column",
        gap: 8,
      }}
    >
      <div style={{ fontWeight: "bold", fontSize: 13 }}>Map Editor (dev only)</div>

      <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        Territory
        <select
          defaultValue=""
          onChange={(e) => e.target.value && scene?.editorSelectTerritory(e.target.value)}
          style={{ background: "#1e293b", color: "#e2e8f0", padding: 4, borderRadius: 4 }}
        >
          <option value="" disabled>
            Select territory…
          </option>
          {TERRITORY_NAMES.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </label>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
        <button onClick={() => scene?.editorStartNewPolygon()}>New polygon</button>
        <button onClick={() => scene?.editorUndo()}>Undo point</button>
        <button onClick={() => scene?.editorClear()}>Clear territory</button>
        <button
          onClick={() => scene?.editorSetLabelMode(!snapshot?.labelMode)}
          style={{ fontWeight: snapshot?.labelMode ? "bold" : "normal" }}
        >
          {snapshot?.labelMode ? "Click map to set label…" : "Set label position"}
        </button>
      </div>

      <div style={{ opacity: 0.8 }}>
        {snapshot?.territory ? (
          <>
            <div>Territory: {snapshot.territory}</div>
            <div>Polygons: {snapshot.polygonCount}</div>
            <div>Points in current polygon: {snapshot.currentPolygonPointCount}</div>
            <div>Label set: {snapshot.hasLabel ? "yes" : "no"}</div>
          </>
        ) : (
          <div>Select a territory, then click the map to add vertices.</div>
        )}
      </div>

      <button onClick={handleExport} style={{ fontWeight: "bold" }}>
        {copied ? "Copied!" : "Export all shapes → clipboard"}
      </button>
    </div>
  );
}
