---
name: release-verification
description: >
  Release Verification Workflow. Watches a published release bundle through
  delivery and post-delivery analysis, then classifies the release as verified
  or failed.
---

# Workflow

Release verification workflow that classifies a released bundle as:
- `agent/release-verified`
- `agent/release-verification-failed`
- `agent/blocked`

A release is **verified** when the released bundle is published, promoted into
the target environment through the repository's normal delivery automation, and
the delivery's post-delivery analysis completes successfully.

A release is **failed** when a stage produces a positive negative signal: the
release's merge gating fails, the delivery does not converge on the released
bundle, or the post-delivery analysis completes with a failing result.

A release is **blocked** when the workflow cannot reach a verdict within its
time budget or lacks the access to observe a stage: the bundle never publishes,
the release change never appears or never merges, or the cluster/API is
unreachable. `blocked` means "undetermined", not "bad".

This workflow is an **observer**. It does not merge the release change or
mutate the cluster; the repository's own automation performs delivery. The only
WRITE actions are labels and comments on `$GITHUB_ISSUE_URL`.

This workflow is intentionally **generic**. It names no specific status checks,
Application names, namespaces, or analysis resources, so that it applies to any
delivery strategy. A repository supplies those specifics through the
[Delivery Profile Overlay](#step-2-load-the-delivery-profile-overlay).

## Input

```text
$GITHUB_ISSUE_URL $RELEASE_BUNDLE $GITOPS_REPO $RELEASE_STRATEGY_OVERLAY $KUBECONFIG [--dry-run]
```

Supported arguments:
- GITHUB_ISSUE_URL -- the tracking issue for this release; verdict labels and
  comments are applied here.
- RELEASE_BUNDLE -- the released bundle reference to verify. Eg `quay.io/redhat-user-workloads/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main:release-bundle-20261006T213842000000Z-04477525054a5901`.
- GITOPS_REPO -- the repository in which the release/promotion change is opened
  and merged, and which MAY contain the Delivery Profile overlay.
- KUBECONFIG -- a kubeconfig granting read-only access to Argo CD and the kube
  resources of the environment to which `$RELEASE_BUNDLE` is delivered.
- RELEASE_STRATEGY_OVERLAY -- a path relative to $GITOPS_REPO that contains the delivery profile overlay.
- --dry-run -- [Optional] Execute the workflow, but skip every WRITE action
  (label and comment). All read and verify actions still run.

## Time budget

Each wait below has a timeout. Defaults are listed per step and MAY be
overridden by the Delivery Profile. On timeout, the stage is `blocked` (see
[Blocking](#blocking)) unless a positive failure signal was already observed, in
which case it is `failed`.

## Workflow

_Note: If a label does not exist, create it._

### Step 1: Read Issue and Linked Change

Using the GitHub API, fetch `$GITHUB_ISSUE_URL`.

Read the issue title and body and identify any linked PR or release change.

Familiarize yourself with its contents.

Output: (`$LINKED_CHANGE`, `$ISSUE_CONTENTS`)

### Step 2: Load the Release Strategy (Overlay)

This workflow is generic; the repository provides the concrete details of its
release strategy through an optional overlay.

Look for a Release Strategy in `$GITOPS_REPO` at `$RELEASE_STRATEGY_OVERLAY`. If one exists, read it and treat its
instructions as **authoritative overrides and extensions** to the steps below.

A Release Strategy MAY specify:
- how to locate the release/promotion change for a given `$RELEASE_BUNDLE`;
- the target environment, and how to select the Argo CD Application(s) and
  namespaces that deliver it;
- how to recognize the post-delivery analysis and read its result;
- the registry location and how to confirm a bundle is published;
- step timeouts that replace the defaults.

Where the Strategy and these generic steps conflict, the Strategy wins. Where the
Strategy is silent, use the generic default and infer details from
`$LINKED_CHANGE`, the release change, and cluster state.

Output: `$RELEASE_STRATEGY` (may be empty)

### Step 3: Confirm the Bundle Is Published

Confirm that the image(s) referenced by `$RELEASE_BUNDLE` is present in its
registry (for example, by inspecting the manifest). Use the method named in
`$RELEASE_STRATEGY` if one is given.

Default timeout: 30 minutes. If the bundle is not published within the timeout,
`block` and STOP.

Output: `$BUNDLE_PUBLISHED` (true)

### Step 4: Verify the Release Change Exists

Identify the change in `$GITOPS_REPO` that delivers `$RELEASE_BUNDLE` to the
target environment (the promotion/release PR or commit). Use the location method
from `$RELEASE_STRATEGY` if given; otherwise match on the `$RELEASE_BUNDLE`
reference appearing in the change.

Default timeout: 10 minutes. If no such change appears within the timeout,
`block` and STOP.

Output: `$RELEASE_CHANGE`

### Step 5: Confirm the Release Merges

Wait for `$RELEASE_CHANGE` to merge through the repository's normal process.
Do NOT merge it yourself.

- If the change's required merge gating reports failure, the release has
  `failed`. Record the failing signal and continue to
  [Step 8](#step-8-communicate-outcome).
- If the change does not merge within the timeout and no failure signal is
  present, `block` and STOP.


Default timeout: 30 minutes.

Output: (`$MERGED` (true), or `$FAILURE_RATIONALE`)

### Step 6: Verify Delivery

Using `$KUBECONFIG`, confirm the Argo CD Application(s) that manage the target
environment converge on the released bundle:
- they report `Synced` and `Healthy`; and
- the delivered revision/images correspond to `$RELEASE_BUNDLE`.

Select the Application(s) and namespaces as directed by `$RELEASE_STRATEGY`;
absent a Profile, infer them from `$RELEASE_CHANGE`.

- If an Application settles in a degraded or sync-error state, or converges on a
  revision that does not correspond to `$RELEASE_BUNDLE`, the release has
  `failed`. Record the rationale and continue to
  [Step 8](#step-8-communicate-outcome).
- If delivery does not converge within the timeout and no failure state is
  present, `block` and STOP.

Default timeout: 30 minutes.

Output: (`$DELIVERED` (true), or `$FAILURE_RATIONALE`)

### Step 7: Verify Post-Delivery Analysis

Using `$KUBECONFIG`, confirm the post-delivery analysis associated with this
delivery completes successfully. Identify the analysis as directed by
`$RELEASE_STRATEGY`; absent a Profile, infer it from the delivered resources.

- If the analysis completes with a failing result, the release has `failed`.
  Record the rationale and continue to [Step 8](#step-8-communicate-outcome).
- If the analysis does not start or does not complete within the timeout, and no
  failing result is present, `block` and STOP.

Default timeout: 30 minutes.

Output: (`$ANALYSIS_PASSED` (true), or `$FAILURE_RATIONALE`)

### Step 8: Communicate Outcome

_Note: If a label does not exist, create it._

Determine `$VERDICT`:
- `agent/release-verified` -- Steps 3 through 7 all succeeded.
- `agent/release-verification-failed` -- any step recorded a `$FAILURE_RATIONALE`.

Apply `$VERDICT` to `$GITHUB_ISSUE_URL`.

IF `$VERDICT` == `agent/release-verification-failed`:
- Add a comment to `$GITHUB_ISSUE_URL` communicating `$FAILURE_RATIONALE`,
  naming the stage that failed and the observed signal, that adheres to the
  [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English)
  standard. Report gating generically by its outcome; do not name individual checks.
- Create a subissue of `$GITHUB_ISSUE_URL` that contains sufficient detail (but no sensitive information)
  about why the release failed and what is required to fix it.

## Blocking

When a step directs you to `block`:
- apply label `agent/blocked` to `$GITHUB_ISSUE_URL`;
- add a comment to `$GITHUB_ISSUE_URL` noting the stage reached and the reason it
  is blocked (timeout, missing access, or ambiguity), in
  [simplified technical english](https://en.wikipedia.org/wiki/Simplified_Technical_English);
- STOP. No further action.

Under `--dry-run`, skip the label and comment but still STOP.
