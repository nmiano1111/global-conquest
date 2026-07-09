"""Extract raw game_events and games from Postgres and write to Parquet."""

from pathlib import Path

import pandas as pd

from global_conquest_analytics.db import get_connection

_RAW_OUTPUT = Path(__file__).parents[2] / "data" / "raw" / "game_events.parquet"
_GAMES_OUTPUT = Path(__file__).parents[2] / "data" / "raw" / "games.parquet"

# Cast UUIDs to text so psycopg3 returns plain strings; PyArrow cannot
# serialise uuid.UUID objects directly.
_QUERY = """
SELECT
    id::text,
    game_id::text,
    game_sequence,
    event_type,
    event_version,
    actor_player_id::text,
    occurred_at,
    payload
FROM game_domain_events
ORDER BY game_id, game_sequence
"""

_COLUMNS = [
    "id",
    "game_id",
    "game_sequence",
    "event_type",
    "event_version",
    "actor_player_id",
    "occurred_at",
    "payload",
]

# A direct dump of games, kept separate from game_domain_events (own table,
# own shape). event_history_complete backs the partial-event-history warning
# in the roll streak report — see backend migration V13 for its semantics.
_GAMES_QUERY = """
SELECT
    id::text,
    name,
    event_history_complete
FROM games
ORDER BY id
"""

_GAMES_COLUMNS = ["id", "name", "event_history_complete"]


def extract_game_events(output_path: Path = _RAW_OUTPUT) -> Path:
    """Query game_events and write the result to a Parquet file.

    Uses a psycopg3 cursor directly (pd.read_sql does not support psycopg3).

    Args:
        output_path: Destination path for the Parquet file.

    Returns:
        The resolved output path.
    """
    output_path.parent.mkdir(parents=True, exist_ok=True)

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(_QUERY)
            rows = cur.fetchall()

    df = pd.DataFrame(rows, columns=_COLUMNS)
    df.to_parquet(output_path, index=False, engine="pyarrow")
    print(f"Extracted {len(df):,} rows → {output_path}")
    return output_path


def extract_games(output_path: Path = _GAMES_OUTPUT) -> Path:
    """Query games (id, name, event_history_complete) and write to Parquet.

    Args:
        output_path: Destination path for the Parquet file.

    Returns:
        The resolved output path.
    """
    output_path.parent.mkdir(parents=True, exist_ok=True)

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(_GAMES_QUERY)
            rows = cur.fetchall()

    df = pd.DataFrame(rows, columns=_GAMES_COLUMNS)
    df.to_parquet(output_path, index=False, engine="pyarrow")
    print(f"Extracted {len(df):,} games → {output_path}")
    return output_path
