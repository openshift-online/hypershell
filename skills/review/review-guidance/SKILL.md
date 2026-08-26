---
name: review-guidance
description: >
  HyperShell-specific review standards for PRs. Loads project conventions,
  security requirements, and component-specific checklists. Use when reviewing
  PRs in this repository.
---

# HyperShell Review Guidance

Apply these standards when reviewing PRs in this repository.

## Mandatory Context Files

Before analyzing any PR, load:

1. `CLAUDE.md`
2. `specs/standards/security/security.spec.md`
3. `specs/standards/control-plane/conventions.spec.md`

## Review Checklists

### Go Backend (API Server)

- [ ] No `panic()` in production code -- return `fmt.Errorf` with context
- [ ] Errors wrapped: `fmt.Errorf("context: %w", err)`
- [ ] `errors.IsNotFound` handled for 404 scenarios
- [ ] No secrets in logs or error messages
- [ ] Input validated (K8s DNS labels, URL parsing)
- [ ] Log injection prevented
- [ ] OpenAPI client not manually edited (`make generate` only)

### Go Control Plane

- [ ] SecurityContext on all pod specs
- [ ] Resource limits/requests on containers
- [ ] Status updated on error paths
- [ ] No `panic()` in non-test code
- [ ] Proper context propagation (no `context.TODO()`)
- [ ] `gofmt -w .` applied
- [ ] `go vet ./...` passes
- [ ] Reconcile pattern used (not create-or-skip)

### General

- [ ] No `panic()` in production Go code
- [ ] PostgreSQL for persistent storage
- [ ] Image references consistent across manifests
- [ ] Conventional commit message

### Test Diff Scrutiny

Review the diff hunks inside *existing* test files, not just newly added tests.
When a test's assertions change (not just its surrounding scaffolding), the test
used to prove one thing and now proves the opposite - that's a removed guarantee,
not just an updated one.

- [ ] Every modified assertion in a pre-existing test is individually justified,
      not just "updated to make it pass"
- [ ] Watch for the specific pattern: an existing test asserted a blank/optional/
      zero-value field was accepted (`BeEmpty()`, `omitempty`, no error), and the
      diff flips it to required/rejected (`Equal(nonEmptyValue)`, expects an error).
      This is a contract change wearing a test-fixup disguise.
- [ ] If a field, config value, or dependency moves from optional -> required,
      confirm there's a fallback/migration path for records or environments that
      predate the change - not just a new validation error. Absence of a fallback
      is a Blocker if the field could already hold the old (now-invalid) value in
      a running environment; otherwise Major.
- [ ] Prefer additive test changes over in-place rewrites: the old behavior should
      either still be covered under a renamed test (if intentionally deprecated)
      or coexist with a new test for the new behavior - not be silently replaced.
- [ ] Multiple unrelated tests changing the same input value in the same PR
      (e.g., several tests switching a field from `""` to a fixed literal purely
      to keep passing) is a signal the PR tightened a shared precondition without
      calling it out as a breaking change in the description.

## Severity Classification

- **Blocker** -- Must fix. Security vulnerabilities, data loss, secret leaks
- **Critical** -- Should fix. Missing error handling, `panic()` in handlers
- **Major** -- Important. Architecture violations, missing tests
- **Minor** -- Nice-to-have. Style, docs gaps
