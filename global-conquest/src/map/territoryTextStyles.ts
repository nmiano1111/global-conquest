import { TextStyle } from "pixi.js";

/**
 * Shared text styles for territory army/name labels — used identically by
 * both TerritoryNode (circle) and TerritoryPolygon (shape), so switching a
 * territory from one to the other never changes how its text looks.
 */

// Army count: large, centered. A solid dark stroke outline is far more readable
// than a drop shadow alone — it guarantees contrast against any player color.
export const ARMY_TEXT_STYLE = new TextStyle({
  fill: "#ffffff",
  fontSize: 32,
  fontWeight: "bold",
  align: "center",
  stroke: { color: "#000000", width: 6 },
});

// Name label: white, bold stroke outline so it reads on any fill color.
export const NAME_TEXT_STYLE = new TextStyle({
  fill: "#ffffff",
  fontSize: 16,
  fontWeight: "800",
  align: "center",
  stroke: { color: "rgba(0,0,0,0.75)", width: 3.5 },
  dropShadow: { color: "#000000", blur: 3, distance: 1, alpha: 0.4 },
});
