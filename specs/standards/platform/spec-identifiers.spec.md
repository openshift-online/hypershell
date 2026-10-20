# Spec Identifiers Specification

**ID prefix:** STD-IDS

## Purpose

Every requirement and scenario in `specs/` needs a stable, unique identifier
so tests, commits, and tooling can cite exact behavior instead of a heading
title that can be reworded at any time. This spec defines the identifier
syntax, the canonical headings that carry it, the registry that assigns
module and group codes, and the stability rules that keep an ID meaningful
for the life of the requirement it names.

## Requirements

### Requirement: STD-IDS-01 -- Identifier Syntax

A requirement identifier SHALL have the form `MODULE-GROUP-COUNTER`:

- **MODULE**: 3 or 4 uppercase letters or digits, starting with a letter.
  The coarse domain the spec belongs to (for example `OSG`, `E2E`). Several
  files MAY share one module.
- **GROUP**: 3 or 4 uppercase letters or digits, starting with a letter. The
  sub-area inside the module. A file SHALL use exactly one group unless the
  file already hosts clearly distinct, independently-evolving sub-areas, in
  which case it MAY host more than one group (see `specs/ids.md` for the
  files that do).
- **COUNTER**: 2 digits, left-padded with `0` (`01`..`99`). A group SHALL use
  3 digits only once it needs a 100th entry; existing 2-digit IDs in that
  group are not rewritten when this happens.

A scenario identifier SHALL be its parent requirement's identifier plus a
2-digit scenario counter, separated by a dot: `MODULE-GROUP-COUNTER.NN`. The
scenario counter restarts at `01` for each requirement, in document order.

#### Scenario: STD-IDS-01.01 -- A well-formed requirement ID

- GIVEN a spec file whose registered prefix is `OSG-TLS`
- WHEN its third requirement is assigned an ID
- THEN the ID SHALL read `OSG-TLS-03`

#### Scenario: STD-IDS-01.02 -- A well-formed scenario ID

- GIVEN a requirement with ID `OSG-TLS-03`
- WHEN its second scenario is assigned an ID
- THEN the ID SHALL read `OSG-TLS-03.02`

### Requirement: STD-IDS-02 -- Canonical Headings

A requirement SHALL be declared with the heading:

```markdown
### Requirement: <ID> -- <Name>
```

A scenario SHALL be declared with the heading:

```markdown
#### Scenario: <ID> -- <Name>
```

A requirement that exists only to document a retained, deprecated, or
historical behavior MAY use `### Legacy compatibility: <ID> -- <Name>`
instead of `### Requirement:`. It still carries an ID and still numbers its
scenarios the same way; the different heading word signals to readers that
the behavior is being preserved, not newly specified.

No other heading form is valid for a new or edited requirement or scenario.

#### Scenario: STD-IDS-02.01 -- Heading rejected without the separator

- GIVEN a heading `### Requirement: OSG-TLS-03 Certificate Renewal` (no `--`)
- WHEN the ID-format check runs
- THEN the heading SHALL be rejected as malformed

### Requirement: STD-IDS-03 -- The ID Registry

`specs/ids.md` SHALL list every registered `MODULE-GROUP` prefix, the one
file (or, for the files noted above, the small set of files) that owns it,
and whether the file is exempt from numbering.

A new module or group SHALL be registered in `specs/ids.md` before a
requirement uses it. Two files SHALL NOT register the same `MODULE-GROUP`
prefix.

A spec file with no `### Requirement` heading (a narrative or reference
document, not a set of testable behavior contracts) SHALL be listed in the
registry as exempt, with a one-line reason, instead of being forced into the
Requirement/Scenario format.

#### Scenario: STD-IDS-03.01 -- Registering a new group

- GIVEN a new spec file for a sub-area of the `OSG` module that has no group yet
- WHEN the spec is drafted
- THEN `specs/ids.md` SHALL gain a row for the new `OSG-<GROUP>` prefix before any `### Requirement: OSG-<GROUP>-01` heading is written

### Requirement: STD-IDS-04 -- ID Stability

1. An ID SHALL NOT change once merged, even when the requirement is reworded.
2. An ID SHALL NOT be reused. Deleting a requirement retires its ID. The
   spec SHALL list retired IDs in a `## Retired IDs` section, with a
   "replaced by" note when relevant.
3. Moving a requirement to another file SHALL keep its ID. The prefix
   records where the requirement started, not where it currently lives.
4. Reordering requirements within a file SHALL NOT renumber them. The
   counter is an identifier, not a position: a requirement inserted between
   `OSG-TLS-03` and `OSG-TLS-04` SHALL take the next free counter in the
   group (for example `OSG-TLS-19`), not `OSG-TLS-03A` or a shift of every
   later counter.
5. Cross-references between specs SHALL cite the ID, not a title or section
   number.

#### Scenario: STD-IDS-04.01 -- Inserting a requirement does not renumber siblings

- GIVEN a group with requirements `OSG-TLS-01` through `OSG-TLS-18`
- WHEN a new requirement is inserted between `OSG-TLS-03` and `OSG-TLS-04` in the document
- THEN the new requirement SHALL be assigned `OSG-TLS-19`
- AND `OSG-TLS-04` through `OSG-TLS-18` SHALL keep their existing IDs

#### Scenario: STD-IDS-04.02 -- A retired ID is never reassigned

