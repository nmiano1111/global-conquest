"""Unit tests for combat.py — no live database required."""

from __future__ import annotations

import json
import uuid

import pandas as pd
from global_conquest_analytics.combat import (
    build_player_combat_summary,
    normalize_combat_events,
)

# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

_PLAYER_A = str(uuid.UUID(int=1))
_PLAYER_B = str(uuid.UUID(int=2))
_GAME_1 = str(uuid.UUID(int=10))
_GAME_2 = str(uuid.UUID(int=11))


def _make_payload(
    *,
    attacker_player_id: str = _PLAYER_A,
    defender_player_id: str = _PLAYER_B,
    attacker_dice: list[int] | None = None,
    defender_dice: list[int] | None = None,
    territory_captured: bool = False,
    attacker_losses: int = 0,
    defender_losses: int = 1,
) -> dict:
    if attacker_dice is None:
        attacker_dice = [6, 5]
    if defender_dice is None:
        defender_dice = [3]
    return {
        "schema_version": 1,
        "turn_number": 1,
        "phase": "attack",
        "attacker_player_id": attacker_player_id,
        "defender_player_id": defender_player_id,
        "source_territory_id": "brazil",
        "target_territory_id": "north_africa",
        "source_armies_before": 5,
        "target_armies_before": 2,
        "attacker_dice": attacker_dice,
        "defender_dice": defender_dice,
        "comparisons": [
            {"attacker_die": 6, "defender_die": 3, "loser": "defender"},
        ],
        "attacker_losses": attacker_losses,
        "defender_losses": defender_losses,
        "source_armies_after": 5 - attacker_losses,
        "target_armies_after": 2 - defender_losses if not territory_captured else 0,
        "territory_captured": territory_captured,
    }


def _make_raw_df(rows: list[dict]) -> pd.DataFrame:
    """Build a mock raw events DataFrame matching the real extract.py schema.

    Real columns: id, game_id, game_sequence, event_type, event_version,
                  actor_player_id, occurred_at, payload
    """
    records = []
    for i, row in enumerate(rows):
        records.append(
            {
                "id": str(uuid.uuid4()),
                "game_id": row.get("game_id", _GAME_1),
                "game_sequence": row.get("game_sequence", i + 1),
                "event_type": row.get("event_type", "combat_roll_resolved"),
                "event_version": row.get("event_version", 1),
                "actor_player_id": row.get("actor_player_id", _PLAYER_A),
                "occurred_at": pd.Timestamp("2024-01-01", tz="UTC"),
                "payload": row.get("payload", _make_payload()),
            }
        )
    return pd.DataFrame(records)


# ---------------------------------------------------------------------------
# Tests: normalize_combat_events
# ---------------------------------------------------------------------------


class TestNormalizeCombatEvents:
    def test_dict_payload_normalised(self) -> None:
        """Dict payload values are flattened into columns."""
        payload = _make_payload()
        df = _make_raw_df([{"payload": payload}])
        result = normalize_combat_events(df)

        assert len(result) == 1
        assert result.iloc[0]["attacker_player_id"] == _PLAYER_A
        assert result.iloc[0]["defender_player_id"] == _PLAYER_B
        assert result.iloc[0]["attacker_dice"] == [6, 5]
        assert result.iloc[0]["defender_dice"] == [3]
        assert result.iloc[0]["territory_captured"] == False  # noqa: E712 (numpy bool)

    def test_json_string_payload_normalised(self) -> None:
        """JSON-serialised string payloads are parsed and flattened."""
        payload = _make_payload()
        df = _make_raw_df([{"payload": json.dumps(payload)}])
        result = normalize_combat_events(df)

        assert len(result) == 1
        assert result.iloc[0]["attacker_player_id"] == _PLAYER_A
        assert result.iloc[0]["source_territory_id"] == "brazil"

    def test_unrelated_event_types_filtered_out(self) -> None:
        """Rows with other event_types are excluded."""
        df = _make_raw_df(
            [
                {"event_type": "game_started"},
                {"event_type": "combat_roll_resolved"},
                {"event_type": "territory_claimed"},
            ]
        )
        result = normalize_combat_events(df)
        assert len(result) == 1

    def test_empty_input_returns_empty_df(self) -> None:
        """An empty raw DataFrame returns an empty result."""
        df = pd.DataFrame(
            columns=["id", "game_id", "game_sequence", "event_type", "event_version",
                     "actor_player_id", "occurred_at", "payload"]
        )
        result = normalize_combat_events(df)
        assert result.empty

    def test_no_combat_events_returns_empty_df(self) -> None:
        """A DataFrame with no combat events returns empty."""
        df = _make_raw_df([{"event_type": "reinforcement_placed"}])
        result = normalize_combat_events(df)
        assert result.empty

    def test_metadata_columns_preserved(self) -> None:
        """id, game_id, game_sequence, event_version, actor_player_id, occurred_at are kept."""
        df = _make_raw_df([{}])
        result = normalize_combat_events(df)
        expected_cols = [
            "id", "game_id", "game_sequence", "event_version", "actor_player_id", "occurred_at"
        ]
        for col in expected_cols:
            assert col in result.columns, f"Missing column: {col}"

    def test_nested_list_fields_preserved(self) -> None:
        """attacker_dice, defender_dice, and comparisons stay as lists."""
        payload = _make_payload(attacker_dice=[6, 4, 2], defender_dice=[5, 3])
        df = _make_raw_df([{"payload": payload}])
        result = normalize_combat_events(df)
        assert isinstance(result.iloc[0]["attacker_dice"], list)
        assert isinstance(result.iloc[0]["defender_dice"], list)
        assert isinstance(result.iloc[0]["comparisons"], list)

    def test_territory_captured_flag(self) -> None:
        """territory_captured=True is preserved."""
        payload = _make_payload(territory_captured=True, defender_losses=2)
        payload["target_armies_after"] = 0
        df = _make_raw_df([{"payload": payload}])
        result = normalize_combat_events(df)
        assert result.iloc[0]["territory_captured"] == True  # noqa: E712 (numpy bool)


