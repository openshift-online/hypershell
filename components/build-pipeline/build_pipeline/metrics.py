"""Run metric summaries and cross-configuration comparison (AB-08)."""

from __future__ import annotations

from .types import RunRecord


def summarize(rec: RunRecord) -> dict:
    steps = rec.steps
    return {
        "run_id": rec.run_id,
        "profile": rec.profile,
        "mode": rec.mode,
        "git_base": rec.git_base,
        "status": rec.status,
        "steps_total": len(steps),
        "steps_completed": sum(1 for s in steps if s.status == "completed"),
        "steps_failed": sum(1 for s in steps if s.status == "failed"),
        "prompt_tokens": sum(s.prompt_tokens for s in steps),
        "completion_tokens": sum(s.completion_tokens for s in steps),
        "cost_usd": round(sum(s.cost_usd for s in steps), 4),
        "duration_s": round(sum(s.duration_s for s in steps), 2),
        "tool_calls": sum(s.tool_calls for s in steps),
        "retries": sum(s.retries for s in steps),
        "escalations": sum(1 for s in steps if s.escalated),
    }


_COLS = [
    ("profile", "profile"),
    ("status", "status"),
    ("steps_completed", "done"),
    ("cost_usd", "cost($)"),
    ("prompt_tokens", "in-tok"),
    ("completion_tokens", "out-tok"),
    ("duration_s", "secs"),
    ("escalations", "esc"),
]


def compare(records: list[RunRecord]) -> str:
    """Render a fixed-width comparison table across runs of the same graph.

    Correctness is held constant by gates (AB-04), so this table is read as a
    cost/latency comparison at equal quality when every run's status is
    ``completed``.
    """
    rows = [summarize(r) for r in records]
    headers = [label for _, label in _COLS]
    table = [headers] + [[str(row[key]) for key, _ in _COLS] for row in rows]
    widths = [max(len(r[i]) for r in table) for i in range(len(headers))]
    lines = []
    for ri, row in enumerate(table):
        lines.append("  ".join(cell.ljust(widths[i]) for i, cell in enumerate(row)))
        if ri == 0:
            lines.append("  ".join("-" * widths[i] for i in range(len(headers))))
    return "\n".join(lines)
