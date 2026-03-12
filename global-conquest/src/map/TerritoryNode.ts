import { Circle, Container, Graphics, Text, TextStyle } from "pixi.js";
import type { TerritoryDisplayState } from "./types";

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

export class TerritoryNode extends Container {
  private circleGfx: Graphics;
  private armyLabel: Text;

  constructor(name: string, x: number, y: number, onClick: (name: string) => void) {
    super();
    this.position.set(x, y);

    // Extend hit area slightly beyond the visible circle for easier clicking.
    this.hitArea = new Circle(0, 0, TERRITORY_RADIUS + 4);
    this.eventMode = "static";
    this.cursor = "pointer";
    this.on("pointerdown", () => onClick(name));

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

    this.drawCircle("#e2e8f0", false);
  }

  update(state: TerritoryDisplayState, playerColors: string[]) {
    const fill =
      state.owner >= 0 ? (playerColors[state.owner] ?? "#e2e8f0") : "#e2e8f0";
    this.drawCircle(fill, state.isSelected);
    this.armyLabel.text = String(state.armies);
  }

  private drawCircle(fillColor: string, selected: boolean) {
    const g = this.circleGfx.clear();
    const R = TERRITORY_RADIUS;

    // ── 1. Selection halo ──────────────────────────────────────────────
    // Drawn first so it sits behind everything else. Gold ring + soft fill.
    if (selected) {
      g.circle(0, 0, R + 11)
        .fill({ color: "#fbbf24", alpha: 0.22 })
        .stroke({ color: "#fbbf24", width: 3, alpha: 0.95 });
    }

    // ── 2. Drop shadow ─────────────────────────────────────────────────
    // A slightly offset, blurred dark circle gives the token a lifted look.
    g.circle(3, 5, R).fill({ color: 0x000000, alpha: 0.38 });

    // ── 3. Main token body ─────────────────────────────────────────────
    g.circle(0, 0, R)
      .fill({ color: fillColor, alpha: 0.95 })
      .stroke({
        color: selected ? "#fbbf24" : "#0f172a",
        width: selected ? 3.5 : 2.5,
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
  }
}
