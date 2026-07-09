"""Unit tests for streaks.py — no live database required."""

from __future__ import annotations

from datetime import UTC, datetime

from global_conquest_analytics.streaks import (
    Roll,
    RollResult,
    StreakThresholds,
    build_roll_streak_report,
    classify_roll,
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def mk_roll(
    seq: int,
    attacker_id: str,
    defender_id: str,
    attacker_losses: int,
    defender_losses: int,
    *,
    game_id: str = "g1",
    attacker_territory: str = "alaska",
    defender_territory: str = "kamchatka",
    captured: bool = False,
) -> Roll:
    return Roll(
        event_id=f"ev-{seq}",
        game_id=game_id,
        game_sequence=seq,
        occurred_at=datetime.fromtimestamp(seq, tz=UTC),
        attacker_id=attacker_id,
        defender_id=defender_id,
        attacker_territory=attacker_territory,
        defender_territory=defender_territory,
        attacker_dice=[6],
        defender_dice=[1],
        attacker_losses=attacker_losses,
        defender_losses=defender_losses,
        captured=captured,
    )


def default_thresholds() -> StreakThresholds:
    return StreakThresholds()


# ---------------------------------------------------------------------------
# Classification
# ---------------------------------------------------------------------------


class TestClassifyRoll:
    def test_attacker_win(self) -> None:
        assert classify_roll(mk_roll(1, "a", "b", 0, 1)) is RollResult.ATTACKER_WIN

    def test_attacker_loss(self) -> None:
        assert classify_roll(mk_roll(1, "a", "b", 1, 0)) is RollResult.ATTACKER_LOSS

    def test_split(self) -> None:
        assert classify_roll(mk_roll(1, "a", "b", 1, 1)) is RollResult.SPLIT


# ---------------------------------------------------------------------------
# Streak detection
# ---------------------------------------------------------------------------


class TestStreakDetection:
    def test_two_consecutive_losses_create_streak(self) -> None:
        rolls = [mk_roll(1, "a", "b", 1, 0), mk_roll(2, "a", "b", 1, 0)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_loss_streaks) == 1
        assert r.attacking_loss_streaks[0].streak_length == 2

    def test_single_loss_below_threshold(self) -> None:
        rolls = [mk_roll(1, "a", "b", 1, 0), mk_roll(2, "a", "b", 0, 1)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.attacking_loss_streaks == []

    def test_split_breaks_strict_loss_streak(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "b", 1, 1),
            mk_roll(3, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.attacking_loss_streaks == []

    def test_split_contributes_to_drought(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "b", 1, 1),
            mk_roll(3, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attack_droughts) == 1
        assert r.attack_droughts[0].streak_length == 3

    def test_attacker_win_breaks_drought(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "b", 1, 1),
            mk_roll(3, "a", "b", 0, 1),
            mk_roll(4, "a", "b", 1, 0),
            mk_roll(5, "a", "b", 1, 1),
        ]
        thresholds = StreakThresholds(min_drought_length=2)
        r = build_roll_streak_report("g1", "Test Game", False, rolls, thresholds=thresholds)
        assert len(r.attack_droughts) == 2
        assert all(d.streak_length == 2 for d in r.attack_droughts)

    def test_two_consecutive_wins_create_streak(self) -> None:
        rolls = [mk_roll(1, "a", "b", 0, 1), mk_roll(2, "a", "b", 0, 1)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_win_streaks) == 1
        assert r.attacking_win_streaks[0].streak_length == 2

    def test_streaks_do_not_cross_attacker_boundaries(self) -> None:
        rolls = [
            mk_roll(1, "a", "x", 1, 0),
            mk_roll(2, "b", "x", 1, 0),
            mk_roll(3, "a", "x", 1, 0),
            mk_roll(4, "b", "x", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_loss_streaks) == 2
        attackers = {s.attacker_id for s in r.attacking_loss_streaks}
        assert attackers == {"a", "b"}
        assert all(s.streak_length == 2 for s in r.attacking_loss_streaks)

    def test_streaks_do_not_cross_game_boundaries(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0, game_id="g1"),
            mk_roll(2, "a", "b", 1, 0, game_id="OTHER_GAME"),
            mk_roll(3, "a", "b", 1, 0, game_id="g1"),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_loss_streaks) == 1
        s = r.attacking_loss_streaks[0]
        assert s.streak_length == 2
        assert all(roll.event_seq != 2 for roll in s.rolls)
        assert r.summary_by_attacker[0].attack_rolls_captured == 2

    def test_streaks_ordered_by_event_sequence(self) -> None:
        rolls = [
            mk_roll(3, "a", "b", 1, 0),
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_loss_streaks) == 1
        s = r.attacking_loss_streaks[0]
        assert s.streak_length == 3
        assert [roll.event_seq for roll in s.rolls] == [1, 2, 3]

    def test_end_of_input_streak_emitted(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 0, 1),
            mk_roll(2, "a", "b", 1, 0),
            mk_roll(3, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_loss_streaks) == 1
        assert r.attacking_loss_streaks[0].end_event_seq == 3


# ---------------------------------------------------------------------------
# Summary aggregation
# ---------------------------------------------------------------------------


class TestSummaryAggregation:
    def test_counts_streaks_per_player(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "b", 1, 0),
            mk_roll(3, "a", "b", 0, 1),
            mk_roll(4, "a", "b", 1, 0),
            mk_roll(5, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.summary_by_attacker[0].loss_streak_count_2_plus == 2

    def test_longest_streak_ids(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "b", 1, 0),
            mk_roll(3, "a", "b", 1, 0),
            mk_roll(4, "a", "b", 0, 1),
            mk_roll(5, "a", "b", 1, 0),
            mk_roll(6, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        s = r.summary_by_attacker[0]
        assert s.longest_loss_streak == 3
        assert s.longest_loss_streak_id == "g1:a:attacking_loss:1-3"

    def test_wls_totals(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 0, 1),
            mk_roll(2, "a", "b", 1, 0),
            mk_roll(3, "a", "b", 1, 1),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        s = r.summary_by_attacker[0]
        assert (s.attacker_win_count, s.attacker_loss_count, s.split_count) == (1, 1, 1)

    def test_per_20_attack_rates(self) -> None:
        rolls = []
        for i in range(1, 21):
            if i in (1, 2, 5, 6):
                rolls.append(mk_roll(i, "a", "b", 1, 0))
            else:
                rolls.append(mk_roll(i, "a", "b", 0, 1))
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        s = r.summary_by_attacker[0]
        assert s.attack_rolls_captured == 20
        assert s.loss_streaks_per_20_attacks == 2.0

    def test_zero_rolls_produces_no_summaries(self) -> None:
        r = build_roll_streak_report("g1", "Test Game", False, [])
        assert r.summary_by_attacker == []

    def test_player_with_zero_streaks(self) -> None:
        rolls = [mk_roll(i, "a", "b", 0, 1) for i in range(1, 4)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        s = r.summary_by_attacker[0]
        assert s.loss_streak_count_2_plus == 0
        assert s.attack_drought_count_3_plus == 0
        assert s.longest_loss_streak_id == ""

    def test_missing_player_names_fall_back_to_id(self) -> None:
        rolls = [
            mk_roll(1, "12345678-aaaa-bbbb-cccc-dddddddddddd", "b", 1, 0),
            mk_roll(2, "12345678-aaaa-bbbb-cccc-dddddddddddd", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.summary_by_attacker[0].player_name == "12345678"


# ---------------------------------------------------------------------------
# Individual streak details
# ---------------------------------------------------------------------------


class TestIndividualStreakDetails:
    def test_defenders_involved_collected(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 1, 0),
            mk_roll(2, "a", "c", 1, 0),
            mk_roll(3, "a", "b", 1, 0),
        ]
        r = build_roll_streak_report(
            "g1", "Test Game", False, rolls, names={"b": "Bob", "c": "Carol"}
        )
        assert r.attacking_loss_streaks[0].defenders_involved == ["Bob", "Carol"]

    def test_territories_collected(self) -> None:
        rolls = [
            mk_roll(
                1, "a", "b", 1, 0, attacker_territory="ukraine", defender_territory="afghanistan"
            ),
            mk_roll(
                2, "a", "b", 1, 0, attacker_territory="middle_east", defender_territory="india"
            ),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        s = r.attacking_loss_streaks[0]
        assert s.attacker_territories == ["ukraine", "middle_east"]
        assert s.defender_territories == ["afghanistan", "india"]

    def test_army_loss_sums_and_net_delta(self) -> None:
        rolls = [mk_roll(1, "a", "b", 2, 0), mk_roll(2, "a", "b", 1, 1)]
        thresholds = StreakThresholds(
            min_loss_streak_length=99, min_win_streak_length=99, min_drought_length=2
        )
        r = build_roll_streak_report("g1", "Test Game", False, rolls, thresholds=thresholds)
        s = r.attack_droughts[0]
        assert s.attacker_armies_lost == 3
        assert s.defender_armies_lost == 1
        assert s.net_army_delta_for_attacker == -2

    def test_captures_counted(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 0, 1, captured=True),
            mk_roll(2, "a", "b", 0, 1, captured=False),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.attacking_win_streaks[0].captures_during_streak == 1

    def test_roll_trace_rendered(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 2, 0),
            mk_roll(2, "a", "b", 1, 0),
            mk_roll(3, "a", "b", 1, 1),
        ]
        thresholds = StreakThresholds(
            min_loss_streak_length=99, min_win_streak_length=99, min_drought_length=3
        )
        r = build_roll_streak_report("g1", "Test Game", False, rolls, thresholds=thresholds)
        assert r.attack_droughts[0].roll_trace == "2-0, 1-0, 1-1"

    def test_streak_id_format(self) -> None:
        rolls = [mk_roll(144, "tucker", "nick", 1, 0), mk_roll(145, "tucker", "nick", 1, 0)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.attacking_loss_streaks[0].streak_id == "g1:tucker:attacking_loss:144-145"

    def test_zero_zero_roll_included_in_trace_not_hidden(self) -> None:
        rolls = [mk_roll(1, "a", "b", 1, 0), mk_roll(2, "a", "b", 0, 0)]
        thresholds = StreakThresholds(
            min_loss_streak_length=99, min_win_streak_length=99, min_drought_length=2
        )
        r = build_roll_streak_report("g1", "Test Game", False, rolls, thresholds=thresholds)
        assert r.attack_droughts[0].roll_trace == "1-0, 0-0"


# ---------------------------------------------------------------------------
# Partial history
# ---------------------------------------------------------------------------


class TestPartialHistory:
    def test_partial_history_warning_present(self) -> None:
        rolls = [mk_roll(1, "a", "b", 0, 1)]
        r = build_roll_streak_report("g1", "Test Game", True, rolls)
        assert r.partial_history is True
        assert any("partial event history" in w for w in r.warnings)

    def test_no_partial_history_warning_when_complete(self) -> None:
        rolls = [mk_roll(1, "a", "b", 0, 1)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert r.partial_history is False
        assert not any("partial event history" in w for w in r.warnings)


# ---------------------------------------------------------------------------
# Bad data warnings
# ---------------------------------------------------------------------------


class TestBadDataWarnings:
    def test_zero_zero_outcome_warns(self) -> None:
        rolls = [mk_roll(1, "a", "b", 0, 0)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert any("not a valid combat outcome" in w for w in r.warnings)

    def test_duplicate_sequence_warns(self) -> None:
        rolls = [mk_roll(1, "a", "b", 1, 0), mk_roll(1, "a", "b", 0, 1)]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert any("duplicate event sequence" in w for w in r.warnings)

    def test_one_bad_roll_does_not_corrupt_other_attackers_streaks(self) -> None:
        rolls = [
            mk_roll(1, "a", "b", 0, 0),  # suspicious but attributable
            mk_roll(2, "c", "b", 1, 0),
            mk_roll(3, "c", "b", 1, 0),
        ]
        r = build_roll_streak_report("g1", "Test Game", False, rolls)
        assert len(r.attacking_loss_streaks) == 1
        assert r.attacking_loss_streaks[0].attacker_id == "c"


# ---------------------------------------------------------------------------
# rolls_from_combat_df
# ---------------------------------------------------------------------------


class TestRollsFromCombatDF:
    def test_missing_attacker_id_skipped_with_warning(self) -> None:
        import pandas as pd
        from global_conquest_analytics.streaks import rolls_from_combat_df

        df = pd.DataFrame(
            [
                {
                    "id": "ev-1",
                    "game_id": "g1",
                    "game_sequence": 1,
                    "attacker_player_id": None,
                    "defender_player_id": "b",
                    "attacker_losses": 1,
                    "defender_losses": 0,
                    "attacker_dice": [6],
                    "defender_dice": [1],
                    "source_territory_id": "x",
                    "target_territory_id": "y",
                    "territory_captured": False,
                    "occurred_at": pd.Timestamp("2024-01-01", tz="UTC"),
                }
            ]
        )
        rolls, warnings = rolls_from_combat_df(df)
        assert rolls == []
        assert any("skipped" in w for w in warnings)

    def test_valid_row_converted(self) -> None:
        import pandas as pd
        from global_conquest_analytics.streaks import rolls_from_combat_df

        df = pd.DataFrame(
            [
                {
                    "id": "ev-1",
                    "game_id": "g1",
                    "game_sequence": 1,
                    "attacker_player_id": "a",
                    "defender_player_id": "b",
                    "attacker_losses": 1,
                    "defender_losses": 0,
                    "attacker_dice": [6, 5],
                    "defender_dice": [1],
                    "source_territory_id": "x",
                    "target_territory_id": "y",
                    "territory_captured": False,
                    "occurred_at": pd.Timestamp("2024-01-01", tz="UTC"),
                }
            ]
        )
        rolls, warnings = rolls_from_combat_df(df)
        assert len(rolls) == 1
        assert warnings == []
        assert rolls[0].attacker_id == "a"
        assert rolls[0].attacker_dice == [6, 5]
