"""Durable run state (AB-07): persist a run so it is resumable and its
artifacts survive across steps, and so metrics can be compared later (AB-08).

Layout under the runs directory (default ``.build-pipeline/runs``):

    <runs_dir>/<run_id>/run.json         # RunRecord (metrics + status)
    <runs_dir>/<run_id>/artifacts/<name> # typed artifacts produced by steps
"""

from __future__ import annotations

import json
import os
from dataclasses import asdict
from datetime import datetime, timezone
from pathlib import Path

from .types import GateResult, RunRecord, StepResult


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _record_to_dict(rec: RunRecord) -> dict:
    return asdict(rec)


def _record_from_dict(d: dict) -> RunRecord:
    steps = []
    for s in d.get("steps", []):
        gates = [GateResult(**g) for g in s.get("gates", [])]
        s = {**s, "gates": gates}
        steps.append(StepResult(**s))
    return RunRecord(
        run_id=d["run_id"],
        profile=d["profile"],
        mode=d.get("mode", ""),
        git_base=d.get("git_base", ""),
        started_at=d.get("started_at", ""),
        finished_at=d.get("finished_at", ""),
        status=d.get("status", "running"),
        steps=steps,
    )


class RunStore:
    def __init__(self, runs_dir: Path):
        self.runs_dir = Path(runs_dir)

    def _dir(self, run_id: str) -> Path:
        return self.runs_dir / run_id

    def new_run(self, profile: str, mode: str, git_base: str = "") -> RunRecord:
        run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S") + "-" + profile
        rec = RunRecord(
            run_id=run_id,
            profile=profile,
            mode=mode,
            git_base=git_base,
            started_at=_now(),
        )
        (self._dir(run_id) / "artifacts").mkdir(parents=True, exist_ok=True)
        self.save(rec)
        return rec

    def save(self, rec: RunRecord) -> None:
        d = self._dir(rec.run_id)
        d.mkdir(parents=True, exist_ok=True)
        tmp = d / "run.json.tmp"
        tmp.write_text(json.dumps(_record_to_dict(rec), indent=2))
        os.replace(tmp, d / "run.json")  # atomic replace so readers never see a partial file

    def load(self, run_id: str) -> RunRecord:
        path = self._dir(run_id) / "run.json"
        return _record_from_dict(json.loads(path.read_text()))

    def list_runs(self) -> list[str]:
        if not self.runs_dir.exists():
            return []
        return sorted(p.name for p in self.runs_dir.iterdir() if (p / "run.json").exists())

    def artifact_path(self, run_id: str, name: str) -> Path:
        return self._dir(run_id) / "artifacts" / name

    def write_artifact(self, run_id: str, name: str, content: str) -> Path:
        p = self.artifact_path(run_id, name)
        p.parent.mkdir(parents=True, exist_ok=True)
        tmp = p.with_suffix(p.suffix + ".tmp")
        tmp.write_text(content)
        os.replace(tmp, p)  # atomic: downstream steps never observe a partial artifact (AB-07)
        return p

    def has_artifact(self, run_id: str, name: str) -> bool:
        return self.artifact_path(run_id, name).exists()

    def finish(self, rec: RunRecord, status: str) -> None:
        rec.status = status
        rec.finished_at = _now()
        self.save(rec)
