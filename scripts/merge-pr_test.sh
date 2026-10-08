#!/usr/bin/env bash
# Exercise discovery and merge decisions without GitHub writes or credentials.
set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
python3 - "$repo_root" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

root = Path(sys.argv[1])
with tempfile.TemporaryDirectory() as temporary:
    work = Path(temporary)
    fake = work / "gh"
    fake.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
fixture = json.loads(Path(os.environ["MERGE_FIXTURE"]).read_text())
with open(os.environ["MERGE_CALLS"], "a") as log:
    log.write(json.dumps(args) + "\\n")
if fixture.get("api_failure"):
    sys.exit(2)
if args[:2] == ["issue", "view"]:
    result = fixture["issue"]
elif args[:2] == ["pr", "view"]:
    result = fixture["prs"][args[2]]
elif args[:2] == ["pr", "checks"]:
    result = fixture["checks"]
elif args[:2] == ["api", "graphql"]:
    query = next(value[6:] for value in args if value.startswith("query="))
    if "enqueuePullRequest" in query:
        assert "head=abc123" in args, "enqueue must pin the checked head"
        if fixture.get("enqueue_failure"):
            sys.exit(1)
        result = {"data": {"enqueuePullRequest": {"mergeQueueEntry": {"id": "entry"}}}}
    elif "issue(number:" in query:
        result = fixture["timeline"]
    else:
        result = fixture["queue"]
elif args[0] == "api":
    result = fixture["discovery"]
elif args[:2] in (["label", "create"], ["issue", "edit"]):
    sys.exit(0)
else:
    raise AssertionError(args)
print(json.dumps(result))
if args[:2] == ["pr", "checks"]:
    sys.exit(fixture.get("checks_exit", 0))
