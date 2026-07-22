"""CLI entry points for Global Conquest Analytics.

Commands
--------
export-events
    Connect to Postgres, download the full game_events table, write
    data/raw/game_events.parquet.  Requires DATABASE_URL.

export-games
    Connect to Postgres, download games (id, name, event_history_complete),
    write data/raw/games.parquet. Requires DATABASE_URL.

export-players
    Connect to Postgres, download users (id, username), write
    data/raw/players.parquet. Requires DATABASE_URL. Used to resolve
    attacker/defender display names instead of showing raw UUIDs.

combat-report
    Read data/raw/game_events.parquet (must already exist — run export-events
    first), normalise combat events, validate, generate charts and reports.
    Does NOT query Postgres.

roll-streak-report
    Read data/raw/game_events.parquet and data/raw/games.parquet (must
    already exist — run export-events and export-games first), detect
    attacking win/loss streaks and droughts, write a Markdown or JSON report.
    Reads data/raw/players.parquet if present (run export-players) to show
    usernames instead of UUIDs. Does NOT query Postgres.

fit-board-value
    Read backend/cmd/tdtraindata's JSONL output (backend/data/raw/tdtraindata/*_train.jsonl
    by default — run `go run ./cmd/tdtraindata` first), fit a single whole-board
    linear value function via logistic regression, and export a weights JSON
    file (reports/generated/board_value/<timestamp>.json by default) ready
    for `cmd/tournament --board-value-variant`. Does NOT query Postgres.

fit-gcn
    Same input as fit-board-value, but fits a supervised Graph
    Convolutional Network (see gcn_fit.py's module docstring) instead of
    a linear model, and exports a weights JSON file
    (reports/generated/gcn/<timestamp>.json by default) ready for
    `cmd/tournament --gcn-variant`. Run `go run ./cmd/bvcalibrate
    --model-type gcn` against the output before live use (margins are
    0.0 placeholders here, same as fit-board-value). Does NOT query
    Postgres.
"""

from __future__ import annotations

import argparse
import sys
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import pandas as pd

    from global_conquest_analytics.streaks import RollStreakReport
    from global_conquest_analytics.td_fit import Episode

_RAW_PARQUET = Path(__file__).parents[2] / "data" / "raw" / "game_events.parquet"
_GAMES_PARQUET = Path(__file__).parents[2] / "data" / "raw" / "games.parquet"
_PLAYERS_PARQUET = Path(__file__).parents[2] / "data" / "raw" / "players.parquet"
_ROLL_STREAK_REPORT_DIR = Path(__file__).parents[2] / "reports" / "generated" / "roll_streaks"
# backend/cmd/tdtraindata is a Go tool that writes relative to wherever
# it's invoked (typically backend/, per its own README's examples), not
# relative to this package -- unlike the parents[2]-based paths above,
# which are all analytics-internal (produced by this package's own
# export-* commands), this one points at backend/'s own data directory.
_TDTRAINDATA_DIR = Path(__file__).parents[3] / "backend" / "data" / "raw" / "tdtraindata"
_BOARD_VALUE_REPORT_DIR = Path(__file__).parents[2] / "reports" / "generated" / "board_value"
_GCN_REPORT_DIR = Path(__file__).parents[2] / "reports" / "generated" / "gcn"


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


