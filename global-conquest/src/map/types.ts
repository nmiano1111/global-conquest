export interface TerritoryDisplayState {
  /** Player index, or -1 if unclaimed */
  owner: number;
  armies: number;
  isSelected: boolean;
}