# ---------------------------------------------------------------------------
# Tests: build_player_combat_summary
# ---------------------------------------------------------------------------


class TestBuildPlayerCombatSummary:
    def _make_combat_df(self) -> pd.DataFrame:
        """Return a small normalised combat DataFrame for two players."""
        raw = _make_raw_df(
            [
                {
                    "game_id": _GAME_1,
                    "payload": _make_payload(
                        attacker_player_id=_PLAYER_A,
                        territory_captured=True,
                        attacker_losses=0,
                        defender_losses=2,
                    ),
                },
                {
                    "game_id": _GAME_1,
                    "payload": _make_payload(
                        attacker_player_id=_PLAYER_A,
                        territory_captured=False,
                        attacker_losses=1,
                        defender_losses=0,
                    ),
                },
                {
                    "game_id": _GAME_2,
                    "payload": _make_payload(
                        attacker_player_id=_PLAYER_B,
                        territory_captured=False,
                        attacker_losses=1,
                        defender_losses=1,
                    ),
                },
            ]
        )
        return normalize_combat_events(raw)

    def test_returns_one_row_per_attacker(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        assert len(summary) == 2
        player_ids = set(summary["attacker_player_id"].tolist())
        assert player_ids == {_PLAYER_A, _PLAYER_B}

    def test_attack_rolls_count(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        a_row = summary[summary["attacker_player_id"] == _PLAYER_A].iloc[0]
        assert int(a_row["attack_rolls"]) == 2

    def test_territories_captured_count(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        a_row = summary[summary["attacker_player_id"] == _PLAYER_A].iloc[0]
        assert int(a_row["territories_captured"]) == 1

    def test_capture_rate(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        a_row = summary[summary["attacker_player_id"] == _PLAYER_A].iloc[0]
        assert abs(float(a_row["capture_rate"]) - 0.5) < 1e-9

    def test_total_losses(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        a_row = summary[summary["attacker_player_id"] == _PLAYER_A].iloc[0]
        assert int(a_row["total_attacker_losses"]) == 1
        assert int(a_row["total_defender_losses_inflicted"]) == 2

    def test_games_as_attacker(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        a_row = summary[summary["attacker_player_id"] == _PLAYER_A].iloc[0]
        # Player A only attacked in GAME_1
        assert int(a_row["games_as_attacker"]) == 1

    def test_empty_input_returns_empty_with_expected_columns(self) -> None:
        summary = build_player_combat_summary(pd.DataFrame())
        assert summary.empty
        for col in ["attacker_player_id", "attack_rolls", "territories_captured", "capture_rate"]:
            assert col in summary.columns

    def test_average_attacker_dice(self) -> None:
        combat_df = self._make_combat_df()
        summary = build_player_combat_summary(combat_df)
        a_row = summary[summary["attacker_player_id"] == _PLAYER_A].iloc[0]
        # Both rolls used [6, 5] (2 dice), so average should be 2.0
        assert abs(float(a_row["average_attacker_dice"]) - 2.0) < 1e-9