def export_players() -> None:
    """CLI entry point: extract players (users) from Postgres → raw Parquet."""
    try:
        from global_conquest_analytics.config import get_database_url  # noqa: F401
    except RuntimeError as exc:
        print(f"Configuration error: {exc}", file=sys.stderr)
        sys.exit(1)

    try:
        from global_conquest_analytics.extract import extract_players

        extract_players()
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
        help="Restrict the report to one game by id (default: most recent game)",
    )
    parser.add_argument(
        "--game-name",
        default=None,
        help="Restrict the report to one game by name (case-insensitive). "
        "Use --list-games to see available names.",
    )
    parser.add_argument(
        "--list-games",
        action="store_true",
        help="List available games (id, name, event history status) and exit",
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
    parser.add_argument(
        "--output",
        default=None,
        help=(
            "File path to write the report to (default: "
            "reports/generated/roll_streaks/<game-name>.md|json). "
            "Pass '-' to write only to stdout."
        ),
    )
    args = parser.parse_args(sys.argv[1:])

    if not _GAMES_PARQUET.exists():
        print(
            f"Raw games file not found: {_GAMES_PARQUET}\nRun `export-games` first.",
            file=sys.stderr,
        )
        sys.exit(1)

    import pandas as pd

    games_df = pd.read_parquet(_GAMES_PARQUET, engine="pyarrow")

    if args.list_games:
        _print_game_list(games_df)
        return

    if not _RAW_PARQUET.exists():
        print(
            f"Raw event file not found: {_RAW_PARQUET}\nRun `export-events` first.", file=sys.stderr
        )
        sys.exit(1)
    if args.game_id and args.game_name:
        print("Pass only one of --game-id or --game-name, not both.", file=sys.stderr)
        sys.exit(1)

    from global_conquest_analytics.combat import normalize_combat_events
    from global_conquest_analytics.streak_report import render_json, render_markdown
    from global_conquest_analytics.streaks import (
        StreakThresholds,
        build_roll_streak_report,
        rolls_from_combat_df,
    )

    raw_df = pd.read_parquet(_RAW_PARQUET, engine="pyarrow")
    combat_df = normalize_combat_events(raw_df)

    if combat_df.empty:
        print("No combat_roll_resolved events found in the raw data.", file=sys.stderr)
        sys.exit(1)

    if args.game_name:
        game_row = games_df[games_df["name"].str.lower() == args.game_name.lower()]
        if game_row.empty:
            print(
                f"No game named {args.game_name!r} found. Use --list-games to see "
                "available names.",
                file=sys.stderr,
            )
            sys.exit(1)
        game_id = str(game_row["id"].iloc[0])
    elif args.game_id:
        game_id = args.game_id
        game_row = games_df[games_df["id"] == game_id]
        if game_row.empty:
            print(f"Game {game_id} not found in {_GAMES_PARQUET}.", file=sys.stderr)
            sys.exit(1)
    else:
        game_id = str(combat_df.sort_values("occurred_at")["game_id"].iloc[-1])
        game_row = games_df[games_df["id"] == game_id]
        if game_row.empty:
            print(f"Game {game_id} not found in {_GAMES_PARQUET}.", file=sys.stderr)
            sys.exit(1)

    game_name = str(game_row["name"].iloc[0]) or game_id
    event_history_complete = bool(game_row["event_history_complete"].iloc[0])
    partial_history = not event_history_complete

    if partial_history and not args.include_partial_games:
        print(
            f"Game {game_name!r} has partial event history (streaks only reflect captured "
            "rolls after event logging began); pass --include-partial-games to generate "
            "the report anyway.",
            file=sys.stderr,
        )
        sys.exit(1)

    names = _load_player_names()

    rolls, adapter_warnings = rolls_from_combat_df(
        combat_df[combat_df["game_id"] == game_id]
    )
    thresholds = StreakThresholds(
        min_loss_streak_length=args.min_loss_streak_length,
        min_win_streak_length=args.min_win_streak_length,
        min_drought_length=args.min_drought_length,
    )
    report = build_roll_streak_report(game_id, game_name, partial_history, rolls, names, thresholds)
    report.warnings = adapter_warnings + report.warnings

    if args.player_id:
        report = _filter_report_by_player(report, args.player_id)

    extension = "json" if args.format == "json" else "md"
    body = render_json(report) if args.format == "json" else render_markdown(report, top=args.top)

    if args.output == "-":
        print(body)
        return

    output_path = (
        Path(args.output)
        if args.output
        else _ROLL_STREAK_REPORT_DIR / f"{_slugify(game_name)}.{extension}"
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(body, encoding="utf-8")
    print(f"Report written → {output_path}")


def _print_game_list(games_df: pd.DataFrame) -> None:
    if games_df.empty:
        print("No games found. Run `export-games` first.")
        return
    print(f"{'Game Name':<30} {'Event History':<15} Game ID")
    print(f"{'-' * 30} {'-' * 15} {'-' * 36}")
    for _, row in games_df.sort_values("name").iterrows():
        status = "complete" if row["event_history_complete"] else "partial"
        print(f"{row['name']:<30} {status:<15} {row['id']}")


def _load_player_names() -> dict[str, str]:
    """Load id -> username from data/raw/players.parquet, if present.

    Missing file is not an error: the report falls back to truncated player
    ids, but a note is printed pointing at `export-players`.
    """
    if not _PLAYERS_PARQUET.exists():
        print(
            f"Note: {_PLAYERS_PARQUET} not found — showing player ids instead of "
            "usernames. Run `export-players` to enable names.",
            file=sys.stderr,
        )
        return {}

    import pandas as pd

    players_df = pd.read_parquet(_PLAYERS_PARQUET, engine="pyarrow")
    return dict(zip(players_df["id"], players_df["username"], strict=True))


def _slugify(name: str) -> str:
    slug = "".join(c.lower() if c.isalnum() else "-" for c in name).strip("-")
    while "--" in slug:
        slug = slug.replace("--", "-")
    return slug or "game"


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


def _resolve_tdtraindata_inputs(args_input: list[str] | None) -> list[Path]:
    """Shared --input resolution for fit-board-value/fit-gcn: explicit
    paths, or every *_train.jsonl under backend/data/raw/tdtraindata/. Exits with
    an error (matching both commands' prior behavior) if neither yields
    anything.
    """
    input_paths = (
        [Path(p) for p in args_input]
        if args_input
        else sorted(_TDTRAINDATA_DIR.glob("*_train.jsonl"))
    )
    if not input_paths:
        print(
            f"No training files found under {_TDTRAINDATA_DIR}.\n"
            "Run `go run ./cmd/tdtraindata` first.",
            file=sys.stderr,
        )
        sys.exit(1)
    return input_paths


def _load_tdtraindata_episodes(input_paths: list[Path]) -> tuple[list[Episode], list[str]]:
    """Shared episode + feature-name loading for fit-board-value/fit-gcn,
    printing the same "loading N files" summary both commands already
    did. Callers still print their own final "loaded M episodes..." line
    since fit-gcn's also reports the board's node count.
    """
    from global_conquest_analytics.td_fit import load_episodes, load_feature_names

    print(f"Loading {len(input_paths)} training file(s):")
    for p in input_paths:
        print(f"  {p}")
    episodes = load_episodes(input_paths)
    # Feature names are identical across every input file (one board, the
    # classic map -- see cmd/tdtraindata's featureNamesPath/writeFeatureNames
    # doc comment), so any one file's sidecar suffices.
    feature_names = load_feature_names(input_paths[0].with_suffix(".featurenames.json"))
    return episodes, feature_names


def fit_board_value_command() -> None:
    """CLI entry point: fit a whole-board linear value function, export weights.json.

    Reads backend/data/raw/tdtraindata/*_train.jsonl by default (run
    `go run ./cmd/tdtraindata` first). Does not query Postgres.
    """
    parser = argparse.ArgumentParser(
        prog="fit-board-value",
        description="Fit a whole-board value function from cmd/tdtraindata rows.",
    )
    parser.add_argument(
        "--input",
        action="append",
        default=None,
        help=(
            "Training JSONL file (repeatable). Default: every "
            "*_train.jsonl under backend/data/raw/tdtraindata/."
        ),
    )
    parser.add_argument(
        "--output",
        default=None,
        help=(
            "Destination for the fitted value JSON (default: "
            "reports/generated/board_value/<timestamp>.json)."
        ),
    )
    args = parser.parse_args(sys.argv[1:])

    input_paths = _resolve_tdtraindata_inputs(args.input)

    from global_conquest_analytics.board_fit import export_board_value, fit_board_value

    episodes, feature_names = _load_tdtraindata_episodes(input_paths)
    n_pairs = len({(ep.game_id, ep.player_id) for ep in episodes})
    print(
        f"Loaded {len(episodes)} episodes ({n_pairs} game/player pairs), "
        f"{len(feature_names)} features.\n"
    )

    fit = fit_board_value(episodes, feature_names)
    print(f"Fitted {len(fit.weights)} weights, intercept={fit.intercept:.4f}")

    output_path = (
        Path(args.output)
        if args.output
        else _BOARD_VALUE_REPORT_DIR / f"{datetime.now(UTC).strftime('%Y%m%d-%H%M%S')}.json"
    )
    export_board_value(fit, output_path)
    print(f"Board value written → {output_path}")


def fit_gcn_command() -> None:
    """CLI entry point: fit a supervised GCN value function, export weights.json.

    Reads backend/data/raw/tdtraindata/*_train.jsonl by default (run
    `go run ./cmd/tdtraindata` first). Does not query Postgres.
    """
    parser = argparse.ArgumentParser(
        prog="fit-gcn",
        description="Fit a supervised GCN value function from cmd/tdtraindata rows.",
    )
    parser.add_argument(
        "--input",
        action="append",
        default=None,
        help=(
            "Training JSONL file (repeatable). Default: every "
            "*_train.jsonl under backend/data/raw/tdtraindata/."
        ),
    )
    parser.add_argument(
        "--output",
        default=None,
        help=(
            "Destination for the fitted GCN weights JSON (default: "
            "reports/generated/gcn/<timestamp>.json)."
        ),
    )
    parser.add_argument(
        "--epochs",
        type=int,
        default=20,
        help=(
            "Training epochs (default: 20 -- for --objective td, this is per-timestep "
            "sequential and dramatically slower per epoch than the default supervised "
            "objective's vectorized full-batch pass; time a small run with --input pointed "
            "at one smaller *_train.jsonl file before committing to a large one, and "
            "consider passing a much lower value, e.g. 1-3)."
        ),
    )
    parser.add_argument(
        "--objective",
        choices=["supervised", "td"],
        default="supervised",
        help=(
            "supervised (default): regress every turn-boundary row directly toward that "
            "player's eventual Won/Loss (fit_gcn, BCEWithLogitsLoss, full-batch). "
            "td: semi-gradient TD(lambda) with eligibility traces (fit_gcn_td), "
            "bootstrapping between temporally close states instead -- see gcn_fit.py's "
            "module docstring and fit_gcn_td's own docstring for why this exists."
        ),
    )
    parser.add_argument(
        "--alpha",
        type=float,
        default=1e-3,
        help=(
            "Plain-SGD learning rate for the TD(lambda) eligibility-trace update, only "
            "used with --objective td (default: 1e-3, matching td_fit.fit_td_lambda's own "
            "validated default -- gcn_fit.fit_gcn_td's own docstring explains why this is "
            "plain SGD rather than Adam)."
        ),
    )
    parser.add_argument(
        "--lam",
        type=float,
        default=0.8,
        help="TD(lambda) eligibility-trace decay, only used with --objective td (default: 0.8).",
    )
    parser.add_argument(
        "--td-error-clip",
        type=float,
        default=5.0,
        help=(
            "Bound on |target - V(s_t)| before applying a TD(lambda) update, only used "
            "with --objective td (default: 5.0)."
        ),
    )
    args = parser.parse_args(sys.argv[1:])

    input_paths = _resolve_tdtraindata_inputs(args.input)

    from global_conquest_analytics.gcn_fit import export_gcn, fit_gcn, fit_gcn_td, load_board_schema

    episodes, feature_names = _load_tdtraindata_episodes(input_paths)
    # Board topology is identical across every input file (one board, the
    # classic map), so any one file's sidecar suffices.
    schema = load_board_schema(input_paths[0].with_suffix(".boardschema.json"))
    n_pairs = len({(ep.game_id, ep.player_id) for ep in episodes})
    print(
        f"Loaded {len(episodes)} episodes ({n_pairs} game/player pairs), "
        f"{len(feature_names)} features, {len(schema.order)}-node board.\n"
    )

    if args.objective == "td":
        print(
            f"objective=td: fitting sequentially, episode by episode -- this is much slower "
            f"per epoch than --objective supervised's batched pass. Running {args.epochs} "
            f"epoch(s) across {n_pairs} episodes; if this is your first run, consider "
            f"Ctrl-C and retrying with --epochs 1 and a smaller --input first.\n"
        )

        start = time.monotonic()
        last_print = 0.0

        def on_progress(epoch: int, total_epochs: int, episode: int, total_episodes: int) -> None:
            nonlocal last_print
            now = time.monotonic()
            at_boundary = episode == total_episodes  # always show each epoch's 100%
            if now - last_print < 2.0 and not at_boundary:
                return
            last_print = now
            elapsed = now - start
            done = (epoch - 1) * total_episodes + episode
            total = total_epochs * total_episodes
            rate = done / elapsed if elapsed > 0 else 0.0
            eta = (total - done) / rate if rate > 0 else 0.0
            print(
                f"\r  epoch {epoch}/{total_epochs}, episode {episode}/{total_episodes} "
                f"({100 * done / total:.1f}%) -- elapsed {elapsed:.0f}s, eta {eta:.0f}s",
                end="",
                flush=True,
            )

        fit = fit_gcn_td(
            episodes, feature_names, schema, epochs=args.epochs, alpha=args.alpha,
            lam=args.lam, td_error_clip=args.td_error_clip, on_progress=on_progress,
        )
        print()  # newline after the final \r-updated progress line
    else:
        fit = fit_gcn(episodes, feature_names, schema, epochs=args.epochs)
    print(f"Fitted GCN ({args.objective}) after {args.epochs} epochs")

    output_path = (
        Path(args.output)
        if args.output
        else _GCN_REPORT_DIR / f"{datetime.now(UTC).strftime('%Y%m%d-%H%M%S')}.json"
    )
    export_gcn(fit, output_path)
    print(f"GCN weights written → {output_path}")
