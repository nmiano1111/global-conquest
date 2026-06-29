"""CLI entry points for Global Conquest Analytics.

Commands
--------
export-events
    Connect to Postgres, download the full game_events table, write
    data/raw/game_events.parquet.  Requires DATABASE_URL.

combat-report
    Read data/raw/game_events.parquet (must already exist — run export-events
    first), normalise combat events, validate, generate charts and reports.
    Does NOT query Postgres.
"""

from __future__ import annotations

import sys
from pathlib import Path

_RAW_PARQUET = Path(__file__).parents[2] / "data" / "raw" / "game_events.parquet"


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
