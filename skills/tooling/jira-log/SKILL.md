---
name: jira-log
description: >
  Log Jira issues to the HYPERSHELL project with team:hypershell label
  pre-filled. Makes issues agent-actionable from cold start. Supports
  single tickets and batch creation from bullet lists. Use whenever the
  user wants to create, log, file, track, or open Jira issues — stories,
  bugs, tasks, spikes, epics, cards, or tickets. Also triggers on batch
  creation requests and bullet lists of work items.
---

# Jira Issue Logger

Create well-structured Jira issues in the HYPERSHELL project with the `team:hypershell` label pre-filled. Every issue is built to be agent-actionable from a cold start — meaning another agent (or human) can pick it up and start working immediately without asking clarifying questions.

## What Makes a Jira Agent-Actionable

A Jira is ready for cold-start work when it has: a user story (who/why), acceptance criteria (definition of done), repo + file paths (where to edit), constraints (what not to do), and testing requirements (expected coverage). Bug reports additionally need repro steps. Spikes need deliverables and a time-box. Epics need an overview and linked children — their actionability comes from the children being well-structured, not from the epic itself.

This principle drives every step below — when gathering context, building descriptions, or reviewing batch tickets, ask yourself whether an agent picking this up cold would have enough information to start working without asking questions.

## User Input

```text
$ARGUMENTS
```

Consider the user input before proceeding (if not empty).

## Recognizing Single vs Batch Mode

**Single ticket** (default): The input is a sentence, paragraph, or block of text describing one piece of work.

**Batch mode**: The input contains a markdown bullet list (lines starting with `- ` or `* `). Each top-level bullet becomes a separate ticket. Sub-bullets provide context for that ticket's description. The reason batch mode skips interactive prompting is that asking questions for each of 10+ tickets would be exhausting — instead, use sub-bullet context and reasonable defaults, then confirm the full batch before creating.

## Execution

### Step 1 — Parse

Extract from user input, per ticket:

| Field | Default | Notes |
|-------|---------|-------|
| Summary | (required) | Title of the issue |
| Issue Type | Story | Also: Bug, Task, Spike, Epic. Normalize case. |
| Priority | (inferred or ask) | See priority inference below. Values: Blocker, Critical, Major, Normal, Minor. |
| Activity Type | (inferred) | See inference rules below. |
| Description | (from context) | Sub-bullets, multi-line text, or gathered interactively |
| Epic link | — | If a ticket should belong to an epic |
| Blocking | — | "X blocks Y" relationships |
| Related | — | "X related to Y" relationships |

Type prefix syntax: `[Bug] Session crashes` → type=Bug, summary="Session crashes".

#### Activity Type Inference

Infer `Activity Type` (`customfield_10464`) from the issue type and content. The user can
override by specifying it explicitly (e.g. `Activity: Security & Compliance`).

| Signal | Activity Type |
|--------|---------------|
| New features, stories, product capabilities, CLI commands, UI views, API endpoints | `Product / Portfolio Work` |
| Bugs, alerts, stability fixes, quality improvements, rebranding, lint/format cleanup | `Quality / Stability / Reliability` |
| Security hardening, CVEs, compliance, image migrations, RBAC, credential handling | `Security & Compliance` |
| Tech debt, refactoring, long-term sustainability, dependency upgrades, code cleanup | `Future Sustainability` |

**Decision order** (first match wins):
1. User explicitly states an activity type → use it verbatim
2. Keywords: security, CVE, compliance, vulnerability, RBAC, credential, image migration → `Security & Compliance`
3. Issue type is Bug, or keywords: alert, FIRING, incident, flaky, regression, stability → `Quality / Stability / Reliability`
4. Keywords: tech debt, refactor, deprecat, sustainab, dependency upgrade, cleanup → `Future Sustainability`
5. Default (features, stories, new capabilities, spikes, tests for product work) → `Product / Portfolio Work`

#### Priority Inference

Infer `priority` from the issue context. Available values (highest to lowest):
`Blocker` > `Critical` > `Major` > `Normal` > `Minor`.

**Decision order** (first match wins):
1. User explicitly states a priority → use it verbatim
2. Keywords or context: blocks release, blocks deployment, production down, outage, data loss, `[FIRING]` → `Blocker`
3. Keywords: security vulnerability, CVE with high/critical CVSS, breaks existing functionality, regression in core flow → `Critical`
4. Bugs affecting users, important features on a deadline, compliance-driven work → `Major`
5. Nice-to-have improvements, minor polish, low-impact cleanup → `Minor`
6. **Cannot infer** → ask the user: "What priority should this have?" with options `Blocker`, `Critical`, `Major`, `Normal`, `Minor`

