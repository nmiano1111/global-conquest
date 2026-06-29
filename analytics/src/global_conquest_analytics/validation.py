"""Row-level validation of normalised combat_roll_resolved events."""

from __future__ import annotations

from typing import Any

import numpy as np
import pandas as pd

_SUPPORTED_SCHEMA_VERSIONS = {1}
_VALID_LOSERS = {"attacker", "defender"}
_DIE_MIN = 1
_DIE_MAX = 6


def _failure(
    row_index: int,
    row: pd.Series,  # type: ignore[type-arg]
    field: str,
    message: str,
) -> dict[str, Any]:
    return {
        "row_index": row_index,
        "event_id": row.get("id"),
        "game_id": row.get("game_id"),
        "field": field,
        "message": message,
    }


def _validate_schema_version(
    row_index: int, row: pd.Series, failures: list[dict[str, Any]]  # type: ignore[type-arg]
) -> bool:
    v = row.get("schema_version")
    if v not in _SUPPORTED_SCHEMA_VERSIONS:
        failures.append(
            _failure(row_index, row, "schema_version", f"Unsupported schema_version: {v!r}")
        )
        return False
    return True


def _validate_dice_array(
    row_index: int,
    row: pd.Series,  # type: ignore[type-arg]
    field: str,
    failures: list[dict[str, Any]],
) -> bool:
    raw = row.get(field)
    # Parquet round-trips may return numpy arrays instead of Python lists — normalise.
    try:
        dice = list(raw) if raw is not None else None
    except TypeError:
        dice = None
    if not dice:  # None, empty list, or unconvertible
        failures.append(
            _failure(row_index, row, field, f"{field} must be a non-empty list; got {raw!r}")
        )
        return False
    for i, face in enumerate(dice):
        # Accept both Python int and numpy integer types.
        if not isinstance(face, int | np.integer) or not (_DIE_MIN <= int(face) <= _DIE_MAX):
            failures.append(
                _failure(
                    row_index,
                    row,
                    field,
                    f"{field}[{i}]={face!r} is not a valid die face (1–6)",
                )
            )
            return False
    return True


def _validate_army_math(
    row_index: int,
    row: pd.Series,  # type: ignore[type-arg]
    failures: list[dict[str, Any]],
) -> None:
    before_a = row.get("source_armies_before")
    losses_a = row.get("attacker_losses")
    after_a = row.get("source_armies_after")
    if before_a is not None and losses_a is not None and after_a is not None:
        if int(before_a) - int(losses_a) != int(after_a):
            failures.append(
                _failure(
                    row_index,
                    row,
                    "source_armies_after",
                    f"source_armies_before ({before_a}) - attacker_losses ({losses_a}) "
                    f"!= source_armies_after ({after_a})",
                )
            )

    before_d = row.get("target_armies_before")
    losses_d = row.get("defender_losses")
    after_d = row.get("target_armies_after")
    if before_d is not None and losses_d is not None and after_d is not None:
        if int(before_d) - int(losses_d) != int(after_d):
            failures.append(
                _failure(
                    row_index,
                    row,
                    "target_armies_after",
                    f"target_armies_before ({before_d}) - defender_losses ({losses_d}) "
                    f"!= target_armies_after ({after_d})",
                )
            )


def _validate_loss_comparison_count(
    row_index: int,
    row: pd.Series,  # type: ignore[type-arg]
    failures: list[dict[str, Any]],
) -> None:
    raw_cmp = row.get("comparisons")
    try:
        comparisons = list(raw_cmp) if raw_cmp is not None else None
    except TypeError:
        comparisons = None
    if not comparisons:
        return
    expected = len(comparisons)
    actual = (row.get("attacker_losses") or 0) + (row.get("defender_losses") or 0)
    if actual != expected:
        failures.append(
            _failure(
                row_index,
                row,
                "comparisons",
                f"attacker_losses + defender_losses ({actual}) != len(comparisons) ({expected})",
            )
        )


