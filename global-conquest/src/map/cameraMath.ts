// src/map/cameraMath.ts
//
// Pure camera/gesture math extracted out of MapScene so it can be unit
// tested without a Pixi/WebGL context. MapScene owns all mutable camera
// state (camScale/camX/camY) and calls these as pure functions.

export interface CameraState {
  scale: number;
  x: number;
  y: number;
}

export interface Viewport {
  width: number;
  height: number;
}

export interface WorldSize {
  width: number;
  height: number;
}

/** Scale at which the full world exactly fits inside the viewport ("contain" — nothing cropped, may letterbox). */
export function computeMinZoom(viewport: Viewport, world: WorldSize): number {
  if (world.width <= 0 || world.height <= 0) return 1;
  return Math.min(viewport.width / world.width, viewport.height / world.height);
}

/** Scale at which the world fills the viewport with no letterboxing ("cover" — edges may crop off-screen). */
export function computeCoverZoom(viewport: Viewport, world: WorldSize): number {
  if (world.width <= 0 || world.height <= 0) return 1;
  return Math.max(viewport.width / world.width, viewport.height / world.height);
}

/**
 * Clamp camera x/y so the world never exposes empty space beyond its edges.
 * When the world fits inside the viewport on an axis, center it on that axis.
 */
export function clampCameraPosition(
  cam: CameraState,
  viewport: Viewport,
  world: WorldSize,
): CameraState {
  const ww = world.width * cam.scale;
  const wh = world.height * cam.scale;
  let x = cam.x;
  let y = cam.y;

  if (ww <= viewport.width) {
    x = (viewport.width - ww) / 2;
  } else {
    x = Math.min(0, Math.max(viewport.width - ww, x));
  }

  if (wh <= viewport.height) {
    y = (viewport.height - wh) / 2;
  } else {
    y = Math.min(0, Math.max(viewport.height - wh, y));
  }

  return { scale: cam.scale, x, y };
}

/**
 * Zoom to newScale while keeping the screen-space point (pivotX, pivotY)
 * fixed over the same world point — i.e. zoom toward the cursor / pinch
 * midpoint / double-tap point. Returns the clamped, position-corrected
 * camera state; does not mutate its input.
 */
export function zoomToward(
  cam: CameraState,
  newScale: number,
  pivotX: number,
  pivotY: number,
  minZoom: number,
  maxZoom: number,
  viewport: Viewport,
  world: WorldSize,
): CameraState {
  const clampedScale = Math.max(minZoom, Math.min(maxZoom, newScale));
  const worldX = (pivotX - cam.x) / cam.scale;
  const worldY = (pivotY - cam.y) / cam.scale;
  const next: CameraState = {
    scale: clampedScale,
    x: pivotX - worldX * clampedScale,
    y: pivotY - worldY * clampedScale,
  };
  return clampCameraPosition(next, viewport, world);
}

/**
 * Camera that centers the world in the viewport at either the "contain"
 * scale (whole world visible, may letterbox) or the "cover" scale (fills
 * the viewport edge to edge, may crop world edges off-screen). Callers
 * that want a cover fit clamped to some maximum zoom should clamp the
 * returned scale themselves — this returns the raw fit scale.
 */
export function fitToViewport(
  viewport: Viewport,
  world: WorldSize,
  mode: "contain" | "cover" = "contain",
): CameraState {
  const scale = mode === "cover" ? computeCoverZoom(viewport, world) : computeMinZoom(viewport, world);
  return {
    scale,
    x: (viewport.width - world.width * scale) / 2,
    y: (viewport.height - world.height * scale) / 2,
  };
}

/** Euclidean distance between two screen points — used for pinch/drag math. */
export function distance(ax: number, ay: number, bx: number, by: number): number {
  return Math.hypot(bx - ax, by - ay);
}

/**
 * A pointer has "dragged" once it has moved more than thresholdPx from its
 * down position. Used to distinguish a tap (select a territory / toggle
 * fullscreen) from the start of a pan, so panning never accidentally fires
 * a territory click.
 */
export function exceedsDragThreshold(
  startX: number,
  startY: number,
  currentX: number,
  currentY: number,
  thresholdPx = 6,
): boolean {
  return distance(startX, startY, currentX, currentY) > thresholdPx;
}

export interface TapRecord {
  x: number;
  y: number;
  timeMs: number;
}

/**
 * Determines whether `current` forms a double-tap/double-click with
 * `previous` — same general area, within the time window. Both thresholds
 * are generous enough for touch imprecision while still requiring genuine
 * intent (a real double-tap, not two incidental taps).
 */
export function isDoubleTap(
  previous: TapRecord | null,
  current: TapRecord,
  maxIntervalMs = 300,
  maxDistancePx = 32,
): boolean {
  if (!previous) return false;
  if (current.timeMs - previous.timeMs > maxIntervalMs) return false;
  return distance(previous.x, previous.y, current.x, current.y) <= maxDistancePx;
}

// ---------------------------------------------------------------------------
// Momentum glide (inertial panning after a drag release)
// ---------------------------------------------------------------------------

export interface Velocity {
  /** Pixels per millisecond. */
  vx: number;
  vy: number;
}

export interface PanSample {
  x: number;
  y: number;
  timeMs: number;
}

export function velocityMagnitude(v: Velocity): number {
  return Math.hypot(v.vx, v.vy);
}

/**
 * Estimates release velocity (px/ms) from a short trailing history of pan
 * samples — the earliest sample still within windowMs of the most recent
 * one, compared against that most recent one. Using a short trailing
 * window (rather than the whole drag) means a flick that decelerates
 * before release doesn't inherit speed from earlier in the gesture.
 * Returns null when there's not enough history to estimate from.
 */
export function computeReleaseVelocity(
  samples: readonly PanSample[],
  windowMs = 100,
): Velocity | null {
  if (samples.length < 2) return null;
  const last = samples[samples.length - 1];
  let first = last;
  for (const s of samples) {
    if (last.timeMs - s.timeMs <= windowMs) {
      first = s;
      break;
    }
  }
  const dt = last.timeMs - first.timeMs;
  if (dt <= 0) return null;
  return { vx: (last.x - first.x) / dt, vy: (last.y - first.y) / dt };
}

/**
 * Advances the camera by one momentum-glide step: moves position by
 * velocity × dt, decays velocity by friction (normalized to a ~60fps
 * frame so it feels consistent regardless of actual frame rate), and
 * clamps position to bounds. Velocity is zeroed on any axis that just hit
 * a bound, so the glide stops cleanly at the map's edge instead of
 * jittering against it.
 */
export function stepMomentum(
  cam: CameraState,
  velocity: Velocity,
  dtMs: number,
  friction: number,
  viewport: Viewport,
  world: WorldSize,
): { camera: CameraState; velocity: Velocity } {
  const rawX = cam.x + velocity.vx * dtMs;
  const rawY = cam.y + velocity.vy * dtMs;
  const clamped = clampCameraPosition({ scale: cam.scale, x: rawX, y: rawY }, viewport, world);
  const decay = Math.pow(friction, dtMs / (1000 / 60));
  return {
    camera: clamped,
    velocity: {
      vx: clamped.x !== rawX ? 0 : velocity.vx * decay,
      vy: clamped.y !== rawY ? 0 : velocity.vy * decay,
    },
  };
}
