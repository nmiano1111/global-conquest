"""Unit tests for validation.py — no live database required."""

from __future__ import annotations

import uuid

import pandas as pd
from global_conquest_analytics.validation import validate_combat_df

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_GAME_1 = str(uuid.UUID(int=10))
_GAME_2 = str(uuid.UUID(int=11))


def _valid_row(
    *,
    game_id: str = _GAME_1,
    schema_version: int = 1,
    attacker_dice: list[int] | None = None,
    defender_dice: list[int] | None = None,
    comparisons: list[dict] | None = None,
    attacker_losses: int = 0,
    defender_losses: int = 1,
    source_armies_before: int = 5,
    source_armies_after: int = 5,
    target_armies_before: int = 3,
    target_armies_after: int = 2,
    territory_captured: bool = False,
) -> dict:
    if attacker_dice is None:
        attacker_dice = [6, 5]
    if defender_dice is None:
        defender_dice = [3]
    if comparisons is None:
        comparisons = [{"attacker_die": 6, "defender_die": 3, "loser": "defender"}]
    return {
        "id": str(uuid.uuid4()),
        "game_id": game_id,
        "schema_version": schema_version,
        "attacker_player_id": str(uuid.UUID(int=1)),
        "defender_player_id": str(uuid.UUID(int=2)),
        "attacker_dice": attacker_dice,
        "defender_dice": defender_dice,
        "comparisons": comparisons,
        "attacker_losses": attacker_losses,
        "defender_losses": defender_losses,
        "source_armies_before": source_armies_before,
        "source_armies_after": source_armies_after,
        "target_armies_before": target_armies_before,
        "target_armies_after": target_armies_after,
        "territory_captured": territory_captured,
    }


def _df(*rows: dict) -> pd.DataFrame:
    return pd.DataFrame(list(rows))


def _fields(failures: list[dict]) -> set[str]:
    return {f["field"] for f in failures}


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


class TestValidCombatEvent:
    def test_valid_row_passes_all_checks(self) -> None:
        """A well-formed row produces zero failures."""
        df = _df(_valid_row())
        failures = validate_combat_df(df)
        assert failures == []

    def test_two_valid_rows_pass(self) -> None:
        df = _df(
            _valid_row(),
            _valid_row(),
        )
        failures = validate_combat_df(df)
        assert failures == []


class TestSchemaVersion:
    def test_unsupported_schema_version_detected(self) -> None:
        df = _df(_valid_row(schema_version=99))
        failures = validate_combat_df(df)
        assert any(f["field"] == "schema_version" for f in failures)

    def test_supported_schema_version_passes(self) -> None:
        df = _df(_valid_row(schema_version=1))
        failures = validate_combat_df(df)
        assert not any(f["field"] == "schema_version" for f in failures)


class TestDiceValidation:
    def test_invalid_attacker_die_value_detected(self) -> None:
        df = _df(_valid_row(attacker_dice=[7, 3]))
        failures = validate_combat_df(df)
        assert "attacker_dice" in _fields(failures)

    def test_invalid_defender_die_value_detected(self) -> None:
        df = _df(_valid_row(defender_dice=[0]))
        failures = validate_combat_df(df)
        assert "defender_dice" in _fields(failures)

    def test_zero_die_face_is_invalid(self) -> None:
        df = _df(_valid_row(attacker_dice=[0, 3]))
        failures = validate_combat_df(df)
        assert "attacker_dice" in _fields(failures)

    def test_empty_attacker_dice_detected(self) -> None:
        df = _df(_valid_row(attacker_dice=[]))
        failures = validate_combat_df(df)
        assert "attacker_dice" in _fields(failures)

    def test_empty_defender_dice_detected(self) -> None:
        df = _df(_valid_row(defender_dice=[]))
        failures = validate_combat_df(df)
        assert "defender_dice" in _fields(failures)

    def test_valid_dice_all_faces_pass(self) -> None:
        for face in range(1, 7):
            df = _df(_valid_row(attacker_dice=[face]))
            failures = validate_combat_df(df)
            att_failures = [f for f in failures if f["field"] == "attacker_dice"]
            assert att_failures == [], f"Face {face} incorrectly flagged"


class TestArmyMath:
    def test_incorrect_attacker_army_total_detected(self) -> None:
        df = _df(
            _valid_row(
                source_armies_before=5,
                attacker_losses=1,
                source_armies_after=10,  # wrong
            )
        )
        failures = validate_combat_df(df)
        assert "source_armies_after" in _fields(failures)

    def test_incorrect_defender_army_total_detected(self) -> None:
        df = _df(
            _valid_row(
                target_armies_before=3,
                defender_losses=1,
                target_armies_after=5,  # wrong
            )
        )
        failures = validate_combat_df(df)
        assert "target_armies_after" in _fields(failures)

    def test_correct_army_math_passes(self) -> None:
        df = _df(
            _valid_row(
                source_armies_before=8,
                attacker_losses=1,
                source_armies_after=7,
                target_armies_before=4,
                defender_losses=1,
                target_armies_after=3,
            )
        )
        failures = validate_combat_df(df)
        army_fields = ("source_armies_after", "target_armies_after")
        army_failures = [f for f in failures if f["field"] in army_fields]
        assert army_failures == []


