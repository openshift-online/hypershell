"""Deterministic, model-independent acceptance gates (AB-04).

A gate is a subprocess whose exit status decides pass/fail. The same gate is
applied regardless of which model produced a step's output, so cross-profile
comparisons (AB-08) hold correctness constant.
"""

from __future__ import annotations

import subprocess
import time
from pathlib import Path

from .types import Command, GateResult

_TAIL = 2000


def _tail(s: str) -> str:
    s = s or ""
    return s[-_TAIL:]


def run_command(cmd: Command, repo_root: Path, timeout: float | None = None) -> GateResult:
    cwd = (repo_root / cmd.cwd).resolve()
    start = time.monotonic()
    try:
        proc = subprocess.run(
            list(cmd.argv), cwd=str(cwd), capture_output=True, text=True, timeout=timeout
        )
        rc, out, err = proc.returncode, proc.stdout, proc.stderr
    except subprocess.TimeoutExpired as exc:
        rc, out, err = 124, exc.stdout or "", f"timed out after {timeout}s"
    except (FileNotFoundError, NotADirectoryError) as exc:
        rc, out, err = 127, "", str(exc)
    dur = round(time.monotonic() - start, 3)
    return GateResult(
        name=cmd.name or (cmd.argv[0] if cmd.argv else "command"),
        command=cmd.display(),
        passed=(rc == 0),
        exit_code=rc,
        duration_s=dur,
        stdout_tail=_tail(out),
        stderr_tail=_tail(err),
    )


def run_gates(
    gates: tuple[Command, ...], repo_root: Path, timeout: float | None = None
) -> tuple[bool, list[GateResult]]:
    """Run gates in order, stopping at the first failure. Empty => pass."""
    results: list[GateResult] = []
    for g in gates:
        r = run_command(g, repo_root, timeout=timeout)
        results.append(r)
        if not r.passed:
            return False, results
    return True, results
