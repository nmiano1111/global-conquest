/**
 * The visually distinct states a territory node can render in. Deliberately
 * a closed set (not a boolean) so selection, legality affordance, and
 * combat/capture recency can all be told apart without relying on player
 * color alone.
 */
export type TerritoryHighlightKind =
  | "none"
  | "selected-source"
  | "selected-target"
  | "legal-target"
  | "recent-combat"
  | "recent-capture"
  | "passive";

export interface TerritoryDisplayState {
  /** Player index, or -1 if unclaimed */
  owner: number;
  armies: number;
  highlight: TerritoryHighlightKind;
}