- GIVEN `OSG-TLS-07` was retired when its requirement was deleted
- WHEN a new, unrelated requirement is added to the same group
- THEN it SHALL receive the next free counter after the highest counter ever used in that group, not `07`

### Requirement: STD-IDS-05 -- Verification Method

Each requirement SHALL declare how it is verified: `Test` (an automated test
proves it), `Inspection` (review of code or config proves it), `Analysis`
(reasoning or measurement proves it), or `Demonstration` (a manual run
proves it). `Test` is the default and needs no annotation; any other method
SHALL carry a one-line reason in the spec so exceptions stay rare and
visible.

Evidence for a non-`Test` method lives in git, not in a tracked file. The
commit that performs the verification SHALL carry a trailer of the form:

```
Verifies: <requirement-or-scenario-ID> <method>
```

One trailer per ID. The trailer SHALL be in the commit message (this
repository squash-merges, so PR-body text is lost on merge). The latest
evidence for an ID is `git log --grep 'Verifies: <ID>'`.

#### Scenario: STD-IDS-05.01 -- Recording an inspection-based verification

- GIVEN a requirement `OSG-TLS-05` is annotated `**Verification:** Inspection -- reviewed during cert-manager upgrade`
- WHEN the reviewing commit is created
- THEN its message SHALL include the trailer `Verifies: OSG-TLS-05 inspection`

### Requirement: STD-IDS-06 -- Enforcement

A `make check` script SHALL fail the build when:

1. A requirement or scenario heading has no ID, or a malformed one.
2. An ID is duplicated anywhere in `specs/`.
3. A heading uses a `MODULE-GROUP` prefix not present in `specs/ids.md`, or
   a prefix registered to a different file.
4. A retired ID (listed under a spec's `## Retired IDs`) is reused.
5. A reference to an ID in `specs/`, `skills/`, or code does not resolve to
   an ID that exists.

This check is not yet wired into `make check`. Its implementation is tracked
as follow-up build work under the same work item as this spec,
[issue #636](https://github.com/openshift-online/hypershell/issues/636).

#### Scenario: STD-IDS-06.01 -- Duplicate ID fails the check

- GIVEN two requirements in `specs/` both carry the ID `OSG-TLS-03`
- WHEN the ID check runs
- THEN the check SHALL fail and name both locations

## Retired IDs

None yet. The 411 ad-hoc identifiers that predated this scheme (for example
`OP-DASH-08`, `CP-OBS-07`, `GAM-01`, `WEB-UI-03A`) were renamed, not
retired: each maps to exactly one new ID in `specs/id-rename-map.md`, and
every reference to the old form in `specs/` was rewritten to the new one in
the same change. A rename is not a retirement because the requirement the ID
names did not go away.

## Decisions Recorded (from HYPERSHELL-419 / issue #636)

1. **A spec is each requirement, not each file.** A file gets one
   `MODULE-GROUP` prefix by default. A file MAY host more than one group when
   it already spans clearly distinct, independently-evolving sub-areas;
   `specs/ids.md` marks the three files that do (`content-localization`,
   `trust-performance`, `web-console/architecture`). Splitting those into
   one file per group was judged higher-churn than the problem warrants.
2. **Scenario form is `REQ-ID.NN`**, not a separate counter in the
   `MODULE-GROUP-NN` sequence. The parent requirement stays visible in the
   ID, and a test can cite the exact scenario it verifies.
3. **Codes start with a letter.** Digits are allowed after that (`E2E`,
   `A11Y`, `L10N`), so a code never looks like a bare counter.
4. **Files with no `### Requirement` heading are exempted**, not converted.
   Eight files in the current corpus are narrative or reference documents
   (conventions, cross-cutting constraints, naming rules, image pipelines)
   with no Given/When/Then scenarios; forcing them into the Requirement
   format would manufacture structure the content doesn't have. They are
   listed in `specs/ids.md` with a reserved prefix for future use.
5. **Proposal A (test traceability)**: adopted as a direction, not built
   here. Tests SHOULD cite the scenario ID they verify once this spec lands;
   the traceability-report script is follow-up build work.
6. **Proposal B (verification method per requirement)**: adopted, captured
   in Requirement STD-IDS-05 above.
7. **Proposal C (revision suffixes for stale-test detection)**: not
   adopted. The added friction is not justified until spec churn is shown to
   regularly leave tests out of date.
8. **Proposal D (PR descriptions list `Implements:` IDs)**: not adopted as a
   required field. The `Verifies:` commit trailer (STD-IDS-05) already makes
   "which commit delivered this requirement" a `git log --grep` search;
   requiring a second, PR-body-only field would duplicate that without
   surviving the squash-merge.

## Migration Note

This change renumbers 918 requirements and assigns IDs to 1747 scenarios
across 90 spec files. Every reference to a changed ID elsewhere in `specs/`
was rewritten in the same change (698 rewritten cross-references). Five
`#### Scenario:` headings were left unlabeled because they illustrate
behavior in a narrative section outside any file's `## Requirements`
section (`platform/oidc-integration.spec.md` and
`standards/sdlc/jira-github-sync.spec.md`); a follow-up should either
promote them into a formal requirement or demote their heading level so they
stop looking like scenarios.

Code and test files that cite a pre-migration ID (113 files, per the
originating issue) are unchanged by this PR. Rewriting them, and wiring the
enforcement script from Requirement STD-IDS-06 into `make check`, is
follow-up build work tracked under the same work item.
