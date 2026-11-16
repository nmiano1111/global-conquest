// Territory node geometry, shared by the Pixi node and pure layout code
// (kept free of Pixi imports so layout math stays unit-testable).

export const TERRITORY_RADIUS = 42;

/**
 * Furthest a node draws from its center, in world units: the largest
 * highlight ring (R + 9 + 4 pulse + its stroke) at the largest highlight
 * scale (1.12). Layout code keeps at least this much room around nodes.
 */
export const TERRITORY_MAX_EXTENT = Math.ceil((TERRITORY_RADIUS + 9 + 4 + 4) * 1.12);
