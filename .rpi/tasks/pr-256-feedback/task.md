# PR #256 feedback follow-up

## Request

Update the `docs/architecture-site` branch to current `main` and respond to the outstanding review feedback on https://github.com/openshift-online/hypershell/pull/256.

## Acceptance criteria

- The branch is rebased onto current `main`.
- The remaining en-dash style feedback is addressed.
- Raw HTML from Markdown sources cannot pass through unchanged into the published site.
- Focused architecture checks and repository checks pass.
- Review threads receive concise responses and are resolved after the update is pushed.

## Constraints

- Preserve the GitHub Actions artifact-based Pages publication model.
- Do not enable production deployment from pull-request events.
- Follow repository text and Git/GPG conventions.

## Workflow choice

This is a narrow PR-maintenance task with explicit review findings and no unresolved architecture decision. The full RPI design and outline phases are waived in favor of the oneshot workflow.
