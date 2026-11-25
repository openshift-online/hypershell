#!/usr/bin/env python3
"""Replay past delivery runs to check the Delivery tab rules (FLT-DLV-21).

The replay rebuilds each past change's phases from the GitHub API and the
public bundle registry, then checks the estimate, arrival, and Overdue rules of
specs/platform/fleet-dashboard-delivery-pipeline.spec.md against what really
happened. Each run is estimated only from runs that ended before it.

Sources:

1. Application repository: commits on the default branch and their Konflux
   check runs (component builds and policy checks).
2. Bundle registry: `release-bundle-*` tags (publication time) and each
   bundle's `bundle.json` (component revisions, manifests revision).
3. GitOps repository: release-lock history on the default branch (GitOps
   update), the promotion strategy (environments, dependencies, gates), and the
   merge commits on each environment branch (promotions), with their hydration
   metadata and gate check runs.

Every repository, path, and branch prefix is an argument. Nothing about a
particular fleet is written here (FLT-DLV-15).

Example:

    python3 scripts/delivery_replay.py \\
        --app-repo <owner>/<app> --gitops-repo <owner>/<gitops> \\
        --strategy-path <path/to/promotion.yaml> \\
        --since 2026-11-03 --until 2026-11-13
"""

from __future__ import annotations

import argparse
import base64
import dataclasses
import datetime as dt
import hashlib
import json
import os
import re
import statistics
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Iterable

# --------------------------------------------------------------------------- #
# Rule constants (FLT-DLV-04, FLT-DLV-09, FLT-DLV-10, FLT-DLV-21)
# --------------------------------------------------------------------------- #

ESTIMATE_RUNS = 5  # last N completed runs
MIN_RUNS = 3  # fewer: no estimate
OVERDUE_MARGIN = 5 * 60  # seconds over the slowest of the last runs
STALL_MARGIN = 15 * 60  # a real stall took more than slowest + this
EARLY_FACTOR = 0.8  # early end of the arrival range before publication

RULES = ("2x-estimate", "slowest-of-5", "slowest-of-5+5min")

# Ignorable conclusions: a newer run replaced this one (FLT-DLV-05).
IGNORABLE = {"cancelled", "skipped", "stale"}
SUCCEEDED = {"success", "neutral"}


# --------------------------------------------------------------------------- #
# Pure logic (unit tested in scripts/test_delivery_replay.py)
# --------------------------------------------------------------------------- #


@dataclasses.dataclass(frozen=True)
class Run:
    """One completed execution of a phase: when it ended and how long it took."""

    end: float
    duration: float


@dataclasses.dataclass(frozen=True)
class Estimate:
    median: float
    slowest: float


def estimate(runs: Iterable[Run], at: float) -> Estimate | None:
    """Median and slowest of the last 5 runs that ended before `at` (FLT-DLV-09)."""
    past = sorted((r for r in runs if r.end < at), key=lambda r: r.end)[-ESTIMATE_RUNS:]
    if len(past) < MIN_RUNS:
        return None
    durations = [r.duration for r in past]
    return Estimate(median=statistics.median(durations), slowest=max(durations))


def combine(estimates: Iterable[Estimate | None]) -> Estimate | None:
    """The slowest component decides a per-component phase (FLT-DLV-09).

    Any component without an estimate leaves the phase without one.
    """
    out: Estimate | None = None
    any_seen = False
    for e in estimates:
        any_seen = True
        if e is None:
            return None
        out = e if out is None else Estimate(max(out.median, e.median), max(out.slowest, e.slowest))
    return out if any_seen else None


def bundle_contains(
    bundle_revs: dict[str, str],
    rebuilt: Iterable[str],
    pos: dict[str, int],
    change_pos: int,
    manifests_rev: str,
) -> bool:
    """Containment by component revision (FLT-DLV-06).

    A bundle contains the change when every rebuilt component the bundle lists
    has a revision at or after the change on the linear default branch. A
    change that rebuilt nothing the bundle lists falls back to the
    manifest-revision rule. A revision older than the read window is before
    the change.
    """
    listed = [c for c in rebuilt if c in bundle_revs]
    if not listed:
        return manifest_contains(manifests_rev, pos, change_pos)
    return all(pos.get(bundle_revs[c], -1) >= change_pos for c in listed)


def manifest_contains(manifests_rev: str, pos: dict[str, int], change_pos: int) -> bool:
    """The manifest-revision rule: the manifests revision is at or after the change."""
    return pos.get(manifests_rev, -1) >= change_pos


