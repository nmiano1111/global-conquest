import { Circle, Container, Graphics, Text, TextStyle } from "pixi.js";
import type { TerritoryDisplayState, TerritoryHighlightKind } from "./types";

const TERRITORY_RADIUS = 42;

// Shared text styles — created once, reused across all 42 nodes.
// Army count: large, centered. A solid dark stroke outline is far more readable
// than a drop shadow alone — it guarantees contrast against any player color.
const armyStyle = new TextStyle({
  fill: "#ffffff",
  fontSize: 32,
  fontWeight: "bold",
  align: "center",
  stroke: { color: "#000000", width: 6 },
});

// Name label: small, white, bold stroke outline so it reads on any fill color.
const nameStyle = new TextStyle({
  fill: "#ffffff",
  fontSize: 11,
  fontWeight: "800",
  align: "center",
  stroke: { color: "rgba(0,0,0,0.75)", width: 3.5 },
  dropShadow: { color: "#000000", blur: 3, distance: 1, alpha: 0.4 },
});

/**
 * Per-highlight-kind ring styling. Each kind combines a distinct color,
 * ring width, and fill alpha so states remain distinguishable without
 * relying on color alone (width and alpha differ too), and stay legible
 * for color-blind users.
 */
const HIGHLIGHT_STYLE: Record<
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

export class TerritoryNode extends Container {
  private circleGfx: Graphics;
  private armyLabel: Text;
  private currentHighlight: TerritoryHighlightKind = "none";
  private currentFill = "#e2e8f0";

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
    this.armyLabel = new Text({ text: "0", style: armyStyle });
    this.armyLabel.anchor.set(0.5, 0.5);
    this.armyLabel.position.set(0, -3);
    this.addChild(this.armyLabel);

    // Territory name — sits below the army count in the lower arc of the circle.
    const nameLabel = new Text({ text: name, style: nameStyle });
    nameLabel.anchor.set(0.5, 0);
    nameLabel.position.set(0, 20);
    this.addChild(nameLabel);

    this.drawCircle("#e2e8f0", "none", 0);
  }

  update(state: TerritoryDisplayState, playerColors: string[]) {
    const fill =
      state.owner >= 0 ? (playerColors[state.owner] ?? "#e2e8f0") : "#e2e8f0";
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
