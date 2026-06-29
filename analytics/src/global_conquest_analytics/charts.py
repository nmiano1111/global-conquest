"""Matplotlib charts for combat roll analysis.

Each function saves one PNG to reports/generated/combat/ and closes the figure.
No seaborn. No custom colour palette or style.
"""

from __future__ import annotations

from pathlib import Path

import matplotlib.pyplot as plt
import pandas as pd

_CHART_DIR = Path(__file__).parents[2] / "reports" / "generated" / "combat"


def _ensure_chart_dir(chart_dir: Path = _CHART_DIR) -> Path:
    chart_dir.mkdir(parents=True, exist_ok=True)
    return chart_dir


def _explode_dice(series: pd.Series) -> pd.Series:  # type: ignore[type-arg]
    """Explode a Series of dice lists into a flat Series of individual face values."""
    return series.explode().dropna().astype(int)


def plot_attacker_die_distribution(
    df: pd.DataFrame, chart_dir: Path = _CHART_DIR
) -> Path | None:
    """Bar chart: attacker die-face distribution (faces 1–6).

    Args:
        df: Normalised combat DataFrame.
        chart_dir: Directory to write the PNG.

    Returns:
        Path to the saved PNG, or None if df is empty.
    """
    if df.empty or "attacker_dice" not in df.columns:
        print("No data for attacker die distribution chart.")
        return None

    faces = _explode_dice(df["attacker_dice"])
    if faces.empty:
        print("No attacker dice values found.")
        return None

    counts = faces.value_counts().reindex(range(1, 7), fill_value=0).sort_index()
    total = int(faces.shape[0])

    fig, ax = plt.subplots(figsize=(7, 4))
    ax.bar(counts.index.to_numpy(), counts.to_numpy())
    ax.set_title(f"Attacker Die-Face Distribution (n={total:,} dice)")
    ax.set_xlabel("Die Face")
    ax.set_ylabel("Count")
    ax.set_xticks(range(1, 7))
    fig.tight_layout()

    out = _ensure_chart_dir(chart_dir) / "attacker_die_distribution.png"
    fig.savefig(out, dpi=120)
    plt.close(fig)
    print(f"Saved: {out}")
    return out


def plot_defender_die_distribution(
    df: pd.DataFrame, chart_dir: Path = _CHART_DIR
) -> Path | None:
    """Bar chart: defender die-face distribution (faces 1–6).

    Args:
        df: Normalised combat DataFrame.
        chart_dir: Directory to write the PNG.

    Returns:
        Path to the saved PNG, or None if df is empty.
    """
    if df.empty or "defender_dice" not in df.columns:
        print("No data for defender die distribution chart.")
        return None

    faces = _explode_dice(df["defender_dice"])
    if faces.empty:
        print("No defender dice values found.")
        return None

    counts = faces.value_counts().reindex(range(1, 7), fill_value=0).sort_index()
    total = int(faces.shape[0])

    fig, ax = plt.subplots(figsize=(7, 4))
    ax.bar(counts.index.to_numpy(), counts.to_numpy())
    ax.set_title(f"Defender Die-Face Distribution (n={total:,} dice)")
    ax.set_xlabel("Die Face")
    ax.set_ylabel("Count")
    ax.set_xticks(range(1, 7))
    fig.tight_layout()

    out = _ensure_chart_dir(chart_dir) / "defender_die_distribution.png"
    fig.savefig(out, dpi=120)
    plt.close(fig)
    print(f"Saved: {out}")
    return out


