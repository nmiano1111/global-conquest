"""Transform raw combat_roll_resolved events into an analysis-ready DataFrame."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pandas as pd

_PROCESSED_OUTPUT = (
    Path(__file__).parents[2] / "data" / "processed" / "combat_rolls.parquet"
)

# Metadata columns carried forward from the events table.
# Actual schema: id, game_id, game_sequence, event_type, event_version,
#                actor_player_id, occurred_at, payload
_META_COLS = [
    "id",
    "game_id",
    "game_sequence",
    "event_version",
    "actor_player_id",
    "occurred_at",
]

# Top-level scalar fields to flatten out of the payload JSON.
_PAYLOAD_SCALAR_FIELDS = [
    "schema_version",
    "turn_number",
    "phase",
    "attacker_player_id",
    "defender_player_id",
    "source_territory_id",
    "target_territory_id",
    "source_armies_before",
    "target_armies_before",
    "attacker_losses",
    "defender_losses",
    "source_armies_after",
    "target_armies_after",
    "territory_captured",
]

# Nested list fields kept as Python objects (lists).
_PAYLOAD_LIST_FIELDS = [
    "attacker_dice",
    "defender_dice",
    "comparisons",
]


def _parse_payload(raw: Any) -> dict[str, Any]:
    """Return the event payload as a dict, parsing JSON strings if necessary."""
    if isinstance(raw, dict):
        return raw
    if isinstance(raw, str):
        return json.loads(raw)  # type: ignore[no-any-return]
    raise TypeError(f"Unexpected payload type: {type(raw)}")


def normalize_combat_events(df: pd.DataFrame) -> pd.DataFrame:
    """Filter and flatten combat_roll_resolved rows.

    Accepts the raw game_domain_events DataFrame (as written by extract.py).
    Filters to event_type == 'combat_roll_resolved', flattens the JSON
    payload, and returns a tidy DataFrame.

    Args:
        df: Raw events DataFrame containing at minimum the columns produced
            by the extract query.

    Returns:
        A DataFrame with metadata + flattened body columns, one row per
        combat roll. Returns an empty DataFrame if there are no matching rows.
    """
    combat = df[df["event_type"] == "combat_roll_resolved"].copy()
    if combat.empty:
        return pd.DataFrame()

    records: list[dict[str, Any]] = []
    for _, row in combat.iterrows():
        payload = _parse_payload(row["payload"])
        record: dict[str, Any] = {col: row[col] for col in _META_COLS if col in row.index}
        for field in _PAYLOAD_SCALAR_FIELDS:
            record[field] = payload.get(field)
        for field in _PAYLOAD_LIST_FIELDS:
            record[field] = payload.get(field)
        records.append(record)

    return pd.DataFrame(records)


def build_player_combat_summary(combat_df: pd.DataFrame) -> pd.DataFrame:
    """Aggregate per-attacker statistics from the normalised combat DataFrame.

    Args:
        combat_df: Output of :func:`normalize_combat_events`.

    Returns:
        DataFrame with one row per unique attacker_player_id.
    """
    if combat_df.empty:
        return pd.DataFrame(
            columns=[
                "attacker_player_id",
                "games_as_attacker",
                "attack_rolls",
                "territories_captured",
                "capture_rate",
                "average_attacker_dice",
                "average_source_armies_before",
                "average_target_armies_before",
                "total_attacker_losses",
                "total_defender_losses_inflicted",
            ]
        )

    grouped = combat_df.groupby("attacker_player_id", as_index=False)

    summary = grouped.agg(
        games_as_attacker=("game_id", "nunique"),
        attack_rolls=("id", "count"),
        territories_captured=("territory_captured", "sum"),
        total_attacker_losses=("attacker_losses", "sum"),
        total_defender_losses_inflicted=("defender_losses", "sum"),
        average_source_armies_before=("source_armies_before", "mean"),
        average_target_armies_before=("target_armies_before", "mean"),
    )

    summary["capture_rate"] = summary["territories_captured"] / summary["attack_rolls"]

    def _mean_dice_count(pid: str) -> float:
        rows = combat_df[combat_df["attacker_player_id"] == pid]
        counts = rows["attacker_dice"].apply(
            lambda d: len(d) if d is not None and hasattr(d, "__len__") else 0
        )
        return float(counts.mean()) if len(counts) > 0 else 0.0

    summary["average_attacker_dice"] = summary["attacker_player_id"].apply(_mean_dice_count)

    return summary[
        [
            "attacker_player_id",
            "games_as_attacker",
            "attack_rolls",
            "territories_captured",
            "capture_rate",
            "average_attacker_dice",
            "average_source_armies_before",
            "average_target_armies_before",
            "total_attacker_losses",
            "total_defender_losses_inflicted",
        ]
    ]


def process_combat_events(
    raw_path: Path,
    output_path: Path = _PROCESSED_OUTPUT,
) -> pd.DataFrame:
    """End-to-end: read raw Parquet, normalise, write processed Parquet.

    Args:
        raw_path: Path to the raw game_events Parquet file.
        output_path: Destination for the processed combat_rolls Parquet.

    Returns:
        The normalised combat DataFrame.
    """
    df_raw = pd.read_parquet(raw_path, engine="pyarrow")
    df_combat = normalize_combat_events(df_raw)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    df_combat.to_parquet(output_path, index=False, engine="pyarrow")
    print(f"Processed {len(df_combat):,} combat rolls → {output_path}")
    return df_combat