@dataclasses.dataclass(frozen=True)
class PhaseView:
    """One phase of a change at the moment of the prediction."""

    passed: bool
    elapsed: float  # 0 for a phase not started yet
    est: Estimate | None


def arrival(
    now: float, phases: list[PhaseView], published: bool
) -> tuple[float, float] | None:
    """Expected arrival at the end of the last phase given (FLT-DLV-10).

    Returns (early, late). After publication early == late. None when a phase
    still to run has no estimate.
    """
    rem_est = 0.0
    rem_slow = 0.0
    for p in phases:
        if p.passed:
            continue
        if p.est is None:
            return None
        rem_est += max(0.0, p.est.median - p.elapsed)
        rem_slow += max(0.0, p.est.slowest - p.elapsed)
    if published:
        return now + rem_est, now + rem_est
    return now + EARLY_FACTOR * rem_est, now + rem_slow


def overdue_flags(duration: float, est: Estimate | None) -> dict[str, bool]:
    """Which Overdue rules would have flagged a run of this final duration."""
    if est is None:
        return {r: False for r in RULES}
    return {
        "2x-estimate": duration > 2 * est.median,
        "slowest-of-5": duration > est.slowest,
        "slowest-of-5+5min": duration > est.slowest + OVERDUE_MARGIN,
    }


def real_stall(duration: float, est: Estimate | None) -> bool:
    """A run that finally took more than its slowest of 5 plus 15 minutes (FLT-DLV-21)."""
    return est is not None and duration > est.slowest + STALL_MARGIN


def classify_check(name: str) -> tuple[str, str] | None:
    """('build'|'policy', component) for a Konflux check run name (FLT-DLV-05)."""
    parts = [p.strip() for p in name.split("/")]
    last = parts[-1]
    if last.endswith("-on-push"):
        return "build", last[: -len("-on-push")]
    if "-enterprise-contract /" in name:
        return "policy", last
    return None


def gate_runs(runs: list[dict], keys: Iterable[str], branch: str) -> tuple[float | None, bool, bool]:
    """Gate result of one promotion: (gates passed at, all passed, re-run seen).

    A gate's run is named `<key>/<branch>`, or `<key>` when that is absent. The
    gates pass when the last gate first succeeds: a health check posted again
    later on the same commit does not move the time (FLT-DLV-08). A re-run is
    more than one run of a gate started at or before its first pass; checks
    that confirm the pass again later do not count (FLT-DLV-09).
    """
    by_name: dict[str, list[dict]] = {}
    for r in runs:
        by_name.setdefault(r["name"], []).append(r)
    end = None
    rerun = False
    for k in keys:
        chosen = by_name.get(f"{k}/{branch}") or by_name.get(k)
        if not chosen:
            return None, False, rerun
        ok = [
            t
            for r in chosen
            if r.get("status") == "completed" and r.get("conclusion") in SUCCEEDED
            and (t := ts(r.get("completed_at"))) is not None
        ]
        if not ok:
            rerun = rerun or len(chosen) > 1
            return None, False, rerun
        first = min(ok)
        before = sum(1 for r in chosen if (st := ts(r.get("started_at"))) is not None and st <= first)
        rerun = rerun or before > 1
        end = first if end is None else max(end, first)
    return end, end is not None, rerun


def effective_deps(envs: list[str], deps: dict[str, list[str]]) -> dict[str, list[str]]:
    """The promotion graph: declared dependsOn, or a chain in spec order when
    no environment declares any, as the promoter does (FLT-DLV-08)."""
    if any(deps.get(e) for e in envs):
        return {e: list(deps.get(e, [])) for e in envs}
    return {e: ([envs[i - 1]] if i else []) for i, e in enumerate(envs)}