class TestCaptureFlag:
    def test_incorrect_capture_flag_detected(self) -> None:
        """territory_captured=True but target_armies_after != 0."""
        df = _df(
            _valid_row(
                target_armies_before=3,
                defender_losses=1,
                target_armies_after=2,
                territory_captured=True,  # wrong — armies remain
            )
        )
        failures = validate_combat_df(df)
        assert "territory_captured" in _fields(failures)

    def test_missed_capture_flag_detected(self) -> None:
        """territory_captured=False but target_armies_after == 0."""
        df = _df(
            _valid_row(
                target_armies_before=2,
                defender_losses=2,
                target_armies_after=0,
                territory_captured=False,  # wrong — should be True
            )
        )
        failures = validate_combat_df(df)
        assert "territory_captured" in _fields(failures)

    def test_correct_capture_flag_passes(self) -> None:
        df = _df(
            _valid_row(
                target_armies_before=2,
                defender_losses=2,
                target_armies_after=0,
                territory_captured=True,
            )
        )
        failures = validate_combat_df(df)
        capture_failures = [f for f in failures if f["field"] == "territory_captured"]
        assert capture_failures == []


class TestRiskTieRule:
    def test_tie_with_attacker_losing_passes(self) -> None:
        """Equal dice: loser='attacker' is correct per Risk rules."""
        row = _valid_row(
            attacker_dice=[4],
            defender_dice=[4],
            comparisons=[{"attacker_die": 4, "defender_die": 4, "loser": "attacker"}],
            attacker_losses=1,
            defender_losses=0,
            source_armies_before=5,
            source_armies_after=4,
            target_armies_before=3,
            target_armies_after=3,
        )
        failures = validate_combat_df(_df(row))
        comparison_failures = [f for f in failures if f["field"] == "comparisons"]
        assert comparison_failures == []

    def test_tie_with_defender_losing_is_violation(self) -> None:
        """Equal dice: loser='defender' violates Risk tie rule."""
        row = _valid_row(
            attacker_dice=[4],
            defender_dice=[4],
            comparisons=[{"attacker_die": 4, "defender_die": 4, "loser": "defender"}],
            attacker_losses=0,
            defender_losses=1,
            source_armies_before=5,
            source_armies_after=5,
            target_armies_before=3,
            target_armies_after=2,
        )
        failures = validate_combat_df(_df(row))
        assert "comparisons" in _fields(failures)

    def test_attacker_wins_higher_die(self) -> None:
        """Attacker die > defender die: loser='defender' is correct."""
        row = _valid_row(
            attacker_dice=[6],
            defender_dice=[3],
            comparisons=[{"attacker_die": 6, "defender_die": 3, "loser": "defender"}],
            attacker_losses=0,
            defender_losses=1,
            source_armies_before=5,
            source_armies_after=5,
            target_armies_before=3,
            target_armies_after=2,
        )
        failures = validate_combat_df(_df(row))
        comparison_failures = [f for f in failures if f["field"] == "comparisons"]
        assert comparison_failures == []

    def test_defender_wins_higher_die(self) -> None:
        """Defender die > attacker die: loser='attacker' is correct."""
        row = _valid_row(
            attacker_dice=[2],
            defender_dice=[5],
            comparisons=[{"attacker_die": 2, "defender_die": 5, "loser": "attacker"}],
            attacker_losses=1,
            defender_losses=0,
            source_armies_before=5,
            source_armies_after=4,
            target_armies_before=3,
            target_armies_after=3,
        )
        failures = validate_combat_df(_df(row))
        comparison_failures = [f for f in failures if f["field"] == "comparisons"]
        assert comparison_failures == []


class TestMultipleValidRows:
    def test_two_valid_rows_from_same_game_pass(self) -> None:
        """Multiple valid combat rolls from the same game produce no failures."""
        df = _df(
            _valid_row(game_id=_GAME_1),
            _valid_row(game_id=_GAME_1),
        )
        failures = validate_combat_df(df)
        assert failures == []

    def test_valid_rows_from_different_games_pass(self) -> None:
        """Valid combat rolls from different games produce no failures."""
        df = _df(
            _valid_row(game_id=_GAME_1),
            _valid_row(game_id=_GAME_2),
        )
        failures = validate_combat_df(df)
        assert failures == []