def plot_combat_result_by_dice_counts(
    df: pd.DataFrame, chart_dir: Path = _CHART_DIR
) -> Path | None:
    """Grouped bar chart: combat result (attacker losses vs defender losses)
    broken down by (attacker dice count, defender dice count) combinations.

    Args:
        df: Normalised combat DataFrame.
        chart_dir: Directory to write the PNG.

    Returns:
        Path to the saved PNG, or None if df is empty.
    """
    if df.empty:
        print("No data for combat result by dice counts chart.")
        return None

    tmp = df.copy()
    tmp["att_count"] = tmp["attacker_dice"].apply(
        lambda d: len(d) if d is not None and hasattr(d, "__len__") else 0
    )
    tmp["def_count"] = tmp["defender_dice"].apply(
        lambda d: len(d) if d is not None and hasattr(d, "__len__") else 0
    )
    tmp["combo"] = tmp["att_count"].astype(str) + "v" + tmp["def_count"].astype(str)

    grouped = (
        tmp.groupby("combo")[["attacker_losses", "defender_losses"]]
        .mean()
        .sort_index()
    )

    if grouped.empty:
        print("No aggregated data for combat result by dice counts chart.")
        return None

    combos = grouped.index.tolist()
    x = range(len(combos))
    width = 0.35

    fig, ax = plt.subplots(figsize=(max(6, len(combos) * 1.5), 5))
    ax.bar(
        [i - width / 2 for i in x], grouped["attacker_losses"], width, label="Avg Attacker Losses"
    )
    ax.bar(
        [i + width / 2 for i in x], grouped["defender_losses"], width, label="Avg Defender Losses"
    )
    ax.set_title("Average Combat Losses by Dice Count Combination")
    ax.set_xlabel("Dice Combination (AttackerDice v DefenderDice)")
    ax.set_ylabel("Average Losses per Roll")
    ax.set_xticks(list(x))
    ax.set_xticklabels(combos)
    ax.legend()
    fig.tight_layout()

    out = _ensure_chart_dir(chart_dir) / "combat_result_by_dice_counts.png"
    fig.savefig(out, dpi=120)
    plt.close(fig)
    print(f"Saved: {out}")
    return out


def plot_loss_distribution(
    df: pd.DataFrame, chart_dir: Path = _CHART_DIR
) -> Path | None:
    """Grouped bar chart: distribution of (attacker_losses, defender_losses) pairs.

    Args:
        df: Normalised combat DataFrame.
        chart_dir: Directory to write the PNG.

    Returns:
        Path to the saved PNG, or None if df is empty.
    """
    if df.empty:
        print("No data for loss distribution chart.")
        return None

    counts = (
        df.groupby(["attacker_losses", "defender_losses"])
        .size()
        .reset_index(name="count")
    )

    if counts.empty:
        print("No loss distribution data available.")
        return None

    labels = [
        f"A:{int(r['attacker_losses'])} D:{int(r['defender_losses'])}"
        for _, r in counts.iterrows()
    ]
    values = counts["count"].tolist()
    total = sum(values)

    fig, ax = plt.subplots(figsize=(max(6, len(labels) * 1.4), 5))
    ax.bar(range(len(labels)), values)
    ax.set_title(f"Loss Distribution per Combat Roll (n={total:,} rolls)")
    ax.set_xlabel("(Attacker Losses, Defender Losses)")
    ax.set_ylabel("Number of Rolls")
    ax.set_xticks(range(len(labels)))
    ax.set_xticklabels(labels, rotation=30, ha="right")
    fig.tight_layout()

    out = _ensure_chart_dir(chart_dir) / "loss_distribution.png"
    fig.savefig(out, dpi=120)
    plt.close(fig)
    print(f"Saved: {out}")
    return out


def generate_all_charts(
    df: pd.DataFrame, chart_dir: Path = _CHART_DIR
) -> list[Path]:
    """Generate all four combat charts.

    Args:
        df: Normalised combat DataFrame.
        chart_dir: Directory to write PNG files.

    Returns:
        List of paths for successfully written PNGs.
    """
    results: list[Path] = []
    for fn in (
        plot_attacker_die_distribution,
        plot_defender_die_distribution,
        plot_combat_result_by_dice_counts,
        plot_loss_distribution,
    ):
        path = fn(df, chart_dir)
        if path is not None:
            results.append(path)
    return results
