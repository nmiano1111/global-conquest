import type { TerritoryHighlightKind } from "./types";

/** Fill used for a territory with no owner (owner index -1). */
export const UNOWNED_FILL = "#e2e8f0";

/**
 * Pointer-hover ring, shared by TerritoryNode and TerritoryPolygon. Hover is
 * purely local Pixi pointer state (never round-tripped through React props —
 * see the "Performance" note in territoryShapes.ts), so it is layered on top
 * of whichever TerritoryHighlightKind is already showing rather than being a
 * member of that closed set.
 */
export const HOVER_RING_STYLE = {
  color: 0xffffff,
  ringWidth: 2,
  fillAlpha: 0.12,
};

/**
 * Per-highlight-kind ring styling, shared by the legacy circular
 * TerritoryNode and the polygon-based TerritoryPolygon. Each kind combines a
 * distinct color and ring width so states remain distinguishable without
 * relying on color alone, and stay legible for color-blind users.
 *
 * `fillAlpha` and `scale` are circle-only: TerritoryNode uses `fillAlpha`
 * for its halo ring (drawn outside the token) and `scale` to pop the whole
 * token up slightly on selection. TerritoryPolygon ignores both — an
 * outward halo would need true polygon offsetting to avoid looking
 * distorted on concave shapes, and scaling a geographically-aligned shape
 * would visibly detach it from the map artwork it overlays. Both classes
 * apply the same border color/width from this table either way.
 */
export const HIGHLIGHT_STYLE: Record<
  Exclude<TerritoryHighlightKind, "none">,
  { color: number; ringWidth: number; fillAlpha: number; pulse: boolean; scale: number }
> = {
  "selected-source": { color: 0xfbbf24, ringWidth: 4, fillAlpha: 0.24, pulse: false, scale: 1.12 },
  "selected-target": { color: 0xf43f5e, ringWidth: 4, fillAlpha: 0.22, pulse: false, scale: 1.12 },
  "legal-target": { color: 0x38bdf8, ringWidth: 2.5, fillAlpha: 0.14, pulse: true, scale: 1.0 },
  "recent-combat": { color: 0xf97316, ringWidth: 3, fillAlpha: 0.18, pulse: true, scale: 1.0 },
  "recent-capture": { color: 0x34d399, ringWidth: 3.5, fillAlpha: 0.22, pulse: true, scale: 1.06 },
  passive: { color: 0x818cf8, ringWidth: 2.5, fillAlpha: 0.14, pulse: false, scale: 1.0 },
};