def group_estimates(
    env_est: dict[str, Estimate | None],
    deps: dict[str, list[str]],
    groups: list[tuple[str, list[str]]],
) -> dict[str, Estimate | None]:
    """Each group's estimate from the environment estimates (FLT-DLV-09, FLT-DLV-10).

    An environment ends its own estimate after the latest of its
    dependencies; a group ends when its last environment does. A group's
    estimate is the time it adds after the groups before it. A group with an
    environment without an estimate has none, and neither do later groups.
    """
    def offsets(pick) -> dict[str, float]:
        out: dict[str, float] = {}
        state: dict[str, int] = {}

        def visit(e: str) -> float | None:
            if state.get(e) == 1:
                return None  # a cycle: no estimate
            if state.get(e) == 2:
                return out.get(e)
            state[e] = 1
            try:
                est = env_est.get(e)
                if est is None:
                    return None
                start = 0.0
                for d in deps.get(e, []):
                    v = visit(d)
                    if v is None:
                        return None
                    start = max(start, v)
                out[e] = start + pick(est)
                return out[e]
            finally:
                state[e] = 2

        for e in deps:
            visit(e)
        return out

    med = offsets(lambda e: e.median)
    slow = offsets(lambda e: e.slowest)
    result: dict[str, Estimate | None] = {g: None for g, _ in groups}
    prev_m = prev_s = 0.0
    for g, envs in groups:
        if not envs or any(e not in med or e not in slow for e in envs):
            break
        end_m = max(med[e] for e in envs)
        end_s = max(slow[e] for e in envs)
        result[g] = Estimate(max(0.0, end_m - prev_m), max(0.0, end_s - prev_s))
        prev_m, prev_s = max(prev_m, end_m), max(prev_s, end_s)
    return result


def env_phase(env: str) -> str:
    """Sample key of one environment's promotion runs (FLT-DLV-09)."""
    return "environment:" + env


def ts(iso: str | None) -> float | None:
    if not iso:
        return None
    return dt.datetime.fromisoformat(iso.replace("Z", "+00:00")).timestamp()


def iso(t: float | None) -> str:
    if t is None:
        return "-"
    return dt.datetime.fromtimestamp(t, dt.timezone.utc).strftime("%Y-%m-%d %H:%M")


def minutes(seconds: float | None) -> str:
    return "-" if seconds is None else f"{seconds / 60:.0f}"


# --------------------------------------------------------------------------- #
# API access with an on-disk cache
# --------------------------------------------------------------------------- #