def _validate_capture_flag(
    row_index: int,
    row: pd.Series,  # type: ignore[type-arg]
    failures: list[dict[str, Any]],
) -> None:
    captured = row.get("territory_captured")
    after_d = row.get("target_armies_after")
    if captured is not None and after_d is not None:
        expected_capture = after_d == 0
        if bool(captured) != expected_capture:
            failures.append(
                _failure(
                    row_index,
                    row,
                    "territory_captured",
                    f"territory_captured={captured!r} but target_armies_after={after_d}",
                )
            )


def _validate_comparisons(
    row_index: int,
    row: pd.Series,  # type: ignore[type-arg]
    failures: list[dict[str, Any]],
) -> None:
    raw_cmp = row.get("comparisons")
    try:
        comparisons = list(raw_cmp) if raw_cmp is not None else None
    except TypeError:
        comparisons = None
    if not comparisons:
        return
    for i, cmp in enumerate(comparisons):
        if not isinstance(cmp, dict):
            failures.append(
                _failure(
                    row_index,
                    row,
                    "comparisons",
                    f"comparisons[{i}] is not a dict: {cmp!r}",
                )
            )
            continue

        att_die = cmp.get("attacker_die")
        def_die = cmp.get("defender_die")
        loser = cmp.get("loser")

        if not isinstance(att_die, int) or not (_DIE_MIN <= att_die <= _DIE_MAX):
            failures.append(
                _failure(
                    row_index,
                    row,
                    "comparisons",
                    f"comparisons[{i}].attacker_die={att_die!r} is not a valid die face",
                )
            )
        if not isinstance(def_die, int) or not (_DIE_MIN <= def_die <= _DIE_MAX):
            failures.append(
                _failure(
                    row_index,
                    row,
                    "comparisons",
                    f"comparisons[{i}].defender_die={def_die!r} is not a valid die face",
                )
            )
        if loser not in _VALID_LOSERS:
            failures.append(
                _failure(
                    row_index,
                    row,
                    "comparisons",
                    f"comparisons[{i}].loser={loser!r} is not 'attacker' or 'defender'",
                )
            )
            continue

        # Ties: attacker must lose (Risk rule — defender wins on ties)
        if isinstance(att_die, int) and isinstance(def_die, int):
            if att_die == def_die and loser != "attacker":
                failures.append(
                    _failure(
                        row_index,
                        row,
                        "comparisons",
                        f"comparisons[{i}]: tie ({att_die}=={def_die}) but loser={loser!r}; "
                        "defender wins ties in Risk",
                    )
                )
            elif att_die > def_die and loser != "defender":
                failures.append(
                    _failure(
                        row_index,
                        row,
                        "comparisons",
                        f"comparisons[{i}]: attacker_die ({att_die}) > defender_die ({def_die}) "
                        f"but loser={loser!r}; expected 'defender'",
                    )
                )
            elif att_die < def_die and loser != "attacker":
                failures.append(
                    _failure(
                        row_index,
                        row,
                        "comparisons",
                        f"comparisons[{i}]: attacker_die ({att_die}) < defender_die ({def_die}) "
                        f"but loser={loser!r}; expected 'attacker'",
                    )
                )


def validate_combat_df(df: pd.DataFrame) -> list[dict[str, Any]]:
    """Validate all rows in a normalised combat DataFrame.

    Collects all failures without aborting early. Row-level errors are
    independent — a bad row does not prevent other rows from being checked.

    Args:
        df: Normalised combat DataFrame (output of combat.normalize_combat_events).

    Returns:
        List of failure dicts. Each dict has keys: row_index, event_id, game_id,
        field, message. An empty list means all rows passed.
    """
    failures: list[dict[str, Any]] = []

    # Per-row checks.
    for idx, row in df.iterrows():
        row_index = int(idx)  # type: ignore[call-overload]
        if not _validate_schema_version(row_index, row, failures):
            # If schema version is unsupported, skip payload checks for this row.
            continue
        _validate_dice_array(row_index, row, "attacker_dice", failures)
        _validate_dice_array(row_index, row, "defender_dice", failures)
        _validate_army_math(row_index, row, failures)
        _validate_loss_comparison_count(row_index, row, failures)
        _validate_capture_flag(row_index, row, failures)
        _validate_comparisons(row_index, row, failures)

    return failures
