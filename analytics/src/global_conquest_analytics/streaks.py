"""Attacking roll-streak detection over normalised combat_roll_resolved events.

Pure, DataFrame-free core (Roll in, RollStreakReport out) so it can be unit
tested without pandas fixtures and reused by both the CLI and any future
caller. rolls_from_combat_df() is the only function that touches pandas —
it adapts combat.normalize_combat_events()'s output into Roll objects.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import Enum
from typing import Any

import pandas as pd


class RollResult(Enum):
    ATTACKER_WIN = "attacker_win"
    ATTACKER_LOSS = "attacker_loss"
    SPLIT = "split"


class StreakType(Enum):
    ATTACKING_LOSS = "attacking_loss"
    ATTACKING_WIN = "attacking_win"
    ATTACK_DROUGHT = "attack_drought"


@dataclass(frozen=True)
class Roll:
    """One decoded attack roll, ready for streak detection."""

    event_id: str
    game_id: str
    game_sequence: int
    occurred_at: datetime | None
    attacker_id: str
    defender_id: str
    attacker_territory: str
    defender_territory: str
    attacker_dice: list[int]
    defender_dice: list[int]
    attacker_losses: int
    defender_losses: int
    captured: bool
    comparisons: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class StreakRoll:
    """Display-ready detail for one roll within a Streak."""

    event_seq: int
    created_at: str
    defender_id: str
    defender_name: str
    attacker_territory: str
    defender_territory: str
    attack_dice: list[int]
    defend_dice: list[int]
    attacker_losses: int
    defender_losses: int
    captured: bool


@dataclass
class Streak:
    streak_id: str
    game_id: str
    game_name: str
    attacker_id: str
    attacker_name: str
    streak_type: StreakType
    streak_length: int
    start_event_seq: int
    end_event_seq: int
    start_time: str
    end_time: str
    defenders_involved: list[str]
    attacker_territories: list[str]
    defender_territories: list[str]
    attacker_armies_lost: int
    defender_armies_lost: int
    net_army_delta_for_attacker: int
    captures_during_streak: int
    roll_trace: str
    rolls: list[StreakRoll]


@dataclass
class PlayerStreakSummary:
    player_id: str
    player_name: str
    game_id: str
    game_name: str
    attack_rolls_captured: int
    attacker_win_count: int = 0
    attacker_loss_count: int = 0
    split_count: int = 0
    loss_streak_count_2_plus: int = 0
    longest_loss_streak: int = 0
    longest_loss_streak_id: str = ""
    win_streak_count_2_plus: int = 0
    longest_win_streak: int = 0
    longest_win_streak_id: str = ""
    attack_drought_count_3_plus: int = 0
    longest_attack_drought: int = 0
    longest_attack_drought_id: str = ""
    loss_streaks_per_20_attacks: float = 0.0
    win_streaks_per_20_attacks: float = 0.0
    droughts_per_20_attacks: float = 0.0


@dataclass(frozen=True)
class StreakThresholds:
    min_loss_streak_length: int = 2
    min_win_streak_length: int = 2
    min_drought_length: int = 3


@dataclass
class RollStreakReport:
    game_id: str
    game_name: str
    partial_history: bool
    warnings: list[str]
    summary_by_attacker: list[PlayerStreakSummary]
    attacking_loss_streaks: list[Streak]
    attacking_win_streaks: list[Streak]
    attack_droughts: list[Streak]


def classify_roll(roll: Roll) -> RollResult:
    """Classify a roll from the attacker's perspective."""
    if roll.defender_losses > roll.attacker_losses:
        return RollResult.ATTACKER_WIN
    if roll.attacker_losses > roll.defender_losses:
        return RollResult.ATTACKER_LOSS
    return RollResult.SPLIT


def _matches_streak_type(result: RollResult, streak_type: StreakType) -> bool:
    if streak_type is StreakType.ATTACKING_LOSS:
        return result is RollResult.ATTACKER_LOSS
    if streak_type is StreakType.ATTACKING_WIN:
        return result is RollResult.ATTACKER_WIN
    return result is not RollResult.ATTACKER_WIN  # ATTACK_DROUGHT


def _display_name(names: dict[str, str], player_id: str) -> str:
    name = names.get(player_id)
    if name:
        return name
    return player_id[:8] if len(player_id) >= 8 else player_id


_TIME_FORMAT = "%Y-%m-%dT%H:%M:%SZ"


def _fmt_time(dt: datetime | None) -> str:
    if dt is None:
        return ""
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    return dt.astimezone(UTC).strftime(_TIME_FORMAT)


