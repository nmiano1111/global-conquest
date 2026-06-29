"""Thin Postgres connection helper using psycopg 3."""

from collections.abc import Generator
from contextlib import contextmanager

import psycopg

from global_conquest_analytics.config import get_database_url


@contextmanager
def get_connection() -> Generator[psycopg.Connection, None, None]:
    """Open a psycopg 3 connection and close it when the block exits.

    Usage::

        with get_connection() as conn:
            df = pd.read_sql("SELECT ...", conn)
    """
    url = get_database_url()
    conn = psycopg.connect(url)
    try:
        yield conn
    finally:
        conn.close()
