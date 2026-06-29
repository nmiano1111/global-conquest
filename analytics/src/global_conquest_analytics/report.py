"""Generate Markdown report, validation CSV, and player summary CSV."""

from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path

import pandas as pd

from global_conquest_analytics.combat import build_player_combat_summary
from global_conquest_analytics.validation import validate_combat_df

_REPORT_DIR = Path(__file__).parents[2] / "reports" / "generated" / "combat"

_DIE_FACES = list(range(1, 7))


def _die_pct_table(df: pd.DataFrame, col: str) -> str:
    """Return a Markdown table of die-face percentages for one column."""
    if df.empty or col not in df.columns:
        return "_No data._\n"

    all_faces = df[col].explode().dropna().astype(int)
    total = len(all_faces)
    if total == 0:
        return "_No data._\n"

    counts = all_faces.value_counts().reindex(_DIE_FACES, fill_value=0)
    lines = ["| Face | Count | % |", "|------|-------|---|"]
    for face in _DIE_FACES:
        c = int(counts[face])
        pct = 100.0 * c / total if total else 0.0
        lines.append(f"| {face} | {c:,} | {pct:.1f}% |")
    lines.append(f"| **Total** | {total:,} | 100% |")
    return "\n".join(lines) + "\n"


def _result_count_table(df: pd.DataFrame) -> str:
    """Return a Markdown table of (attacker_losses, defender_losses) counts."""
    if df.empty:
        return "_No data._\n"
    counts = (
        df.groupby(["attacker_losses", "defender_losses"])
        .size()
        .reset_index(name="count")
        .sort_values(["attacker_losses", "defender_losses"])
    )
    lines = ["| Attacker Losses | Defender Losses | Count |", "|---|---|---|"]
    for _, row in counts.iterrows():
        a_losses = int(row["attacker_losses"])
        d_losses = int(row["defender_losses"])
        cnt = int(row["count"])
        lines.append(f"| {a_losses} | {d_losses} | {cnt:,} |")
    return "\n".join(lines) + "\n"


def generate_report(
    combat_df: pd.DataFrame,
    report_dir: Path = _REPORT_DIR,
) -> Path:
    """Generate all report artefacts for combat_roll_resolved events.

    Writes:
      - report.md
      - validation_failures.csv
      - player_combat_summary.csv

    Args:
        combat_df: Normalised combat DataFrame.
        report_dir: Directory to write artefacts.

    Returns:
        Path to report.md.
    """
    report_dir.mkdir(parents=True, exist_ok=True)

    # --- Validation ---
    failures = validate_combat_df(combat_df)
    failure_count = len(failures)

    # Write validation_failures.csv
    failures_csv = report_dir / "validation_failures.csv"
    if failures:
        failures_df = pd.DataFrame(failures)
    else:
        failures_df = pd.DataFrame(columns=["row_index", "event_id", "game_id", "field", "message"])
    failures_df.to_csv(failures_csv, index=False)

    # --- Player summary ---
    player_summary = build_player_combat_summary(combat_df)
    summary_csv = report_dir / "player_combat_summary.csv"
    player_summary.to_csv(summary_csv, index=False)

    # --- Stats for the report header ---
    now_utc = datetime.now(UTC).strftime("%Y-%m-%d %H:%M:%S UTC")
    event_count = len(combat_df)
    game_count = int(combat_df["game_id"].nunique()) if not combat_df.empty else 0
    attacker_count = (
        int(combat_df["attacker_player_id"].nunique()) if not combat_df.empty else 0
    )
    defender_count = (
        int(combat_df["defender_player_id"].nunique()) if not combat_df.empty else 0
    )

    def _total_dice(col: str) -> int:
        if combat_df.empty or col not in combat_df.columns:
            return 0
        return int(combat_df[col].apply(lambda d: len(d) if isinstance(d, list) else 0).sum())

    total_att_dice = _total_dice("attacker_dice")
    total_def_dice = _total_dice("defender_dice")

    att_pct = _die_pct_table(combat_df, "attacker_dice")
    def_pct = _die_pct_table(combat_df, "defender_dice")
    result_table = _result_count_table(combat_df)

    empty_note = ""
    if event_count == 0:
        empty_note = (
            "\n> **Note:** No `combat_roll_resolved` events were found in the raw data. "
            "Run `export-events` to extract fresh data from Postgres, then re-run "
            "`combat-report`.\n"
        )

    md = f"""# Global Conquest — Combat Roll Report

Generated: {now_utc}

{empty_note}
## Summary

| Metric | Value |
|--------|-------|
| Combat rolls | {event_count:,} |
| Games | {game_count:,} |
| Distinct attackers | {attacker_count:,} |
| Distinct defenders | {defender_count:,} |
| Total attacker dice | {total_att_dice:,} |
| Total defender dice | {total_def_dice:,} |
| Validation failures | {failure_count:,} |

> **Sample size warning:** These statistics reflect only the data extracted from
> this instance of Global Conquest. Small datasets (fewer than a few thousand rolls
> per die face) should not be treated as proof of RNG quality or bias.

## Charts

### Attacker Die-Face Distribution

How often each face (1–6) appeared across all attacker dice rolls. In a fair RNG each
face should appear roughly equally. Skew here suggests a biased random number generator
or a bug in dice generation.

![Attacker die distribution](attacker_die_distribution.png)

{att_pct}

### Defender Die-Face Distribution

Same as above but for the defending player's dice. Comparing attacker vs defender
distributions can reveal whether the two dice pools are drawn from the same source.

![Defender die distribution](defender_die_distribution.png)

{def_pct}

### Combat Result by Dice Count Combination

Average attacker and defender losses grouped by how many dice each side rolled
(e.g. 3v2, 2v1, 1v1). This reflects the theoretical odds of each matchup — a 3v2
attack should favour the attacker more than a 1v1.

![Combat result by dice counts](combat_result_by_dice_counts.png)

### Loss Distribution

How often each specific outcome occurred (e.g. attacker loses 1 and defender loses 1,
attacker loses 0 and defender loses 1, etc.). This is the most direct view of combat
fairness across all roll types combined.

![Loss distribution](loss_distribution.png)

## Combat Result Counts

{result_table}

## Artefacts

| File | Description |
|------|-------------|
| `validation_failures.csv` | Row-level validation failures (empty if all rows passed) |
| `player_combat_summary.csv` | Per-attacker aggregate statistics |
"""

    report_path = report_dir / "report.md"
    report_path.write_text(md, encoding="utf-8")
    print(f"Report written → {report_path}")
    print(f"Validation failures: {failure_count}")
    return report_path
