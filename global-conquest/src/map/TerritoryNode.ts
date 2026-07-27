import { Circle, Container, Graphics, Text } from "pixi.js";
import { HIGHLIGHT_STYLE, UNOWNED_FILL } from "./highlightStyles";
import { ARMY_TEXT_STYLE, NAME_TEXT_STYLE } from "./territoryTextStyles";
import type { TerritoryDisplayState, TerritoryHighlightKind } from "./types";

const TERRITORY_RADIUS = 42;

export class TerritoryNode extends Container {
  private circleGfx: Graphics;
  private armyLabel: Text;
  private currentHighlight: TerritoryHighlightKind = "none";
  private currentFill = UNOWNED_FILL;

  constructor(name: string, x: number, y: number, onClick: (name: string) => void) {
    super();
    this.position.set(x, y);

    // Extend hit area slightly beyond the visible circle for easier clicking.
    this.hitArea = new Circle(0, 0, TERRITORY_RADIUS + 4);
    this.eventMode = "static";
    this.cursor = "pointer";
    this.on("pointertap", () => onClick(name));

    this.circleGfx = new Graphics();
    this.addChild(this.circleGfx);

    // Army count — centered. The 32px bold number is the primary focal point.
    this.armyLabel = new Text({ text: "0", style: ARMY_TEXT_STYLE });
    this.armyLabel.anchor.set(0.5, 0.5);
    this.armyLabel.position.set(0, -3);
    this.addChild(this.armyLabel);

    // Territory name — sits below the army count in the lower arc of the circle.
    const nameLabel = new Text({ text: name, style: NAME_TEXT_STYLE });
    nameLabel.anchor.set(0.5, 0);
    nameLabel.position.set(0, 20);
    this.addChild(nameLabel);

    this.drawCircle(UNOWNED_FILL, "none", 0);
  }

  update(state: TerritoryDisplayState, playerColors: string[]) {
    const fill =
      state.owner >= 0 ? (playerColors[state.owner] ?? UNOWNED_FILL) : UNOWNED_FILL;
    this.currentHighlight = state.highlight;
    this.currentFill = fill;
    this.drawCircle(fill, state.highlight, 0);
    this.armyLabel.text = String(state.armies);
  }

  /** Advances the pulse animation for highlight kinds that pulse. No-op (and never called) when reduced-motion is active. */
  tickPulse(phase: number) {
    const style = this.currentHighlight === "none" ? null : HIGHLIGHT_STYLE[this.currentHighlight];
    if (!style?.pulse) return;
    this.drawCircle(this.currentFill, this.currentHighlight, phase);
  }

  private drawCircle(fillColor: string, highlight: TerritoryHighlightKind, pulsePhase: number) {
    const g = this.circleGfx.clear();
    const R = TERRITORY_RADIUS;
    const style = highlight === "none" ? null : HIGHLIGHT_STYLE[highlight];

    // ── 1. Highlight halo ──────────────────────────────────────────────
    // Drawn first so it sits behind everything else. A pulsing kind
    // oscillates ring alpha/radius gently — restrained, not distracting.
    if (style) {
      const pulseT = style.pulse ? (Math.sin(pulsePhase) + 1) / 2 : 1; // 0..1
      const ringR = R + 9 + (style.pulse ? pulseT * 4 : 2);
      const ringAlpha = style.pulse ? 0.5 + pulseT * 0.4 : 0.95;
      g.circle(0, 0, ringR)
        .fill({ color: style.color, alpha: style.fillAlpha })
        .stroke({ color: style.color, width: style.ringWidth, alpha: ringAlpha });
    }

    // ── 2. Drop shadow ─────────────────────────────────────────────────
    // A slightly offset, blurred dark circle gives the token a lifted look.
    g.circle(3, 5, R).fill({ color: 0x000000, alpha: 0.38 });

    // ── 3. Main token body ─────────────────────────────────────────────
    g.circle(0, 0, R)
      .fill({ color: fillColor, alpha: 0.95 })
      .stroke({
        color: style ? style.color : 0x0f172a,
        width: style ? style.ringWidth - 0.5 : 2.5,
      });

    // ── 4. Inner bevel rim ─────────────────────────────────────────────
    // A thin white ring just inside the outer edge creates a raised-edge /
    // beveled feel, making the token look solid rather than flat.
    g.circle(0, 0, R - 5).stroke({ color: "#ffffff", width: 1.5, alpha: 0.28 });

    // ── 5. Gloss highlight ─────────────────────────────────────────────
    // A small white ellipse near the top simulates a light source from
    // above, giving the token a subtle glossy / domed surface.
    g.ellipse(0, -R * 0.3, R * 0.5, R * 0.21).fill({
      color: "#ffffff",
      alpha: 0.22,
    });

    // Selection/target states scale the whole node up slightly — a second,
    // non-color-dependent signal that this territory is the active focus.
    const targetScale = style?.scale ?? 1.0;
    this.scale.set(targetScale);
  }
}
