import { Container, Graphics, Polygon, Text } from "pixi.js";
import { HIGHLIGHT_STYLE, HOVER_RING_STYLE, UNOWNED_FILL } from "./highlightStyles";
import { MAP_OVERLAY_SCALE } from "../router/pages/gameShared";
import { denormalizePoint, denormalizePolygons, type Point, type TerritoryShape } from "./territoryShapes";
import { ARMY_TEXT_STYLE, NAME_TEXT_STYLE } from "./territoryTextStyles";
import type { TerritoryDisplayState, TerritoryHighlightKind } from "./types";

/**
 * Fill alpha matches TerritoryNode's main token body exactly (see
 * TerritoryNode.drawCircle, step 3) — a shaped territory should read as the
 * same solid owner color as the circle it replaces, not a translucent tint.
 * Highlight/hover states only change the border, never this fill, exactly
 * as they do for the circle.
 */
const FILL_ALPHA = 0.95;

/**
 * Territory-shaped interactive overlay: one Graphics per disconnected
 * polygon (all sharing the same click/hover handlers so multi-polygon
 * territories like islands select as a single unit), plus an army-count +
 * name label pair styled and positioned identically to TerritoryNode's.
 *
 * Deliberately reuses TerritoryNode's fill alpha, border logic, and text
 * styles verbatim — the only intended difference between the two is the
 * hit/visual geometry (this territory's actual traced shape vs. a fixed
 * circle), not color, text, or interaction behavior. Two purely
 * shape-driven exceptions, called out where they occur below: no
 * halo/bevel/gloss chrome (those are token-specific effects that don't
 * generalize to arbitrary polygons without distorting the shape), and no
 * selection scale-up (scaling a geographically-aligned shape would visibly
 * detach it from the map artwork it's meant to overlay).
 */
export class TerritoryPolygon extends Container {
  readonly territoryId: string;
  /** World-space (worldContainer-local) point used as this territory's visual center for labels and connector lines. */
  readonly anchorPoint: Point;

  private readonly polygonPoints: Point[][];
  private readonly polygonGfx: Graphics[] = [];
  private readonly armyLabel: Text;

  private currentFill = UNOWNED_FILL;
  private currentHighlight: TerritoryHighlightKind = "none";
  private hovering = false;

  constructor(shape: TerritoryShape, onClick: (name: string) => void) {
    super();
    this.territoryId = shape.territoryId;
    this.polygonPoints = denormalizePolygons(shape.polygons);
    this.anchorPoint = denormalizePoint(shape.labelPosition);

    for (const points of this.polygonPoints) {
      const g = new Graphics();
      g.eventMode = "static";
      g.cursor = "pointer";
      g.hitArea = new Polygon(points);
      g.on("pointertap", () => onClick(this.territoryId));
      g.on("pointerover", () => {
        this.hovering = true;
        this.redraw(0);
      });
      g.on("pointerout", () => {
        this.hovering = false;
        this.redraw(0);
      });
      this.polygonGfx.push(g);
      this.addChild(g);
    }

    // Labels — same styles, same relative offsets as TerritoryNode, just
    // anchored at the shape's configured label position instead of a
    // circle center. Scaled up by MAP_OVERLAY_SCALE to visually match
    // TerritoryNode's text size: legacy circles live inside overlayContainer,
    // which carries that extra static scale on top of the camera zoom (a
    // leftover from aligning old SVG-space dot positions onto risk0.png);
    // this container lives directly in worldContainer (correctly, at true
    // PNG-pixel scale, no compensating transform needed for the polygon
    // geometry itself) and so would otherwise render its labels ~26%
    // smaller than the circle's at the same camera zoom.
    const labelContainer = new Container();
    labelContainer.eventMode = "none";
    labelContainer.position.set(this.anchorPoint.x, this.anchorPoint.y);
    labelContainer.scale.set(MAP_OVERLAY_SCALE);

    this.armyLabel = new Text({ text: "0", style: ARMY_TEXT_STYLE });
    this.armyLabel.eventMode = "none";
    this.armyLabel.anchor.set(0.5, 0.5);
    this.armyLabel.position.set(0, -3);
    labelContainer.addChild(this.armyLabel);

    const nameLabel = new Text({ text: shape.territoryId, style: NAME_TEXT_STYLE });
    nameLabel.eventMode = "none";
    nameLabel.anchor.set(0.5, 0);
    nameLabel.position.set(0, 20);
    labelContainer.addChild(nameLabel);

    this.addChild(labelContainer);

    this.redraw(0);
  }

  update(state: TerritoryDisplayState, playerColors: string[]) {
    this.currentFill = state.owner >= 0 ? (playerColors[state.owner] ?? UNOWNED_FILL) : UNOWNED_FILL;
    this.currentHighlight = state.highlight;
    this.armyLabel.text = String(state.armies);
    this.redraw(0);
  }

  /** Advances the pulse animation for highlight kinds that pulse. No-op (and never called) when reduced-motion is active. */
  tickPulse(phase: number) {
    const style = this.currentHighlight === "none" ? null : HIGHLIGHT_STYLE[this.currentHighlight];
    if (!style?.pulse) return;
    this.redraw(phase);
  }

  private redraw(pulsePhase: number) {
    const style = this.currentHighlight === "none" ? null : HIGHLIGHT_STYLE[this.currentHighlight];

    let borderColor = 0x0f172a;
    let borderWidth = 2.5;
    if (style) {
      // Matches TerritoryNode's main-body stroke exactly (ringWidth - 0.5),
      // then layers the same gentle pulse TerritoryNode applies to its halo
      // ring onto the border width here instead (polygons skip the halo
      // itself — see class doc comment).
      borderColor = style.color;
      borderWidth = style.ringWidth - 0.5;
      if (style.pulse) {
        const pulseT = (Math.sin(pulsePhase) + 1) / 2; // 0..1
        borderWidth += pulseT * 1.5;
      }
    }

    // Hover layers on top of whatever the logical highlight kind already
    // set, rather than replacing it — e.g. a legal-target territory still
    // shows its blue pulsing border while hovered, just thicker/brighter.
    if (this.hovering) {
      borderWidth = Math.max(borderWidth, HOVER_RING_STYLE.ringWidth) + 0.75;
      if (!style) borderColor = HOVER_RING_STYLE.color;
    }

    for (let i = 0; i < this.polygonGfx.length; i++) {
      const points = this.polygonPoints[i];
      const g = this.polygonGfx[i].clear();

      // Drop shadow, offset the same (3, 5) as TerritoryNode's — a plain
      // translated copy of the polygon, which (unlike a halo or bevel)
      // generalizes to any shape without distortion.
      g.poly(points.map((p) => ({ x: p.x + 3, y: p.y + 5 }))).fill({ color: 0x000000, alpha: 0.38 });

      g.poly(points)
        .fill({ color: this.currentFill, alpha: FILL_ALPHA })
        .stroke({ color: borderColor, width: borderWidth });
    }
  }

  destroy(options?: Parameters<Container["destroy"]>[0]) {
    for (const g of this.polygonGfx) {
      g.removeAllListeners();
    }
    super.destroy(options);
  }
}