def build_roll_streak_report(
    game_id: str,
    game_name: str,
    partial_history: bool,
    rolls: list[Roll],
    names: dict[str, str] | None = None,
    thresholds: StreakThresholds | None = None,
) -> RollStreakReport:
    """Pure core of the roll streak report.

    Args:
        game_id: Game this report is scoped to. Rolls belonging to a
            different game_id are defensively excluded — streaks never cross
            games even if the caller passes a mixed-game list.
        game_name: Display name for the game.
        partial_history: True when event capture may have begun mid-game.
        rolls: Decoded attack rolls (any order; sorted internally).
        names: player_id -> display name. Missing entries fall back to a
            truncated player_id.
        thresholds: Minimum streak lengths. Defaults to 2/2/3.

    Returns:
        Per-player summaries, individual qualifying streaks, and data-quality warnings.
    """
    names = names or {}
    thresholds = thresholds or StreakThresholds()

    warnings = list(_detect_data_warnings(rolls, game_id))
    if partial_history:
        warnings.append(
            "this game has partial event history. Streaks only reflect "
            "captured rolls after event logging began."
        )

    scoped = sorted(
        (r for r in rolls if r.game_id == game_id),
        key=lambda r: r.game_sequence,
    )

    by_attacker: dict[str, list[Roll]] = {}
    attacker_order: list[str] = []
    for r in scoped:
        if not r.attacker_id:
            continue
        if r.attacker_id not in by_attacker:
            attacker_order.append(r.attacker_id)
            by_attacker[r.attacker_id] = []
        by_attacker[r.attacker_id].append(r)

    summaries: list[PlayerStreakSummary] = []
    loss_streaks: list[Streak] = []
    win_streaks: list[Streak] = []
    droughts: list[Streak] = []

    for attacker_id in attacker_order:
        attacker_rolls = by_attacker[attacker_id]
        attacker_name = _display_name(names, attacker_id)

        summary = PlayerStreakSummary(
            player_id=attacker_id,
            player_name=attacker_name,
            game_id=game_id,
            game_name=game_name,
            attack_rolls_captured=len(attacker_rolls),
        )
        for r in attacker_rolls:
            result = classify_roll(r)
            if result is RollResult.ATTACKER_WIN:
                summary.attacker_win_count += 1
            elif result is RollResult.ATTACKER_LOSS:
                summary.attacker_loss_count += 1
            else:
                summary.split_count += 1

        player_loss_streaks = _detect_streaks(
            attacker_rolls, StreakType.ATTACKING_LOSS, thresholds.min_loss_streak_length,
            game_id, game_name, attacker_id, attacker_name, names,
        )
        player_win_streaks = _detect_streaks(
            attacker_rolls, StreakType.ATTACKING_WIN, thresholds.min_win_streak_length,
            game_id, game_name, attacker_id, attacker_name, names,
        )
        player_droughts = _detect_streaks(
            attacker_rolls, StreakType.ATTACK_DROUGHT, thresholds.min_drought_length,
            game_id, game_name, attacker_id, attacker_name, names,
        )

        summary.loss_streak_count_2_plus = len(player_loss_streaks)
        summary.win_streak_count_2_plus = len(player_win_streaks)
        summary.attack_drought_count_3_plus = len(player_droughts)

        if player_loss_streaks:
            longest = max(player_loss_streaks, key=lambda s: s.streak_length)
            summary.longest_loss_streak = longest.streak_length
            summary.longest_loss_streak_id = longest.streak_id
        if player_win_streaks:
            longest = max(player_win_streaks, key=lambda s: s.streak_length)
            summary.longest_win_streak = longest.streak_length
            summary.longest_win_streak_id = longest.streak_id
        if player_droughts:
            longest = max(player_droughts, key=lambda s: s.streak_length)
            summary.longest_attack_drought = longest.streak_length
            summary.longest_attack_drought_id = longest.streak_id

        if summary.attack_rolls_captured > 0:
            n = summary.attack_rolls_captured
            summary.loss_streaks_per_20_attacks = summary.loss_streak_count_2_plus / n * 20
            summary.win_streaks_per_20_attacks = summary.win_streak_count_2_plus / n * 20
            summary.droughts_per_20_attacks = summary.attack_drought_count_3_plus / n * 20

        summaries.append(summary)
        loss_streaks.extend(player_loss_streaks)
        win_streaks.extend(player_win_streaks)
        droughts.extend(player_droughts)

    _sort_streaks(loss_streaks)
    _sort_streaks(win_streaks)
    _sort_streaks(droughts)
    _sort_summaries(summaries)

    return RollStreakReport(
        game_id=game_id,
        game_name=game_name,
        partial_history=partial_history,
        warnings=warnings,
        summary_by_attacker=summaries,
        attacking_loss_streaks=loss_streaks,
        attacking_win_streaks=win_streaks,
        attack_droughts=droughts,
    )