In **batch mode**, do not ask per-ticket. Instead, default unresolvable priorities to `Normal` and note
it in the confirmation table so the user can edit before creation.

### Step 2 — Gather Context (single ticket only)

To make a Jira actionable by an agent picking it up cold, gather the information they'd need. The specific info depends on the issue type:

**Stories** need: a user story (As a [user], I want [X], so that [Y]), acceptance criteria, and the target component (e.g., `api-server`, `control-plane`).

**Bugs** need: steps to reproduce, expected vs actual behavior, and environment info if relevant.

**Spikes** need: the question to answer, expected deliverables, and a time-box. Always include a time-box — spikes without one tend to expand indefinitely.

**Epics** need: a high-level overview of the initiative and the child stories/tasks that make it up. Epics are containers, so they don't need Testing Requirements or Acceptance Criteria of their own — their children carry those.

**All types** benefit from: relevant file paths, related issues/PRs/specs, constraints, and testing requirements.

In batch mode, skip this interactive step — use whatever context the sub-bullets provide and fill in reasonable defaults for the rest. But don't skip content just because it's batch: every non-Epic ticket still needs Testing Requirements and Relevant Paths in its description, even if you have to infer them from the component. Going back to add these later is painful.

### Step 3 — Build Description

Use this template, dropping sections that don't apply:

```markdown
## Overview
[One paragraph: what needs to be done and why]

## User Story
As a [type of user], I want [goal], so that [benefit].

## Acceptance Criteria
- [ ] [Criterion 1]
- [ ] [Criterion 2]

## Technical Context
**Repo**: openshift-online/hypershell
**Component**: [e.g. api-server, control-plane]
**Relevant Paths**:
- `components/[component]/path/to/relevant/file`

## Related Links
- Spec: [link to relevant spec in specs/]
- Related Issues: [HYPERSHELL-XXXX]

## Constraints
- [What NOT to do]

## Testing Requirements
- [ ] Unit tests for [X]

## Bug Details
**Steps to Reproduce**: ...
**Expected**: ...
**Actual**: ...

## Spike Deliverables
- [ ] [Output: e.g. design doc, prototype, findings]
**Time-box**: [e.g. 2 days]
```

**Section guidance by type:**
- **Epics**: Overview only. No Acceptance Criteria, Testing Requirements, or User Story — those belong on the children.
- **Stories**: Overview, User Story, Acceptance Criteria, Technical Context (with Relevant Paths), Testing Requirements.
- **Bugs**: Overview, Bug Details (Steps/Expected/Actual), Technical Context (with Relevant Paths), Testing Requirements.
- **Tasks**: Overview, Acceptance Criteria, Technical Context (with Relevant Paths), Testing Requirements.
- **Spikes**: Overview, Spike Deliverables (must include Time-box), Technical Context (with Relevant Paths).

### Step 4 — Confirm

**Single ticket:**

```
About to create HYPERSHELL Jira:

**Summary**: [extracted summary]
**Type**: [Story/Bug/Task/Spike/Epic]
**Priority**: [inferred or user-selected priority]
**Activity Type**: [inferred activity type]

**Description Preview**:
[First 500 chars of formatted description]

Shall I create this issue? (yes/no/edit)
```

**Batch mode** — show a summary table:

```
About to create N HYPERSHELL tickets:

| # | Type  | Priority | Summary                        | Activity Type                     | Epic          |
|---|-------|----------|--------------------------------|-----------------------------------|---------------|
| 1 | Epic  | Normal   | Feature X                      | Product / Portfolio Work          | —             |
| 2 | Story | Major    | Implement Y                    | Product / Portfolio Work          | Feature X     |
| 3 | Bug   | Major    | Fix Z                          | Quality / Stability / Reliability | —             |

Blocking: #2 blocks #3
Related: #4 related to #5

Create all? (yes/no/edit)
```

### Step 5 — Create

Use `mcp__jira__jira_create_issue` with:

