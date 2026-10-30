---
name: funnel-reconcile
description: "Reconcile one execution record with Jira, including terminal merge."
---

Read `/tmp/funnel-workflow.md` and execute that reconcile workflow for exactly
`GITHUB_ISSUE_URL`. `PREVIOUS_PHASE` contains the phase-label set before the last
worker attempt, when known. Compare it with the current phase to decide whether
a transition comment is needed. Resolve the human owner and preserve attribution.
The repository is `REPOSITORY`; use `JIRA_BASE_URL` as the configured Jira tenant.
Use `gh` for GitHub and Node HTTPS requests for Jira; no MCP tools.

Use the Jira helper for every Jira request, in either authentication mode:

```sh
node /tmp/funnel-jira-request.mjs GET '/rest/api/3/issue/KEY-123'
node /tmp/funnel-jira-request.mjs POST '/rest/api/3/search/jql' request.json
```

Pass request JSON through a file or stdin (`-`). Paginate requests explicitly.
The helper routes API tokens to the tenant and ACLI OAuth to the selected cloud
API, and reads its private credential snapshot internally. Never print or copy
`/tmp/.funnel-jira.json`, construct authentication headers yourself, or use
`acli auth login` in the sandbox. OAuth refresh stays on the host. If the helper
reports expiry or HTTP 401/403, record a failure and stop; the dispatcher retries
with a fresh host snapshot. Do not report a partial sync as success.

Enforce `agent/allowed` and `LABEL_SCOPE` again immediately before writes. Honor
`DRY_RUN=true`. Follow the workflow's canonical-link-only resolution, authority,
idempotency, transition-category, remote-link, terminal-closure, and failure
reporting rules. Resolve PRs only from an explicit PR URL in the execution
record or a PR closing reference to that exact issue. Paginate discovery and
stop with an error on multiple matching PRs. Never select the newest match or
infer a relationship from a descriptive Jira-key mention. A closed record must
still project Done when no merged PR can be detected.

Write `$FULLSEND_OUTPUT_DIR/result.json` with this shape:

```json
{"status":"success","errors":[],"records":[{"url":"https://github.com/owner/repo/issues/1"}]}
```

Use `status: "failed"` and list every error in `errors` if any write failed or
resolution was ambiguous. A partial completion is a failure. An unauthorized
record is a successful no-write skip, and must be reported in the result.
