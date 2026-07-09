"""CLI entry points for Global Conquest Analytics.

Commands
--------
export-events
    Connect to Postgres, download the full game_events table, write
    data/raw/game_events.parquet.  Requires DATABASE_URL.

export-games
    Connect to Postgres, download games (id, name, event_history_complete),
    write data/raw/games.parquet. Requires DATABASE_URL.

combat-report
    Read data/raw/game_events.parquet (must already exist — run export-events
    first), normalise combat events, validate, generate charts and reports.
    Does NOT query Postgres.

roll-streak-report
    Read data/raw/game_events.parquet and data/raw/games.parquet (must
    already exist — run export-events and export-games first), detect
    attacking win/loss streaks and droughts, write a Markdown or JSON report.
    Does NOT query Postgres.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from global_conquest_analytics.streaks import RollStreakReport

_RAW_PARQUET = Path(__file__).parents[2] / "data" / "raw" / "game_events.parquet"
_GAMES_PARQUET = Path(__file__).parents[2] / "data" / "raw" / "games.parquet"


def export_events() -> None:
    """CLI entry point: extract game_events from Postgres → raw Parquet."""
    # Import here so missing DATABASE_URL only fails at runtime of this command.
    try:
        from global_conquest_analytics.config import get_database_url  # noqa: F401
    except RuntimeError as exc:
        print(f"Configuration error: {exc}", file=sys.stderr)
        sys.exit(1)

    try:
        from global_conquest_analytics.extract import extract_game_events

        extract_game_events()
    except RuntimeError as exc:
        print(f"Configuration error: {exc}", file=sys.stderr)
        sys.exit(1)
    except Exception as exc:  # noqa: BLE001
        print(f"Extraction failed: {exc}", file=sys.stderr)
        sys.exit(1)


def export_games() -> None:
    """CLI entry point: extract games from Postgres → raw Parquet."""
    try:
        from global_conquest_analytics.config import get_database_url  # noqa: F401
    except RuntimeError as exc:
        print(f"Configuration error: {exc}", file=sys.stderr)
        sys.exit(1)

    try:
        from global_conquest_analytics.extract import extract_games

        extract_games()
    except RuntimeError as exc:
        print(f"Configuration error: {exc}", file=sys.stderr)
        sys.exit(1)
    except Exception as exc:  # noqa: BLE001
        print(f"Extraction failed: {exc}", file=sys.stderr)
        sys.exit(1)


def generate_combat_report_command() -> None:
    """CLI entry point: normalise, validate, chart, and report on combat rolls."""
    if not _RAW_PARQUET.exists():
        print(
            f"Raw event file not found: {_RAW_PARQUET}\n"
            "Run `export-events` first to extract data from Postgres.",
            file=sys.stderr,
        )
        sys.exit(1)

    from global_conquest_analytics.charts import generate_all_charts
    from global_conquest_analytics.combat import process_combat_events
    from global_conquest_analytics.report import generate_report

    # 1. Normalise and write processed Parquet.
    combat_df = process_combat_events(_RAW_PARQUET)

    if combat_df.empty:
        print(
            "Warning: no combat_roll_resolved events found in the raw data. "
            "Generating an empty report.",
            file=sys.stderr,
        )

    # 2. Generate charts (graceful if empty).
    generate_all_charts(combat_df)

    # 3. Generate report + CSVs.
    generate_report(combat_df)


def generate_roll_streak_report_command() -> None:
    """CLI entry point: detect attacking win/loss streaks and droughts.

    Reads data/raw/game_events.parquet and data/raw/games.parquet (run
    export-events and export-games first). Does not query Postgres.
    """
    parser = argparse.ArgumentParser(
        prog="roll-streak-report",
        description="Report attacking win/loss streaks and droughts from captured combat rolls.",
    )
    parser.add_argument(
        "--game-id",
        default=None,
        help="Restrict the report to one game (default: most recent game)",
    )
    parser.add_argument(
        "--player-id", default=None, help="Restrict the report to one attacker's player id"
    )
    parser.add_argument("--min-loss-streak-length", type=int, default=2)
    parser.add_argument("--min-win-streak-length", type=int, default=2)
    parser.add_argument("--min-drought-length", type=int, default=3)
    parser.add_argument(
        "--top",
        type=int,
        default=5,
        help="Individual streaks shown per section in Markdown (0 = all)",
    )
    parser.add_argument("--format", choices=["markdown", "json"], default="markdown")
    parser.add_argument(
        "--include-partial-games",
        action="store_true",
        help="Required to proceed when the target game has partial event history",
    )
    args = parser.parse_args(sys.argv[1:])

    if not _RAW_PARQUET.exists():
        print(
            f"Raw event file not found: {_RAW_PARQUET}\nRun `export-events` first.", file=sys.stderr
        )
        sys.exit(1)
    if not _GAMES_PARQUET.exists():
        print(
            f"Raw games file not found: {_GAMES_PARQUET}\nRun `export-games` first.",
            file=sys.stderr,
        )
        sys.exit(1)

    import pandas as pd

    from global_conquest_analytics.combat import normalize_combat_events
    from global_conquest_analytics.streak_report import render_json, render_markdown
    from global_conquest_analytics.streaks import (
        StreakThresholds,
        build_roll_streak_report,
        rolls_from_combat_df,
    )

    raw_df = pd.read_parquet(_RAW_PARQUET, engine="pyarrow")
    combat_df = normalize_combat_events(raw_df)
    games_df = pd.read_parquet(_GAMES_PARQUET, engine="pyarrow")

    if combat_df.empty:
        print("No combat_roll_resolved events found in the raw data.", file=sys.stderr)
        sys.exit(1)

    game_id = args.game_id
    if game_id is None:
        game_id = combat_df.sort_values("occurred_at")["game_id"].iloc[-1]

    game_row = games_df[games_df["id"] == game_id]
    if game_row.empty:
        print(f"Game {game_id} not found in {_GAMES_PARQUET}.", file=sys.stderr)
        sys.exit(1)
    game_name = str(game_row["name"].iloc[0]) or game_id
    event_history_complete = bool(game_row["event_history_complete"].iloc[0])
    partial_history = not event_history_complete

    if partial_history and not args.include_partial_games:
        print(
            f"Game {game_id} has partial event history (streaks only reflect captured "
            "rolls after event logging began); pass --include-partial-games to generate "
            "the report anyway.",
            file=sys.stderr,
        )
        sys.exit(1)

    rolls, adapter_warnings = rolls_from_combat_df(
        combat_df[combat_df["game_id"] == game_id]
    )
    thresholds = StreakThresholds(
        min_loss_streak_length=args.min_loss_streak_length,
        min_win_streak_length=args.min_win_streak_length,
        min_drought_length=args.min_drought_length,
    )
    report = build_roll_streak_report(game_id, game_name, partial_history, rolls, {}, thresholds)
    report.warnings = adapter_warnings + report.warnings

    if args.player_id:
        report = _filter_report_by_player(report, args.player_id)

    if args.format == "json":
        print(render_json(report))
    else:
        print(render_markdown(report, top=args.top))


def _filter_report_by_player(report: RollStreakReport, player_id: str) -> RollStreakReport:
    import dataclasses

    return dataclasses.replace(
        report,
        summary_by_attacker=[
            s for s in report.summary_by_attacker if s.player_id == player_id
        ],
        attacking_loss_streaks=[
            s for s in report.attacking_loss_streaks if s.attacker_id == player_id
        ],
        attacking_win_streaks=[
            s for s in report.attacking_win_streaks if s.attacker_id == player_id
        ],
        attack_droughts=[
            s for s in report.attack_droughts if s.attacker_id == player_id
        ],
    )