class Api:
    def __init__(self, cache_dir: Path, token: str, refresh_lists: bool):
        self.cache = cache_dir
        self.cache.mkdir(parents=True, exist_ok=True)
        self.token = token
        self.refresh_lists = refresh_lists
        self.calls = 0
        self.registry_tokens: dict[str, str] = {}

    def _get(self, url: str, headers: dict[str, str], immutable: bool) -> Any:
        key = hashlib.sha256((url + json.dumps(headers, sort_keys=True)).encode()).hexdigest()
        path = self.cache / f"{key}.json"
        if path.exists() and (immutable or not self.refresh_lists):
            return json.loads(path.read_text())
        req = urllib.request.Request(url, headers=headers)
        self.calls += 1
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                body = json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            if e.code == 404:
                body = None
            else:
                raise
        path.write_text(json.dumps(body))
        return body

    def gh(self, path: str, immutable: bool = False) -> Any:
        headers = {"Accept": "application/vnd.github+json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return self._get("https://api.github.com" + path, headers, immutable)

    def gh_pages(self, path: str, key: str | None = None, limit: int = 30) -> list:
        out = []
        sep = "&" if "?" in path else "?"
        for page in range(1, limit + 1):
            body = self.gh(f"{path}{sep}per_page=100&page={page}")
            items = body.get(key, []) if key and body else (body or [])
            out.extend(items)
            if len(items) < 100:
                break
        return out

    def registry_json(self, host: str, repo: str, path: str, accept: str = "") -> Any:
        if repo not in self.registry_tokens:
            tok = self._get(
                f"https://{host}/v2/auth?service={host}&scope=repository:{repo}:pull", {}, False
            )
            self.registry_tokens[repo] = (tok or {}).get("token", "")
        headers = {"Authorization": f"Bearer {self.registry_tokens[repo]}"}
        if accept:
            headers["Accept"] = accept
        return self._get(f"https://{host}/v2/{repo}/{path}", headers, True)


# --------------------------------------------------------------------------- #
# Reconstruction
# --------------------------------------------------------------------------- #


@dataclasses.dataclass
class Bundle:
    tag: str
    digest: str
    published: float
    components: dict[str, str]
    manifests_rev: str
    locked_at: float | None = None


@dataclasses.dataclass
class Promotion:
    env: str
    sha: str
    merged: float
    digest: str | None
    gates_at: float | None
    passed: bool
    rerun: bool


@dataclasses.dataclass
class Change:
    sha: str
    merged: float
    title: str
    pos: int
    builds: dict[str, list[dict]]
    policies: dict[str, list[dict]]
    rebuilt: list[str] = dataclasses.field(default_factory=list)


def load_commits(api: Api, repo: str, since: dt.datetime, until: dt.datetime) -> list[dict]:
    q = urllib.parse.urlencode({"sha": "main", "since": since.isoformat(), "until": until.isoformat()})
    commits = api.gh_pages(f"/repos/{repo}/commits?{q}", limit=20)
    # Default branch is linear (squash merges): newest first from the API.
    return [c for c in commits if len(c.get("parents", [])) <= 1]


def load_checks(api: Api, repo: str, sha: str, app: str) -> list[dict]:
    runs = api.gh_pages(f"/repos/{repo}/commits/{sha}/check-runs?filter=all", key="check_runs", limit=5)
    return [r for r in runs if not app or (r.get("app") or {}).get("slug") == app]


def load_bundles(api: Api, lock_repo_ref: str, since: float) -> list[Bundle]:
    host, repo = lock_repo_ref.split("/", 1)
    out: list[Bundle] = []
    for page in range(1, 20):
        q = urllib.parse.urlencode(
            {"onlyActiveTags": "true", "limit": 100, "page": page, "filter_tag_name": "like:release-bundle-%"}
        )
        body = api._get(f"https://{host}/api/v1/repository/{repo}/tag/?{q}", {}, False) or {}
        tags = body.get("tags", [])
        stop = False
        for t in tags:
            published = float(t.get("start_ts") or 0)
            if published < since:
                stop = True
                continue
            content = bundle_json(api, host, repo, t["manifest_digest"])
            if content is None:
                continue
            comps = {
                c["name"]: c.get("source", {}).get("git", {}).get("revision", "")
                for c in content.get("components", [])
            }
            out.append(
                Bundle(
                    tag=t["name"],
                    digest=t["manifest_digest"],
                    published=published,
                    components=comps,
                    manifests_rev=content.get("manifests", {}).get("git", {}).get("revision", ""),
                )
            )
        if stop or not body.get("has_additional"):
            break
    # A digest can carry more than one tag; keep the first publication.
    by_digest: dict[str, Bundle] = {}
    for b in sorted(out, key=lambda b: b.published):
        by_digest.setdefault(b.digest, b)
    return sorted(by_digest.values(), key=lambda b: b.published)


def bundle_json(api: Api, host: str, repo: str, digest: str) -> dict | None:
    manifest = api.registry_json(host, repo, f"manifests/{digest}", "application/vnd.oci.image.manifest.v1+json")
    if not manifest:
        return None
    for layer in manifest.get("layers", []):
        if (layer.get("annotations") or {}).get("org.opencontainers.image.title") == "bundle.json":
            return api.registry_json(host, repo, f"blobs/{layer['digest']}")
    return None


def contents_json(api: Api, repo: str, path: str, ref: str) -> Any:
    body = api.gh(f"/repos/{repo}/contents/{path}?ref={ref}", immutable=True)
    if not body or body.get("encoding") != "base64":
        return None
    raw = base64.b64decode(body["content"].replace("\n", ""))
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return raw.decode()


def lock_digest(api: Api, repo: str, lock_path: str, sha: str) -> str | None:
    lock = contents_json(api, repo, lock_path, sha)
    if not isinstance(lock, dict):
        return None
    return (lock.get("reference") or {}).get("digest")


def parse_strategy(text: str) -> tuple[list[str], dict[str, list[str]], dict[str, list[str]]]:
    """Environments, their dependencies, and their gate keys from a PromotionStrategy.

    A minimal reader for the strategy's own YAML layout: global
    activeCommitStatuses apply to every environment.
    """
    envs: list[str] = []
    deps: dict[str, list[str]] = {}
    gates: dict[str, list[str]] = {}
    global_gates: list[str] = []
    doc = text.split("\n---", 1)[0]
    section = None
    current = None
    list_key = None
    for line in doc.splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        indent = len(line) - len(line.lstrip())
        s = line.strip()
        if indent == 2 and s.endswith(":"):
            section = s[:-1]
            list_key = None
            continue
        if section == "activeCommitStatuses" and s.startswith("- key:"):
            global_gates.append(s.split(":", 1)[1].strip())
        if section != "environments":
            continue
        m = re.match(r"- branch:\s*(\S+)", s)
        if m and indent == 2:
            current = m.group(1)
            envs.append(current)
            deps[current], gates[current] = [], []
            list_key = None
            continue
        if current is None:
            continue
        if s.endswith(":") and not s.startswith("-"):
            list_key = s[:-1]
            continue
        if list_key == "dependsOn" and s.startswith("- "):
            deps[current].append(s[2:].strip())
        elif list_key == "activeCommitStatuses" and s.startswith("- key:"):
            gates[current].append(s.split(":", 1)[1].strip())
    for e in envs:
        gates[e] = sorted(set(global_gates + gates[e]))
    return envs, deps, gates


def env_groups(envs: list[str], deps: dict[str, list[str]], group_re: str) -> list[tuple[str, list[str]]]:
    """Group environments by a branch-name regex; order by promotion-graph layer (FLT-DLV-03)."""
    layer: dict[str, int] = {}

    def depth(e: str, seen: frozenset = frozenset()) -> int:
        if e in layer:
            return layer[e]
        ds = [d for d in deps.get(e, []) if d in deps and d not in seen]
        layer[e] = 0 if not ds else 1 + max(depth(d, seen | {e}) for d in ds)
        return layer[e]

    groups: dict[str, list[str]] = {}
    for e in envs:
        m = re.search(group_re, e)
        groups.setdefault(m.group(1) if m else e, []).append(e)
    return sorted(groups.items(), key=lambda kv: min(depth(e) for e in kv[1]))


def load_promotions(
    api: Api, repo: str, lock_path: str, env: str, gates: list[str], since: dt.datetime, until: dt.datetime
) -> list[Promotion]:
    q = urllib.parse.urlencode({"sha": env, "since": since.isoformat(), "until": until.isoformat()})
    commits = api.gh_pages(f"/repos/{repo}/commits?{q}", limit=10)
    out = []
    for c in commits:
        if len(c.get("parents", [])) < 2:
            continue
        sha = c["sha"]
        meta = contents_json(api, repo, "hydrator.metadata", sha)
        digest = None
        if isinstance(meta, dict) and meta.get("drySha"):
            digest = lock_digest(api, repo, lock_path, meta["drySha"])
        runs = api.gh_pages(f"/repos/{repo}/commits/{sha}/check-runs?filter=all", key="check_runs", limit=3)
        gates_at, passed, rerun = gate_runs(runs, gates, env)
        out.append(Promotion(env, sha, ts(c["commit"]["committer"]["date"]) or 0, digest, gates_at, passed, rerun))
    return sorted(out, key=lambda p: p.merged)


# --------------------------------------------------------------------------- #
# Replay
# --------------------------------------------------------------------------- #


class Replay:
    def __init__(self, args: argparse.Namespace, api: Api):
        self.args = args
        self.api = api

    def load(self) -> None:
        a = self.args
        self.since = dt.datetime.fromisoformat(a.since).replace(tzinfo=dt.timezone.utc)
        self.until = dt.datetime.fromisoformat(a.until).replace(tzinfo=dt.timezone.utc) + dt.timedelta(days=1)
        self.seed = self.since - dt.timedelta(days=a.seed_days)
        # Read promotions past --until so changes merged near the end can finish.
        self.horizon = min(self.until + dt.timedelta(days=a.tail_days), dt.datetime.now(dt.timezone.utc))

        raw = load_commits(self.api, a.app_repo, self.seed, self.until)
        raw.reverse()  # oldest first
        self.changes: list[Change] = []
        for i, c in enumerate(raw):
            runs = load_checks(self.api, a.app_repo, c["sha"], a.check_app)
            builds: dict[str, list[dict]] = {}
            policies: dict[str, list[dict]] = {}
            for r in runs:
                kind = classify_check(r["name"])
                if kind is None:
                    continue
                target = builds if kind[0] == "build" else policies
                target.setdefault(kind[1], []).append(r)
            self.changes.append(
                Change(
                    sha=c["sha"],
                    merged=ts(c["commit"]["committer"]["date"]) or 0,
                    title=c["commit"]["message"].split("\n")[0],
                    pos=i,
                    builds=builds,
                    policies=policies,
                )
            )
        self.pos = {c.sha: c.pos for c in self.changes}
        self.by_sha = {c.sha: c for c in self.changes}

        lock = contents_json(self.api, a.gitops_repo, a.lock_path, "main")
        registry = (lock or {}).get("reference", {}).get("repository", "") if isinstance(lock, dict) else ""
        if a.bundle_repository:
            registry = a.bundle_repository
        self.bundles = load_bundles(self.api, registry, self.seed.timestamp())
        self.bundled = set().union(*(b.components for b in self.bundles)) if self.bundles else set()

        # GitOps update: first release-lock commit on the default branch per digest.
        q = urllib.parse.urlencode(
            {"sha": "main", "path": a.lock_path, "since": self.seed.isoformat(), "until": self.horizon.isoformat()}
        )
        lock_commits = self.api.gh_pages(f"/repos/{a.gitops_repo}/commits?{q}", limit=10)
        self.locks: list[tuple[float, str]] = []
        for c in lock_commits:
            d = lock_digest(self.api, a.gitops_repo, a.lock_path, c["sha"])
            if d:
                self.locks.append((ts(c["commit"]["committer"]["date"]) or 0, d))
        self.locks.sort()
        by_digest = {b.digest: b for b in self.bundles}
        for t, d in self.locks:
            b = by_digest.get(d)
            if b and b.locked_at is None:
                b.locked_at = t

        text = contents_json(self.api, a.gitops_repo, a.strategy_path, "main")
        envs, deps, gates = parse_strategy(text if isinstance(text, str) else "")
        self.envs = envs
        self.deps = effective_deps(envs, deps)
        self.groups = env_groups(envs, self.deps, a.env_group)
        self.promotions: dict[str, list[Promotion]] = {}
        for e in envs:
            self.promotions[e] = load_promotions(
                self.api, a.gitops_repo, a.lock_path, e, gates[e], self.seed, self.horizon
            )

        for c in self.changes:
            c.rebuilt = sorted(
                comp
                for comp, runs in c.builds.items()
                if comp in self.bundled and any(r.get("conclusion") not in IGNORABLE for r in runs)
            )

    # -- per-change actual timeline ------------------------------------------

    def contains(self, b: Bundle, c: Change) -> bool:
        return bundle_contains(b.components, c.rebuilt, self.pos, c.pos, b.manifests_rev)

    def first_bundle(self, c: Change, manifest_rule: bool = False) -> Bundle | None:
        for b in self.bundles:
            if b.published < c.merged:
                continue
            ok = (
                manifest_contains(b.manifests_rev, self.pos, c.pos)
                if manifest_rule
                else self.contains(b, c)
            )
            if ok:
                return b
        return None

    @staticmethod
    def latest(runs: list[dict]) -> dict | None:
        live = [r for r in runs if r.get("conclusion") not in IGNORABLE]
        return max(live, key=lambda r: r.get("started_at") or "") if live else None

    def own_build_ok(self, c: Change) -> bool:
        """Every rebuilt component built and passed its policy check on this commit."""
        for comp in c.rebuilt:
            b = self.latest(c.builds.get(comp, []))
            p = self.latest(c.policies.get(comp, []))
            if not b or b.get("conclusion") not in SUCCEEDED:
                return False
            if not p or p.get("conclusion") not in SUCCEEDED:
                return False
        return True

    def timeline(self, c: Change) -> dict[str, float | None] | None:
        """End time of each phase for the change, or None when it never got a bundle."""
        out: dict[str, float | None] = {}
        if c.rebuilt and self.own_build_ok(c):
            out["build"] = max(ts(self.latest(c.builds[k])["completed_at"]) or 0 for k in c.rebuilt)
            out["policy"] = max(ts(self.latest(c.policies[k])["completed_at"]) or 0 for k in c.rebuilt)
        b = self.first_bundle(c)
        if b is None:
            return None
        out["release"] = b.published
        gitops = next((t for t, d in self.locks if (bb := self.by_digest(d)) and self.contains(bb, c)), None)
        out["gitops"] = gitops
        prev = gitops
        for g, envs in self.groups:
            end = None
            for e in envs:
                finish = None
                for p in self.promotions.get(e, []):
                    bb = self.by_digest(p.digest) if p.digest else None
                    if bb and self.contains(bb, c) and p.passed:
                        finish = p.gates_at
                        break
                if finish is None:
                    end = None
                    break
                end = finish if end is None else max(end, finish)
            out[f"env:{g}"] = end if (end is not None and prev is not None) else None
            prev = out[f"env:{g}"]
        return out

    def by_digest(self, d: str | None) -> Bundle | None:
        if not hasattr(self, "_by_digest"):
            self._by_digest = {b.digest: b for b in self.bundles}
        return self._by_digest.get(d or "")

    # -- phase runs (sampled once per execution) ------------------------------

    def phase_runs(self) -> dict[str, list[Run]]:
        runs: dict[str, list[Run]] = {}

        def add(key: str, start: float | None, end: float | None) -> None:
            if start is None or end is None or end < start:
                return
            runs.setdefault(key, []).append(Run(end, end - start))

        for c in self.changes:
            for comp, rs in c.builds.items():
                live = [r for r in rs if r.get("conclusion") not in IGNORABLE]
                if len(live) != 1 or live[0].get("conclusion") not in SUCCEEDED:
                    continue
                bend = ts(live[0].get("completed_at"))
                add(f"build:{comp}", c.merged, bend)
                ps = [r for r in c.policies.get(comp, []) if r.get("conclusion") not in IGNORABLE]
                if len(ps) == 1 and ps[0].get("conclusion") in SUCCEEDED:
                    add(f"policy:{comp}", bend, ts(ps[0].get("completed_at")))
        for b in self.bundles:
            mc = self.by_sha.get(b.manifests_rev)
            if mc is not None:
                ends = [
                    ts(self.latest(mc.policies[k])["completed_at"])
                    for k, rev in b.components.items()
                    if rev == b.manifests_rev and self.latest(mc.policies.get(k, []))
                    and self.latest(mc.policies[k]).get("conclusion") in SUCCEEDED
                ]
                ends = [e for e in ends if e is not None]
                if ends:
                    add("release", max(ends), b.published)
            add("gitops", b.published, b.locked_at)
        # Environment: one run per passed promotion, from its merge on the
        # environment branch to its gates' first pass. Groups are not sampled:
        # their estimate follows the dependsOn graph (FLT-DLV-09).
        for e, promos in self.promotions.items():
            for p in promos:
                if p.passed and not p.rerun:
                    add(env_phase(e), p.merged, p.gates_at)
        return runs

    # -- estimates for a change at a moment -----------------------------------

    def phase_estimate(self, runs: dict[str, list[Run]], c: Change, phase: str, at: float) -> Estimate | None:
        if phase in ("build", "policy"):
            comps = c.rebuilt or sorted(c.builds)
            return combine(estimate(runs.get(f"{phase}:{k}", []), at) for k in comps)
        if phase.startswith("env:"):
            env_est = {e: estimate(runs.get(env_phase(e), []), at) for e in self.deps}
            return group_estimates(env_est, self.deps, self.groups).get(phase[len("env:"):])
        return estimate(runs.get(phase, []), at)

    def phases(self, c: Change) -> list[str]:
        out = ["build", "policy"] if c.rebuilt else []
        return out + ["release", "gitops"] + [f"env:{g}" for g, _ in self.groups]

    # -- report ----------------------------------------------------------------

    def report(self) -> dict[str, Any]:
        runs = self.phase_runs()
        lo, hi = self.since.timestamp(), self.until.timestamp()
        in_range = [c for c in self.changes if lo <= c.merged < hi]
        last = f"env:{self.groups[-1][0]}" if self.groups else None

        totals, miss_merge, miss_pub, miss_last = [], [], [], []
        in_range_hits = 0
        range_n = 0
        flags = {r: 0 for r in RULES}
        false = {r: 0 for r in RULES}
        caught = {r: 0 for r in RULES}
        stalls = 0
        overdue_detail: list[str] = []

        for c in in_range:
            tl = self.timeline(c)
            if tl is None or last is None or tl.get(last) is None:
                continue
            actual = tl[last]
            totals.append(actual - c.merged)
            ph = self.phases(c)

            def view(at: float, done: set[str], current: str | None, elapsed: float) -> list[PhaseView]:
                return [
                    PhaseView(
                        passed=p in done,
                        elapsed=elapsed if p == current else 0.0,
                        est=self.phase_estimate(runs, c, p, at),
                    )
                    for p in ph
                ]

            # At merge.
            a = arrival(c.merged, view(c.merged, set(), ph[0], 0.0), published=False)
            if a is not None:
                mid = (a[0] + a[1]) / 2
                miss_merge.append(abs(mid - actual))
                range_n += 1
                in_range_hits += a[0] <= actual <= a[1]
            # At publication.
            pub = tl["release"]
            done = {p for p in ph if p in ("build", "policy", "release")}
            a = arrival(pub, view(pub, done, "gitops", 0.0), published=True)
            if a is not None:
                miss_pub.append(abs(a[0] - actual))
            # At entry to the last group's phase.
            idx = ph.index(last)
            entry = tl.get(ph[idx - 1])
            if entry is not None:
                done = set(ph[:idx])
                a = arrival(entry, view(entry, done, last, 0.0), published=True)
                if a is not None:
                    miss_last.append(abs(a[0] - actual))

            # Overdue: each phase's final duration against its estimate at its start.
            prev = c.merged
            for p in ph:
                end = tl.get(p)
                if end is None:
                    break
                if prev is None or end < prev:
                    prev = end
                    continue
                est = self.phase_estimate(runs, c, p, prev)
                d = end - prev
                f = overdue_flags(d, est)
                stall = real_stall(d, est)
                stalls += stall
                for r in RULES:
                    flags[r] += f[r]
                    false[r] += f[r] and not stall
                    caught[r] += f[r] and stall
                if stall:
                    overdue_detail.append(
                        f"{c.sha[:7]} {p}: {minutes(d)} min (estimate {minutes(est.median)}, slowest {minutes(est.slowest)})"
                    )
                prev = end

        misplaced, gap = 0, 0.0
        with_builds = [c for c in in_range if c.rebuilt]
        for c in with_builds:
            new, old = self.first_bundle(c), self.first_bundle(c, manifest_rule=True)
            if new is not old:
                misplaced += 1
                if new and old:
                    gap = max(gap, abs(new.published - old.published))

        def med(v: list[float]) -> float | None:
            return statistics.median(v) if v else None

        return {
            "range": f"{self.args.since}..{self.args.until}",
            "changes_merged": len(in_range),
            "changes_delivered": len(totals),
            "merge_to_last_median_min": minutes(med(totals)),
            "merge_to_last_fastest_min": minutes(min(totals) if totals else None),
            "arrival_miss_at_merge_min": minutes(med(miss_merge)),
            "arrival_in_range_at_merge": f"{in_range_hits}/{range_n}",
            "arrival_miss_at_publish_min": minutes(med(miss_pub)),
            "arrival_miss_at_last_group_min": minutes(med(miss_last)),
            "overdue": {
                r: {"flags": flags[r], "false_alarms": false[r], "stalls_caught": f"{caught[r]}/{stalls}"}
                for r in RULES
            },
            "real_stalls": overdue_detail,
            "containment": {
                "changes_with_builds": len(with_builds),
                "misplaced_by_manifest_rule": misplaced,
                "largest_gap_hours": round(gap / 3600, 1),
            },
            "samples": {k: len(v) for k, v in sorted(runs.items())},
            "groups": [g for g, _ in self.groups],
            "api_calls": self.api.calls,
        }


def github_token() -> str:
    tok = os.environ.get("GITHUB_TOKEN", "")
    if tok:
        return tok
    try:
        return subprocess.run(["gh", "auth", "token"], capture_output=True, text=True, check=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return ""


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--app-repo", required=True, help="application repository, owner/name")
    p.add_argument("--gitops-repo", required=True, help="GitOps repository, owner/name")
    p.add_argument("--strategy-path", required=True, help="PromotionStrategy YAML path in the GitOps repository")
    p.add_argument("--lock-path", default="versions/hypershell-release-lock.json")
    p.add_argument("--bundle-repository", default="", help="host/repo of the bundles; default: the lock's reference")
    p.add_argument("--env-branch-prefix", default="env/", help="environment branch prefix (informational)")
    p.add_argument(
        "--env-group",
        default=r"^env/([^-]+)",
        help="regex on the environment branch; group 1 is the environment group "
        "(stands in for the delivery env label, which the GitHub API cannot read)",
    )
    p.add_argument("--check-app", default="red-hat-konflux")
    p.add_argument("--since", required=True, help="first merge day, YYYY-MM-DD")
    p.add_argument("--until", required=True, help="last merge day, YYYY-MM-DD (inclusive)")
    p.add_argument("--seed-days", type=int, default=7, help="history before --since that seeds estimates")
    p.add_argument("--tail-days", type=int, default=3, help="promotions read after --until")
    p.add_argument("--cache-dir", default=os.path.join(tempfile.gettempdir(), "delivery-replay-cache"))
    p.add_argument("--refresh-lists", action="store_true", help="re-read list endpoints instead of the cache")
    p.add_argument("--json", action="store_true", help="print the report as JSON")
    a = p.parse_args(argv)

    if not a.env_group.startswith("^" + re.escape(a.env_branch_prefix)) and a.env_branch_prefix not in a.env_group:
        print(f"note: --env-group does not mention prefix {a.env_branch_prefix!r}", file=sys.stderr)

    api = Api(Path(a.cache_dir), github_token(), a.refresh_lists)
    replay = Replay(a, api)
    replay.load()
    report = replay.report()
    if a.json:
        print(json.dumps(report, indent=2))
        return 0
    for k, v in report.items():
        if isinstance(v, dict):
            print(f"{k}:")
            for kk, vv in v.items():
                print(f"  {kk}: {vv}")
        elif isinstance(v, list):
            print(f"{k}:")
            for item in v:
                print(f"  {item}")
        else:
            print(f"{k}: {v}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