''')
    fake.chmod(0o755)
    env = dict(os.environ, PATH=f"{work}:{os.environ['PATH']}", REPOSITORY="org/repo",
               ITEM_NUMBER="1", DRY_RUN="false", MAX_ITEMS="2",
               MERGE_FIXTURE=str(work / "fixture.json"), MERGE_CALLS=str(work / "calls"))
    approval = {"createdAt": "2026-10-01T00:00:00Z", "label": {"name": "agent/review-code-approved"}}
    reference = {"source": {"number": 2, "repository": {"nameWithOwner": "org/repo"}}}
    def base():
        return {"issue": {"state": "OPEN", "labels": [{"name": "agent/review-code-approved"}], "body": ""},
                "prs": {"2": {"number": 2, "state": "OPEN", "id": "PR2", "headRefOid": "abc123",
                               "mergeable": "MERGEABLE", "isDraft": False}},
                "timeline": [{"data": {"repository": {"issue": {"timelineItems": {"nodes": [approval]}}}}},
                             {"data": {"repository": {"issue": {"timelineItems": {"nodes": [reference]}}}}}],
                "queue": [{"data": {"node": {"mergeQueueEntry": None, "timelineItems": {"nodes": []}}}}],
                "checks": [{"bucket": "pass"}]}
    def execute(fixture, script="run.sh", dry=False):
        (work / "fixture.json").write_text(json.dumps(fixture))
        (work / "calls").write_text("")
        # Redirect the harness result into the fixture directory for test isolation.
        source = (root / ".hypershell/agents/merge-pr" / script).read_text()
        source = source.replace("readonly result_file=/tmp/result.json", f"readonly result_file={work}/result.json")
        (work / "result.json").unlink(missing_ok=True)
        run = subprocess.run(["bash", "-c", source], env=dict(env, DRY_RUN=str(dry).lower()),
                             capture_output=True, text=True)
        calls = [json.loads(line) for line in (work / "calls").read_text().splitlines()]
        mutations = [call for call in calls if call[:2] in (["label", "create"], ["issue", "edit"])
                     or any("mutation(" in arg for arg in call)]
        result = json.loads((work / "result.json").read_text()) if script == "run.sh" else None
        return run, result, mutations
    tested = 0
    def verify(fixture, status, action=None, dry=False, failure=False):
        global tested
        run, result, mutations = execute(fixture, dry=dry)
        assert (run.returncode != 0) == failure, (run.stderr, result)
        assert result["status"] == status, result
        if action == "enqueue":
            assert len(mutations) == 1 and "enqueuePullRequest" in " ".join(mutations[0]), mutations
        elif action in ("labels", "merged"):
            assert len(mutations) == 3, mutations
            label = "agent/merged" if action == "merged" else "agent/cant-merge"
            assert "--add-label" in mutations[1] and label in mutations[1], mutations
            assert "--remove-label" in mutations[2] and "agent/review-code-approved" in mutations[2], mutations
        else:
            assert not mutations, mutations
        tested += 1
    verify(base(), "success", "enqueue")
    verify(base(), "success", dry=True)
    for bucket, code in [("fail", 1), ("cancel", 1)]:
        fixture = base(); fixture.update(checks=[{"bucket": bucket}], checks_exit=code)
        verify(fixture, "success", "labels")
        verify(fixture, "success", dry=True)
    fixture = base(); fixture["prs"]["2"]["mergeable"] = "CONFLICTING"
    verify(fixture, "success", "labels")
    verify(fixture, "success", dry=True)
    fixture = base(); fixture.update(checks=[{"bucket": "pending"}], checks_exit=8)
    verify(fixture, "skip")
    for field, value in [("mergeable", "UNKNOWN"), ("isDraft", True), ("state", "CLOSED")]:
        fixture = base(); fixture["prs"]["2"][field] = value
        verify(fixture, "skip")
    fixture = base(); fixture["prs"]["2"]["state"] = "MERGED"
    verify(fixture, "success", "merged")
    verify(fixture, "success", dry=True)
    fixture["issue"]["state"] = "CLOSED"
    verify(fixture, "success", "merged")
    verify(fixture, "success", dry=True)
    fixture["prs"]["2"]["state"] = "OPEN"
    verify(fixture, "skip")
    fixture = base(); fixture["queue"][0]["data"]["node"]["mergeQueueEntry"] = {"id": "entry"}
    verify(fixture, "skip")
    fixture["checks"] = [{"bucket": "fail"}]; fixture["checks_exit"] = 1
    verify(fixture, "success", "labels")
    fixture = base()
    fixture["queue"][0]["data"]["node"]["timelineItems"]["nodes"] = [
        {"__typename": "AddedToMergeQueueEvent", "createdAt": "2026-10-02T00:00:00Z"},
        {"__typename": "RemovedFromMergeQueueEvent", "createdAt": "2026-10-03T00:00:00Z"}]
    verify(fixture, "success", "labels")
    verify(fixture, "success", dry=True)
    fixture["timeline"][0]["data"]["repository"]["issue"]["timelineItems"]["nodes"] = [
        dict(approval, createdAt="2026-10-04T00:00:00Z")]
    verify(fixture, "success", "enqueue")
    fixture = base(); fixture["issue"]["labels"] = []
    verify(fixture, "skip")
    fixture = base(); fixture["timeline"] = []
    verify(fixture, "skip")
    fixture["issue"]["body"] = "Implementation: https://github.com/org/repo/pull/2"
    verify(fixture, "success", "enqueue")
    fixture = base()
    fixture["timeline"][1]["data"]["repository"]["issue"]["timelineItems"]["nodes"].append(
        {"subject": {"number": 3, "repository": {"nameWithOwner": "org/repo"}}})
    fixture["prs"]["3"] = dict(fixture["prs"]["2"], number=3)
    verify(fixture, "skip")
    fixture = base(); fixture["api_failure"] = True
    verify(fixture, "failed", failure=True)
    fixture = base(); fixture["enqueue_failure"] = True
    run, result, mutations = execute(fixture)
    assert run.returncode != 0 and result["status"] == "failed"
    assert len(mutations) == 1  # Only the attempted enqueue, no relabeling.
    tested += 1
    fixture = {"discovery": [[{"number": 4, "html_url": "u4", "title": "four"},
                               {"number": 2, "html_url": "pr", "title": "PR", "pull_request": {}}],
                              [{"number": 3, "html_url": "u3", "title": "three"},
                               {"number": 1, "html_url": "u1", "title": "one"}]]}
    run, _, mutations = execute(fixture, "discover.sh")
    assert run.returncode == 0 and not mutations
    assert [json.loads(line)["number"] for line in run.stdout.splitlines()] == [1, 3], run.stdout
    fixture["api_failure"] = True
    run, _, mutations = execute(fixture, "discover.sh")
    assert run.returncode == 0 and not run.stdout and not mutations
    tested += 2
    print(f"merge-pr: {tested} scenarios passed")
PY
