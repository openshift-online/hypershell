# ADR-0027: Fullsend Lifecycle Label Hooks design decisions

Rationale relocated from [`specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md`](../../specs/standards/sdlc/fullsend-lifecycle-label-hooks.spec.md) (SDLC-HOOK). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Label transitions live in shared scripts, not agent prose | A deterministic script applies the same labels in the same order every time. Prose leaves the model responsible for remembering every step, so a finished task can still leave a transition half-applied. |
| Agents return a validated structured result | A fixed-enum, versioned result keeps arbitrary label names, repository targets, and shell commands out of the model output and gives the post-script a trusted contract to act on. |
| Validation is read-only and precedes any mutation | Deciding whether output is valid must never change workflow state, so an invalid or unvalidated result cannot advance the funnel or leave labels half-changed. |
| Evidence is checked against GitHub before applying | A schema-valid result is not proof of completed work. Rechecking the pull request and head commit rejects stale reviews and prevents advancing on work that changed. |
| The post-script re-reads and verifies the final state | Several GitHub API calls are not a transaction. Re-reading after applying, and reporting success only after verification, keeps a partial failure visible instead of falsely successful. |
| Only the named label changes are applied, never a snapshot replace | Replacing the full label set from an old snapshot would erase a human block or authorization change made during the run. Adding and removing only the named labels preserves human decisions. |
| Protected label categories are never mutated by a transition | Scope, authorization, execution-record, and human-block labels carry decisions outside one phase. Treating them as protected stops a phase transition from undoing a human gate or the canonical link. |
| Retry counts come from the issue timeline | The number of prior rejections is authoritative evidence on GitHub, not a value the agent can assert, so the block decision cannot be gamed by the result. |
| Behavior lives under `.fullsend/`, not the local dispatcher | Keeping scripts, schemas, and configuration under `.fullsend/` lets the same harnesses run identically in local and cloud Fullsend execution, with no dependency on a workstation or a local database. |
| One writer for phase labels | Routing intake and reconciliation label writes through the same contract, or removing that responsibility, prevents competing writers from leaving conflicting phase labels. |

## Additional rationale

- Transitions are moved out of prose instructions because a model applying labels from prose must remember every required step and its order: an agent can finish its task and leave a label update incomplete, a sequence of separate API calls can fail partway through a transition, and conflicting labels can prevent a trigger from matching or leave the next phase unclear.
