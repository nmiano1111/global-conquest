"""Configuration helpers — load .env and expose typed accessors."""

import os

from dotenv import load_dotenv

# Load .env from the analytics project root (two levels up from this file).
# This is a no-op if the file does not exist; the caller must set the var another way.
_ENV_FILE = os.path.join(os.path.dirname(__file__), "..", "..", ".env")
load_dotenv(_ENV_FILE, override=False)


def get_database_url() -> str:
    """Return the DATABASE_URL from the environment.

    Raises:
        RuntimeError: If DATABASE_URL is not set.
    """
    url = os.environ.get("DATABASE_URL")
    if not url:
        raise RuntimeError(
            "DATABASE_URL is not set. "
            "Copy .env.example to .env and fill in your Postgres credentials."
        )
    return url
