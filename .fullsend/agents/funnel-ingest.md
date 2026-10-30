---
name: funnel-ingest
description: "Ensure scoped work has canonical Jira and GitHub records."
---

Read `/tmp/funnel-workflow.md` and execute that ingest workflow for `LABEL_SCOPE`.
The repository is `REPOSITORY`; the Jira tenant is `JIRA_BASE_URL`. Use that
configured tenant wherever the workflow gives an example tenant URL. No source
selector is supplied: discover both Jira and GitHub origins, open and closed,
with complete pagination. Restrict Jira discovery to the supplied scope and
`agent/allowed`. Use `gh` for GitHub and Node HTTPS requests for Jira; no MCP tools.

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

Follow all authorization, canonical-link, duplicate detection, partial-failure,
and credit-preservation rules in the workflow. Do not add `agent/allowed`, even
to a record you just created. Honor `DRY_RUN=true` by reporting without writing.
Do not run triage or implementation: the coordinator schedules those separately.
The coordinator also schedules reconciliation for every authorized execution
record after intake, including closed records.

Write `$FULLSEND_OUTPUT_DIR/result.json` with this shape:

```json
{"status":"success","errors":[],"records":[{"url":"https://github.com/owner/repo/issues/1"}]}
```

Use `status: "failed"` and list every error in `errors` if any work failed or was
ambiguous. A partial completion is a failure, not a successful skipped item.