def _detect_streaks(
    rolls: list[Roll],
    streak_type: StreakType,
    min_length: int,
    game_id: str,
    game_name: str,
    attacker_id: str,
    attacker_name: str,
    names: dict[str, str],
) -> list[Streak]:
    streaks: list[Streak] = []
    run: list[Roll] = []

    def flush() -> None:
        if len(run) >= min_length:
            streaks.append(
                _build_streak(
                    list(run), game_id, game_name, attacker_id, attacker_name, streak_type, names
                )
            )
        run.clear()

    for r in rolls:
        if _matches_streak_type(classify_roll(r), streak_type):
            run.append(r)
        else:
            flush()
    flush()

    return streaks


def _build_streak(
    run: list[Roll],
    game_id: str,
    game_name: str,
    attacker_id: str,
    attacker_name: str,
    streak_type: StreakType,
    names: dict[str, str],
) -> Streak:
    first, last = run[0], run[-1]

    seen_defenders: set[str] = set()
    seen_attacker_terr: set[str] = set()
    seen_defender_terr: set[str] = set()
    defenders_involved: list[str] = []
    attacker_territories: list[str] = []
    defender_territories: list[str] = []
    trace_parts: list[str] = []
    streak_rolls: list[StreakRoll] = []

    attacker_armies_lost = 0
    defender_armies_lost = 0
    captures = 0

    for r in run:
        defender_name = _display_name(names, r.defender_id)
        if defender_name not in seen_defenders:
            seen_defenders.add(defender_name)
            defenders_involved.append(defender_name)
        if r.attacker_territory not in seen_attacker_terr:
            seen_attacker_terr.add(r.attacker_territory)
            attacker_territories.append(r.attacker_territory)
        if r.defender_territory not in seen_defender_terr:
            seen_defender_terr.add(r.defender_territory)
            defender_territories.append(r.defender_territory)

        attacker_armies_lost += r.attacker_losses
        defender_armies_lost += r.defender_losses
        if r.captured:
            captures += 1

        trace_parts.append(f"{r.attacker_losses}-{r.defender_losses}")
        streak_rolls.append(
            StreakRoll(
                event_seq=r.game_sequence,
                created_at=_fmt_time(r.occurred_at),
                defender_id=r.defender_id,
                defender_name=defender_name,
                attacker_territory=r.attacker_territory,
                defender_territory=r.defender_territory,
                attack_dice=r.attacker_dice,
                defend_dice=r.defender_dice,
                attacker_losses=r.attacker_losses,
                defender_losses=r.defender_losses,
                captured=r.captured,
            )
        )

    streak_id = (
        f"{game_id}:{attacker_id}:{streak_type.value}:{first.game_sequence}-{last.game_sequence}"
    )
    return Streak(
        streak_id=streak_id,
        game_id=game_id,
        game_name=game_name,
        attacker_id=attacker_id,
        attacker_name=attacker_name,
        streak_type=streak_type,
        streak_length=len(run),
        start_event_seq=first.game_sequence,
        end_event_seq=last.game_sequence,
        start_time=_fmt_time(first.occurred_at),
        end_time=_fmt_time(last.occurred_at),
        defenders_involved=defenders_involved,
        attacker_territories=attacker_territories,
        defender_territories=defender_territories,
        attacker_armies_lost=attacker_armies_lost,
        defender_armies_lost=defender_armies_lost,
        net_army_delta_for_attacker=defender_armies_lost - attacker_armies_lost,
        captures_during_streak=captures,
        roll_trace=", ".join(trace_parts),
        rolls=streak_rolls,
    )


def _sort_streaks(streaks: list[Streak]) -> None:
    streaks.sort(key=lambda s: (-s.streak_length, s.start_time or "", s.start_event_seq))


def _sort_summaries(summaries: list[PlayerStreakSummary]) -> None:
    summaries.sort(
        key=lambda s: (
            -s.longest_attack_drought,
            -s.longest_loss_streak,
            -s.loss_streak_count_2_plus,
            -s.attack_rolls_captured,
        )
    )