```json
{
  "project_key": "HYPERSHELL",
  "summary": "[summary]",
  "issue_type": "[Story|Bug|Task|Spike|Epic]",
  "description": "[structured description from step 3]",
  "additional_fields": "{\"priority\": {\"name\": \"[priority]\"}, \"labels\": [\"team:hypershell\"], \"customfield_10464\": {\"value\": \"[inferred activity type]\"}}"
}
```

**Batch execution order** (the order matters because later steps depend on earlier ones):
1. Create Epics first — their keys are needed for linking
2. Create remaining tickets (Stories, Bugs, Tasks, Spikes)
3. Link child tickets to their epics via `mcp__jira__jira_link_to_epic`
4. Create blocking relationships via `mcp__jira__jira_create_issue_link` with `link_type: "Blocks"`
5. Create related links via `mcp__jira__jira_create_issue_link` with `link_type: "Related"`

**Important**: the Jira link type for related issues is `"Related"` (not "Relates"). Using the wrong name silently fails.

Parallelize where possible: all epics can be created in parallel, then all non-epics in parallel, then all links in parallel. But each phase must complete before the next begins.

### Step 6 — Report

**Single ticket:**
```
Created: [ISSUE_KEY]
Link: https://redhat.atlassian.net/browse/[ISSUE_KEY]

Summary: [summary]
Type: [issue type]
Priority: [priority]
Activity Type: [inferred activity type]
Agent Cold-Start Ready: Yes
```

**Batch:**
```
Created N tickets:

| Key            | Type  | Priority | Summary                        | Activity Type                     | Epic          |
|----------------|-------|----------|--------------------------------|-----------------------------------|---------------|
| HYPERSHELL-XXXXX  | Epic  | Normal   | Feature X                      | Product / Portfolio Work          | —             |
| HYPERSHELL-XXXXX  | Story | Major    | Implement Y                    | Product / Portfolio Work          | Feature X     |
| HYPERSHELL-XXXXX  | Bug   | Major    | Fix Z                          | Quality / Stability / Reliability | —             |

Links created:
- HYPERSHELL-XXXXX blocks HYPERSHELL-XXXXX
- HYPERSHELL-XXXXX → Epic HYPERSHELL-XXXXX
```

## Examples

### Quick Story
```
/jira-log Add gateway health check endpoint
```
Prompts for acceptance criteria, relevant files, etc.

### Bug Report
```
/jira-log [Bug] Gateway list doesn't refresh after deletion

Steps:
1. Create a gateway
2. Delete the gateway via API
3. Query the gateway list

Expected: Gateway disappears from list
Actual: Gateway remains until reconciler runs

Component: api-server
Files: components/api-server/pkg/api/
```

### Tech Debt Task
```
/jira-log [Task] Add OwnerReferences to reconciler-created Secrets

Control plane creates Secrets without OwnerReferences, leaving orphans.

Component: control-plane
Files: components/control-plane/internal/reconciler/reconciler.go
```

### Spike
```
/jira-log [Spike] Investigate gRPC watch stream connection pooling strategies

Questions: Can we pool connections? What's the memory overhead?
Component: control-plane
Deliverables: Findings doc with benchmarks
Time-box: 3 days
```

### Batch
```
/jira-log
- [Epic] Fleet Onboarding Flow
- Implement fleet registration API
  - Component: api-server
  - Acceptance: fleet can be registered with managed clusters
- [Task] Add fleet onboarding docs
  - Component: docs
- [Bug] ManagedCluster status not updated after registration
  - Steps: register cluster, observe status stays pending
```
Creates 1 epic + 2 stories + 1 bug, links stories to the epic.

### Batch with Blocking
```
/jira-log
- [Epic] RBAC Hardening
- [Task] Audit existing ClusterRole permissions
  - Component: control-plane
  - Files: components/control-plane/manifests/rbac/
  - blocks: Implement namespace-scoped RBAC
- [Story] Implement namespace-scoped RBAC for gateways
  - Component: api-server
  - Acceptance: users can only see gateways in their fleet
- [Spike] Investigate OPA integration for policy enforcement
  - Deliverables: findings doc with recommendation
  - Time-box: 3 days
```
Creates 1 epic + 1 story + 1 task + 1 spike, links all to the epic, creates blocking relationship from audit → RBAC story.

## Quick Reference

| Field | Value |
|-------|-------|
| Project | HYPERSHELL |
| Label | `team:hypershell` |
| Activity Type field | `customfield_10464` |
| Browse URL | `https://redhat.atlassian.net/browse/` |
| Board | 13804 (Hypershell Kanban Board) |
