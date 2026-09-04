"""Command-line entry point (AB-06, AB-11).

Subcommands:
  list        Show the build chain graph.
  profiles    List profiles, or show one profile's per-step model resolution.
  run         Run the chain (whole or a subgraph) under a profile.
  resume      Continue a previous run, skipping completed steps.
  runs        List recorded runs.
  compare     Compare recorded runs on cost/latency (AB-08).
  check-drift Flag divergence between the skill and the graph (AB-05, heuristic).

The list/profiles/dry-run paths use only the standard library; a real run of an
LLM-backed step lazily requires LangChain and provider credentials.
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

from . import graph, metrics
from .engine import Engine, EngineConfig
from .profiles import list_profiles, load_profile
from .state import RunStore

SKILL_PATH = "skills/build/full-stack-pipeline/SKILL.md"


def _repo_root(explicit: str | None) -> Path:
    if explicit:
        return Path(explicit).resolve()
    try:
        top = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True
        ).stdout.strip()
        if top:
            return Path(top).resolve()
    except Exception:
        pass
    return Path.cwd()


def _git_head(repo_root: Path) -> str:
    try:
        return subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=str(repo_root), capture_output=True, text=True, check=True
        ).stdout.strip()
    except Exception:
        return ""


def _runs_dir(repo_root: Path, explicit: str | None) -> Path:
    return Path(explicit).resolve() if explicit else repo_root / ".build-pipeline" / "runs"


# -- subcommands --------------------------------------------------------------

def cmd_list(args) -> int:
    for s in graph.topo_order():
        deps = ",".join(s.deps) or "-"
        gates = " | ".join(g.display() for g in s.gates) or ("action: " + s.action.display() if s.action else "-")
        print(f"[w{s.wave}] {s.id:<14} {s.step_class.value:<10} tier={s.default_tier().value:<8} deps={deps}")
        print(f"        gates/action: {gates}")
    return 0


def cmd_profiles(args) -> int:
    if not args.profile:
        for name in list_profiles():
            print(name)
        return 0
    profile = load_profile(args.profile)
    print(f"profile: {profile.name}")
    for tier, cfg in sorted(profile.tiers.items()):
        print(f"  tier {tier:<9} -> {cfg.get('provider')}:{cfg.get('model')}")
    print("resolved model per step:")
    for s in graph.topo_order():
        b = profile.resolve(s)
        print(f"  {s.id:<14} {s.step_class.value:<10} -> {b.label() if b else '(no model / tool-only)'}")
    return 0


def _build_engine(args, repo_root: Path):
    profile = load_profile(args.profile)
    store = RunStore(_runs_dir(repo_root, args.runs_dir))
    cfg = EngineConfig(
        repo_root=repo_root,
        profile=profile,
        store=store,
        mode=args.mode,
        dry_run=args.dry_run,
        approve_human=args.approve_human,
        max_attempts=args.max_attempts,
        gate_timeout=args.gate_timeout,
    )
    return Engine(cfg), store, profile


def _report(rec) -> None:
    s = metrics.summarize(rec)
    print(f"\nrun {rec.run_id} [{rec.status}]")
    for step in rec.steps:
        mark = {"completed": "ok", "failed": "FAIL", "planned": "plan",
                "blocked": "blocked", "awaiting-human": "human", "skipped": "skip"}.get(step.status, step.status)
        extra = f" model={step.model}" if step.model else ""
        gate = "" if step.gate_passed is None else f" gate={'pass' if step.gate_passed else 'fail'}"
        esc = " escalated" if step.escalated else ""
        print(f"  {mark:<8} {step.step_id:<14} {step.step_class:<10}{extra}{gate}{esc}")
        if step.error and step.status != "completed":
            print(f"           {step.error}")
    print(f"tokens in/out={s['prompt_tokens']}/{s['completion_tokens']} cost=${s['cost_usd']} "
          f"time={s['duration_s']}s escalations={s['escalations']}")


def cmd_run(args) -> int:
    repo_root = _repo_root(args.repo_root)
    engine, store, _ = _build_engine(args, repo_root)
    steps = graph.select(from_=getattr(args, "from"), to=args.to, only=args.only)
    rec = store.new_run(load_profile(args.profile).name, args.mode, _git_head(repo_root))
    rec = engine.run(steps, rec, supplied=set(args.supply or []))
    _report(rec)
    return 0 if rec.status in ("completed", "running") else 1


def cmd_resume(args) -> int:
    repo_root = _repo_root(args.repo_root)
    engine, store, _ = _build_engine(args, repo_root)
    rec = store.load(args.run_id)
    rec = engine.run(graph.topo_order(), rec, supplied=set(args.supply or []))
    _report(rec)
    return 0 if rec.status in ("completed", "running") else 1


def cmd_runs(args) -> int:
    store = RunStore(_runs_dir(_repo_root(args.repo_root), args.runs_dir))
    for rid in store.list_runs():
        print(rid)
    return 0


def cmd_compare(args) -> int:
    store = RunStore(_runs_dir(_repo_root(args.repo_root), args.runs_dir))
    recs = [store.load(rid) for rid in args.run_ids]
    print(metrics.compare(recs))
    return 0


def cmd_check_drift(args) -> int:
    """Heuristic drift check (AB-05): every graph gate/action command should
    appear in the skill. Reports commands present in the graph but not the skill.
    """
    repo_root = _repo_root(args.repo_root)
    skill = (repo_root / SKILL_PATH).read_text()
    missing: list[str] = []
    for s in graph.PIPELINE:
        cmds = list(s.gates) + ([s.action] if s.action else [])
        for c in cmds:
            token = " ".join(c.argv[:2])  # e.g. "make test", "go build", "bash components/pr-test/..."
            probe = c.argv[-1] if c.argv[-1] not in ("./...",) else token
            if token not in skill and probe not in skill:
                missing.append(f"{s.id}: {c.display()}")
    if missing:
        print("DRIFT: graph commands not found in the skill (heuristic):")
        for m in missing:
            print(f"  - {m}")
        return 1
    print("no drift detected (heuristic: all graph commands appear in the skill)")
    return 0


# -- parser -------------------------------------------------------------------

def _add_run_opts(p) -> None:
    p.add_argument("--profile", default="tiered", help="profile name or path to a .toml (default: tiered)")
    p.add_argument("--mode", choices=["supervised", "autonomous"], default="supervised")
    p.add_argument("--dry-run", action="store_true", help="resolve models and plan without calling models or running gates")
    p.add_argument("--approve-human", action="store_true", help="pre-approve human checkpoints")
    p.add_argument("--max-attempts", type=int, default=3)
    p.add_argument("--gate-timeout", type=float, default=None)
    p.add_argument("--supply", nargs="*", default=[], metavar="ARTIFACT", help="artifacts already available (for subgraph runs)")
    p.add_argument("--repo-root", default=None)
    p.add_argument("--runs-dir", default=None)


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(prog="build-pipeline", description="Agentic build chain (HYPERSHELL-301)")
    sub = ap.add_subparsers(dest="cmd", required=True)

    sub.add_parser("list", help="show the build chain graph").set_defaults(func=cmd_list)

    pp = sub.add_parser("profiles", help="list profiles or show one profile's resolution")
    pp.add_argument("profile", nargs="?", default=None)
    pp.set_defaults(func=cmd_profiles)

    pr = sub.add_parser("run", help="run the chain or a subgraph")
    pr.add_argument("--from", dest="from", default=None, help="start stage (by step id)")
    pr.add_argument("--to", default=None, help="end stage (by step id)")
    pr.add_argument("--only", default=None, help="run a single step")
    _add_run_opts(pr)
    pr.set_defaults(func=cmd_run)

    rs = sub.add_parser("resume", help="continue a previous run")
    rs.add_argument("run_id")
    _add_run_opts(rs)
    rs.set_defaults(func=cmd_resume)

    ru = sub.add_parser("runs", help="list recorded runs")
    ru.add_argument("--repo-root", default=None)
    ru.add_argument("--runs-dir", default=None)
    ru.set_defaults(func=cmd_runs)

    cp = sub.add_parser("compare", help="compare recorded runs")
    cp.add_argument("run_ids", nargs="+")
    cp.add_argument("--repo-root", default=None)
    cp.add_argument("--runs-dir", default=None)
    cp.set_defaults(func=cmd_compare)

    cd = sub.add_parser("check-drift", help="flag skill/graph divergence (heuristic)")
    cd.add_argument("--repo-root", default=None)
    cd.set_defaults(func=cmd_check_drift)

    return ap


def main(argv: list[str] | None = None) -> int:
    graph.validate()
    args = build_parser().parse_args(argv)
    return args.func(args)


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