def _detect_data_warnings(rolls: list[Roll], game_id: str) -> list[str]:
    """Surface suspicious data that survived decoding: duplicate sequences,
    zero-zero outcomes, and dice/loss-comparison mismatches."""
    warnings: list[str] = []
    seen_seq: dict[int, int] = {}

    for r in rolls:
        if r.game_id != game_id:
            continue
        if r.game_sequence <= 0:
            warnings.append(
                f"event {r.event_id} has a missing or invalid game_sequence ({r.game_sequence})"
            )
        seen_seq[r.game_sequence] = seen_seq.get(r.game_sequence, 0) + 1

        if r.attacker_losses == 0 and r.defender_losses == 0:
            warnings.append(
                f"event {r.event_id} (game_sequence {r.game_sequence}) has attacker_losses=0 "
                "and defender_losses=0, which is not a valid combat outcome"
            )

        if r.comparisons:
            expected = len(r.comparisons)
            actual = r.attacker_losses + r.defender_losses
            if actual != expected:
                warnings.append(
                    f"event {r.event_id} (game_sequence {r.game_sequence}) has "
                    f"attacker_losses+defender_losses ({actual}) != len(comparisons) ({expected})"
                )

    for seq in sorted(s for s, count in seen_seq.items() if count > 1):
        warnings.append(
            f"game_sequence {seq} appears {seen_seq[seq]} times (duplicate event sequence)"
        )

    return warnings


def rolls_from_combat_df(df: pd.DataFrame) -> tuple[list[Roll], list[str]]:
    """Adapt combat.normalize_combat_events()'s output into Roll objects.

    Rows missing data required to attribute a roll to an attacker (game_id,
    game_sequence, or attacker_player_id) are skipped and explained in the
    returned warnings list; everything else is passed through so suspicious
    but attributable data (e.g. 0-0 outcomes) stays visible in the report.

    Returns:
        (rolls, warnings)
    """
    warnings: list[str] = []
    if df.empty:
        return [], warnings

    rolls: list[Roll] = []
    for _, row in df.iterrows():
        event_id = str(row.get("id", ""))
        game_id = row.get("game_id")
        game_sequence = row.get("game_sequence")
        attacker_id = row.get("attacker_player_id")

        if not game_id or pd.isna(game_sequence) or not attacker_id:
            warnings.append(
                f"event {event_id} is missing game_id, game_sequence, "
                "or attacker_player_id; skipped"
            )
            continue

        defender_id = row.get("defender_player_id") or ""
        if not defender_id:
            warnings.append(
                f"event {event_id} (game_sequence {game_sequence}) is missing defender_player_id"
            )

        attacker_losses = row.get("attacker_losses")
        defender_losses = row.get("defender_losses")
        if attacker_losses is None or pd.isna(attacker_losses):
            warnings.append(
                f"event {event_id} (game_sequence {game_sequence}) is missing "
                "attacker_losses; treated as 0"
            )
            attacker_losses = 0
        if defender_losses is None or pd.isna(defender_losses):
            warnings.append(
                f"event {event_id} (game_sequence {game_sequence}) is missing "
                "defender_losses; treated as 0"
            )
            defender_losses = 0
        attacker_losses = int(attacker_losses)
        defender_losses = int(defender_losses)
        if attacker_losses < 0 or defender_losses < 0:
            warnings.append(
                f"event {event_id} (game_sequence {game_sequence}) has negative losses; skipped"
            )
            continue

        attacker_dice = _coerce_dice(row.get("attacker_dice"))
        defender_dice = _coerce_dice(row.get("defender_dice"))
        if attacker_dice is None or defender_dice is None:
            warnings.append(
                f"event {event_id} (game_sequence {game_sequence}) has a malformed dice array"
            )
            attacker_dice = attacker_dice or []
            defender_dice = defender_dice or []

        occurred_at = row.get("occurred_at")
        if occurred_at is not None and pd.notna(occurred_at):
            occurred_at = pd.Timestamp(occurred_at).to_pydatetime()
        else:
            occurred_at = None

        comparisons_raw = row.get("comparisons")
        comparisons = list(comparisons_raw) if isinstance(comparisons_raw, list) else []

        rolls.append(
            Roll(
                event_id=event_id,
                game_id=str(game_id),
                game_sequence=int(game_sequence),
                occurred_at=occurred_at,
                attacker_id=str(attacker_id),
                defender_id=str(defender_id),
                attacker_territory=str(row.get("source_territory_id") or ""),
                defender_territory=str(row.get("target_territory_id") or ""),
                attacker_dice=attacker_dice,
                defender_dice=defender_dice,
                attacker_losses=attacker_losses,
                defender_losses=defender_losses,
                captured=bool(row.get("territory_captured", False)),
                comparisons=comparisons,
            )
        )

    return rolls, warnings


def _coerce_dice(raw: Any) -> list[int] | None:
    if raw is None:
        return None
    try:
        dice = list(raw)
    except TypeError:
        return None
    if not dice:
        return None
    try:
        return [int(d) for d in dice]
    except (TypeError, ValueError):
        return None
