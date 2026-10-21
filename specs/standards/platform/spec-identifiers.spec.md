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
   an ID that exists. A token counts as a reference when its `MODULE-GROUP`
   prefix is registered in `specs/ids.md`.
6. An old ID listed in `specs/id-rename-map.md` is still cited anywhere
   other than the rename map and this spec. The error names the new ID.
7. A test citation marker (Requirement STD-IDS-07) is malformed, or names
   an ID that does not exist.

`scripts/check_spec_ids.py` implements this check, and `make check` runs it.

#### Scenario: STD-IDS-06.01 -- Duplicate ID fails the check

- GIVEN two requirements in `specs/` both carry the ID `OSG-TLS-03`
- WHEN the ID check runs
- THEN the check SHALL fail and name both locations

#### Scenario: STD-IDS-06.02 -- An old ID in code fails the check

- GIVEN a Go comment cites `GAM-09`, which `specs/id-rename-map.md` renamed
- WHEN the ID check runs
- THEN the check SHALL fail and name the new ID that replaces `GAM-09`

### Requirement: STD-IDS-07 -- Test Citation Markers

A test that verifies a scenario SHALL carry a marker comment on the line
directly above the test function, subtest, or test case:

```go
// Verifies: OSG-TLS-03.02
func TestCertificateRenewal(t *testing.T) {
```

```ts
// Verifies: OSG-TLS-03.02, OSG-TLS-03.03
it('renews before expiry', () => {
```

Shell and Python tests use `# Verifies: ...`.

1. The marker SHALL be the only content on its line: the comment token,
   `Verifies:`, then one or more IDs separated by `, `.
2. A test SHALL cite the scenario ID when it proves that scenario. It MAY
   cite a requirement ID when it proves the requirement as a whole and no
   single scenario fits.
3. A marker SHALL be added only when the test asserts the scenario's THEN
   clause. A test that only touches the same feature SHALL NOT be marked.
4. A test MAY cite several IDs, and an ID MAY be cited by several tests.
5. Free-text mentions of an ID in other comments are allowed, but they do
   not count as test coverage.

The marker reuses the word of the `Verifies:` commit trailer
(STD-IDS-05). The trailer records non-`Test` evidence in git history; the
marker records `Test` evidence in the test source.

#### Scenario: STD-IDS-07.01 -- A marked test counts as coverage

- GIVEN `// Verifies: OSG-TLS-03.02` sits directly above `func TestCertificateRenewal`
- WHEN the traceability report is generated
- THEN scenario `OSG-TLS-03.02` SHALL be reported as covered by that test
- AND requirement `OSG-TLS-03` SHALL be reported as covered

#### Scenario: STD-IDS-07.02 -- A marker naming a missing ID fails the check

- GIVEN a marker `// Verifies: OSG-TLS-03.09` and no scenario `OSG-TLS-03.09` exists
- WHEN the ID check runs
- THEN the check SHALL fail and name the marker's location

### Requirement: STD-IDS-08 -- Traceability Report

`make traceability` SHALL print a report built from the spec headings and
the test markers. The report is generated on demand and SHALL NOT be
committed. It SHALL show:

1. Totals for requirements and scenarios: how many exist, how many are
   covered by a test, and how many are not.
2. A per-spec-file summary.
3. For each requirement and scenario, the tests that cite it, as
   `path:line` with the test name.
4. A list of scenarios no test cites.

A scenario is covered when a marker cites it. A requirement is covered
when a marker cites it or any of its scenarios. A requirement whose
verification method (STD-IDS-05) is `Inspection`, `Analysis`, or
`Demonstration` SHALL be counted separately and not listed as a gap.

A requirement or scenario that no test has ever covered SHALL NOT fail
`make check`. The report shows these gaps; closing them is planned work.
Coverage that existed and is lost is handled by STD-IDS-09.

#### Scenario: STD-IDS-08.01 -- An inspection-verified requirement is not a gap

- GIVEN requirement `OSG-TLS-05` is annotated `**Verification:** Inspection -- reviewed during cert-manager upgrade`
- AND no test cites `OSG-TLS-05` or its scenarios
- WHEN the traceability report is generated
- THEN `OSG-TLS-05` SHALL be counted as verified by inspection
- AND it SHALL NOT appear in the uncovered list

### Requirement: STD-IDS-09 -- Coverage Ratchet

Test coverage SHALL NOT drop silently.

1. `specs/test-coverage-baseline.txt` SHALL list every requirement and
   scenario ID that a test covered when the file was last updated, one ID
   per line. "Covered" has the same meaning as in STD-IDS-08.
2. `make check` SHALL fail when an ID in the baseline is no longer covered
   by any test. The error SHALL name the ID.
3. `make check` SHALL fail when the baseline lists an ID that is not a
   current requirement or scenario, so the baseline cannot go stale.
4. Removing an ID from the baseline is the deliberate way to accept lost
   coverage. The pull request that removes it SHALL say why.
5. Newly covered IDs that are not yet in the baseline SHALL NOT fail
   `make check`. The check SHALL print a notice, and
   `make traceability-baseline` SHALL add them.

#### Scenario: STD-IDS-09.01 -- Deleting the last covering test fails the check

- GIVEN `OSG-TLS-03.02` is listed in the baseline
- AND the only test with `Verifies: OSG-TLS-03.02` is deleted
- WHEN `make check` runs
- THEN the check SHALL fail and name `OSG-TLS-03.02`

#### Scenario: STD-IDS-09.02 -- New coverage only prints a notice

- GIVEN a new test adds `Verifies: OSG-TLS-03.03`
- AND `OSG-TLS-03.03` is not in the baseline
- WHEN `make check` runs
- THEN the check SHALL pass
- AND it SHALL print a notice to run `make traceability-baseline`

#### Scenario: STD-IDS-09.03 -- A baseline entry for a missing ID fails the check

- GIVEN the baseline lists `OSG-TLS-03.09`
- AND no requirement or scenario `OSG-TLS-03.09` exists
- WHEN `make check` runs
- THEN the check SHALL fail and ask for the line to be removed

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
5. **Proposal A (test traceability)**: adopted. Tests cite the scenario
   ID they verify with a `Verifies:` marker (STD-IDS-07), and
   `make traceability` reports coverage (STD-IDS-08). A committed baseline
   keeps covered IDs from losing their tests silently (STD-IDS-09).
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
`standards/sdlc/jira-github-sync.spec.md`). The follow-up change renamed
them to `#### Example:` so they no longer look like scenarios.

Code and test files that cited a pre-migration ID were rewritten to the
new IDs in a follow-up change, which also added the STD-IDS-06 check to
`make check`. That change also renamed a few IDs added after the migration
in the old style; they are listed under "Later Renames" in
`specs/id-rename-map.md`.
