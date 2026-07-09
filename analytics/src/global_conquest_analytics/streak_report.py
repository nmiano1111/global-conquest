"""Markdown and JSON rendering for RollStreakReport.

Kept separate from streaks.py so the detection core stays pure and
format-agnostic (streaks.py has no knowledge of Markdown, JSON, or Discord).
"""

from __future__ import annotations

import dataclasses
import json
from typing import Any

from global_conquest_analytics.streaks import PlayerStreakSummary, RollStreakReport, Streak


def render_markdown(report: RollStreakReport, top: int = 0) -> str:
    """Render the full Markdown report.

    Args:
        report: The report to render.
        top: Limit each individual-streak section to the top N entries
            (already sorted length DESC, start_time ASC). 0 = show all.
    """
    lines: list[str] = [f"# Roll Streak Report — {report.game_name}", ""]
    lines.append(_overview_line(report))
    lines.append("")

    if report.partial_history:
        lines.append(
            "Note: this game has partial event history. Streaks only reflect "
            "captured rolls after event logging began."
        )
        lines.append("")

    lines += [
        "## Definitions",
        "",
        "- Attacking loss streak: 2+ consecutive rolls where the attacker loses more "
        "armies than the defender.",
        "- Attacking win streak: 2+ consecutive rolls where the defender loses more "
        "armies than the attacker.",
        "- Attack drought: 3+ consecutive rolls where the attacker does not win the roll.",
        "- Roll trace format: attacker losses - defender losses.",
        "",
        "## Summary by Attacker",
        "",
    ]
    lines += _summary_table(report.summary_by_attacker)

    lines += ["", "## Individual Attacking Loss Streaks", ""]
    lines += _streak_list("L", report.attacking_loss_streaks, top, "attacking losses")

    lines += ["", "## Individual Attacking Win Streaks", ""]
    lines += _streak_list("W", report.attacking_win_streaks, top, "attacking wins")

    lines += ["", "## Individual Attack Droughts", ""]
    lines += _streak_list("D", report.attack_droughts, top, "roll drought")

    if report.warnings:
        lines += ["", "## Diagnostics", ""]
        lines += [f"- {w}" for w in report.warnings]

    return "\n".join(lines) + "\n"


def _overview_line(report: RollStreakReport) -> str:
    total_rolls = sum(p.attack_rolls_captured for p in report.summary_by_attacker)
    total_loss_streaks = len(report.attacking_loss_streaks)
    total_win_streaks = len(report.attacking_win_streaks)
    total_droughts = len(report.attack_droughts)
    return (
        f"**{len(report.summary_by_attacker)} attacker(s)** · **{total_rolls} rolls captured** · "
        f"{total_loss_streaks} loss streak(s) · {total_win_streaks} win streak(s) · "
        f"{total_droughts} drought(s)"
    )


def _summary_table(summaries: list[PlayerStreakSummary]) -> list[str]:
    if not summaries:
        return ["_No attack rolls captured._"]
    lines = [
        "| Player | Rolls | W/L/S | Loss Streaks | Worst Loss | Win Streaks | Best Win | "
        "Droughts | Worst Drought | Loss Streaks / 20 | Win Streaks / 20 | Droughts / 20 |",
        "|---|---|---|---|---|---|---|---|---|---|---|---|",
    ]
    for p in summaries:
        lines.append(
            f"| {p.player_name} | {p.attack_rolls_captured} | "
            f"{p.attacker_win_count}/{p.attacker_loss_count}/{p.split_count} | "
            f"{p.loss_streak_count_2_plus} | {p.longest_loss_streak} | "
            f"{p.win_streak_count_2_plus} | {p.longest_win_streak} | "
            f"{p.attack_drought_count_3_plus} | {p.longest_attack_drought} | "
            f"{p.loss_streaks_per_20_attacks:.2f} | {p.win_streaks_per_20_attacks:.2f} | "
            f"{p.droughts_per_20_attacks:.2f} |"
        )
    return lines


def _streak_list(prefix: str, streaks: list[Streak], top: int, unit_label: str) -> list[str]:
    if not streaks:
        return ["_None._"]
    n = len(streaks) if top <= 0 else min(top, len(streaks))
    lines: list[str] = []
    for i, s in enumerate(streaks[:n], start=1):
        if i > 1:
            lines.append("")
        lines.append(f"{prefix}{i}. {s.attacker_name} — {s.streak_length} {unit_label}")
        lines.append(
            f"    Events {s.start_event_seq}–{s.end_event_seq} · lost {s.attacker_armies_lost} · "
            f"killed {s.defender_armies_lost} · net {s.net_army_delta_for_attacker}"
        )
        lines.append(f"    Against: {', '.join(s.defenders_involved)}")
        lines.append(f"    Territories: {_territory_pairs(s)}")
        lines.append(f"    Captures: {s.captures_during_streak}")
        lines.append(f"    Rolls: {s.roll_trace}")
    if top > 0 and len(streaks) > n:
        remaining = len(streaks) - n
        lines.append("")
        lines.append(
            f"_({remaining} more not shown — use --top 0 or JSON output for the full list)_"
        )
    return lines


def _territory_pairs(s: Streak) -> str:
    seen: set[str] = set()
    pairs: list[str] = []
    for roll in s.rolls:
        pair = f"{roll.attacker_territory} → {roll.defender_territory}"
        if pair in seen:
            continue
        seen.add(pair)
        pairs.append(pair)
    return ", ".join(pairs)


def report_to_dict(report: RollStreakReport) -> dict[str, Any]:
    """Convert a RollStreakReport into the documented JSON shape."""

    def streak_dict(s: Streak) -> dict[str, Any]:
        d = dataclasses.asdict(s)
        d["streak_type"] = s.streak_type.value
        return d

    return {
        "game_id": report.game_id,
        "game_name": report.game_name,
        "partial_history": report.partial_history,
        "warnings": report.warnings,
        "summary_by_attacker": [dataclasses.asdict(p) for p in report.summary_by_attacker],
        "streaks": {
            "attacking_loss": [streak_dict(s) for s in report.attacking_loss_streaks],
            "attacking_win": [streak_dict(s) for s in report.attacking_win_streaks],
            "attack_drought": [streak_dict(s) for s in report.attack_droughts],
        },
    }


def render_json(report: RollStreakReport, indent: int = 2) -> str:
    return json.dumps(report_to_dict(report), indent=indent, default=str)
