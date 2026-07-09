# Global Conquest Analytics

Local extraction, analysis, visualisation, and report generation for Global Conquest game events stored in Postgres. No hosted services or background jobs required.

## Purpose

- Pull raw `game_events` rows from Postgres into local Parquet files.
- Normalise and validate specific event types (currently: `combat_roll_resolved`).
- Generate Matplotlib charts and a Markdown report.

## Requirements

- Python 3.12+
- [Poetry](https://python-poetry.org/) 1.7+
- A running Postgres instance with the Global Conquest schema

## Setup

```bash
# From the analytics/ directory
poetry install

# Copy the env template and fill in your credentials
cp .env.example .env
# Edit .env — set DATABASE_URL
```

## Environment Configuration

```
DATABASE_URL=postgresql://user:password@localhost:5432/global_conquest
```

The default local dev credentials (from the backend CLAUDE.md) are:
```
DATABASE_URL=postgresql://globalconq:globalconq@localhost:5432/globalconq
```

## CLI Commands

### 1. Export events from Postgres

Requires `DATABASE_URL` to be set.

```bash
poetry run export-events
```

Writes: `data/raw/game_events.parquet`

### 2. Generate combat report

Does **not** query Postgres. Run `export-events` first.

```bash
poetry run combat-report
```

Writes:
- `data/processed/combat_rolls.parquet` — normalised combat events
- `reports/generated/combat/report.md` — Markdown report
- `reports/generated/combat/validation_failures.csv`
- `reports/generated/combat/player_combat_summary.csv`
- `reports/generated/combat/*.png` — four Matplotlib charts

### 3. Export games from Postgres

Requires `DATABASE_URL` to be set. Needed by `roll-streak-report` for game
names and partial-event-history detection.

```bash
poetry run export-games
```

Writes: `data/raw/games.parquet`

### 4. Generate roll streak report

Reports consecutive attacking loss streaks, win streaks, and "attack
droughts" (non-win runs) per player. Does **not** query Postgres — run
`export-events` and `export-games` first.

```bash
poetry run roll-streak-report --format markdown
poetry run roll-streak-report --format json --game-id <game-id>
```

Options: `--game-id`, `--player-id`, `--min-loss-streak-length` (default 2),
`--min-win-streak-length` (default 2), `--min-drought-length` (default 3),
`--top` (default 5, per-section streak count in Markdown; 0 = show all),
`--format markdown|json`, `--include-partial-games` (required to proceed
when the target game's `event_history_complete` flag is false).

## Output Locations

| Path | Description |
|------|-------------|
| `data/raw/game_events.parquet` | Raw extract — all event types, payload as JSON string |
| `data/processed/combat_rolls.parquet` | Flattened combat events only |
| `reports/generated/combat/report.md` | Human-readable summary |
| `reports/generated/combat/validation_failures.csv` | Row-level validation errors |
| `reports/generated/combat/player_combat_summary.csv` | Per-attacker statistics |

## Raw vs Processed

- **Raw** (`data/raw/`): a direct dump of the `game_events` table. The `payload` column is stored as a JSON string. Nothing is transformed.
- **Processed** (`data/processed/`): event-type-specific transforms. For combat events, the JSON payload is flattened into columns and list fields (dice arrays, comparisons) remain as Python lists.

## Running Tests

```bash
poetry run pytest
```

## Linting and Type Checking

```bash
poetry run ruff check .
poetry run mypy src
```

## Notebooks

Place Jupyter notebooks in `notebooks/`. Start JupyterLab with:

```bash
poetry run jupyter lab
```

## Scope and Limitations

- Only `combat_roll_resolved` events are analysed. Other event types remain in the raw Parquet for future use.
- All processing is local and single-machine. No distributed compute, no streaming pipeline.
- The `export-events` command takes a full snapshot; it does not perform incremental updates.

## Sample Size Warning

Die-face distribution statistics should not be interpreted as evidence of RNG quality or bias without a sufficient sample size. A few hundred or even a few thousand rolls are not statistically conclusive. The report notes the sample count alongside all percentages.

## Gitignored Data

`data/raw/`, `data/processed/`, and `reports/generated/` are gitignored. Re-generate them locally by running the CLI commands above. The `.gitkeep` files that mark these directories are tracked.
