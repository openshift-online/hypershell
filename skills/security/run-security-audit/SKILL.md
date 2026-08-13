---
name: run-security-audit
description: >-
  Run a comprehensive security audit on this codebase using the
  ai-security-harness methodology. Bootstraps the harness repo, checks for
  optional pre-scanner tools, then executes threat modeling and/or code audit
  against the current repository. Use when asked to "run a security audit",
  "security review", "threat model", or "vulnerability assessment".
argument-hint: "[threat-model|code-audit|full] [--skip-scanners] [--quick]"
user-invocable: true
---

# Run Security Audit

Perform a security audit of this repository using the Red Hat
ai-security-harness methodology. This skill bootstraps the harness as a
standalone tool, then runs its audit skills against the current codebase.

## Step 0 - Parse arguments

Parse `$ARGUMENTS` to determine scope:

| First token | What runs |
|---|---|
| `threat-model` | Threat model only (bootstrap mode) |
| `code-audit` | Secure code audit only (assumes threat model exists or skips coverage diff) |
| `full` | Threat model first, then code audit (recommended for first run) |
| empty | Ask the user which scope they want |

Flags:
- `--skip-scanners` - skip all deterministic pre-scanners even if tools are available
- `--quick` - single-pass audit instead of the default dual-pass

## Step 1 - Bootstrap the harness

Clone or update the ai-security-harness repo. The harness is a standalone
tool with Python scripts, rule packs, and schemas needed by the audit.

```bash
HARNESS_DIR="$(git rev-parse --show-toplevel)/apm_modules/hybrid-platforms-sec/ai-security-harness"

if [ -d "$HARNESS_DIR/.git" ]; then
  git -C "$HARNESS_DIR" fetch origin main --depth 1
  git -C "$HARNESS_DIR" reset --hard origin/main
else
  mkdir -p "$(dirname "$HARNESS_DIR")"
  git clone --depth 1 --branch main \
    https://gitlab.cee.redhat.com/hybrid-platforms-sec/ai-security-harness.git \
    "$HARNESS_DIR"
fi
```

If the clone fails (network, auth), stop and tell the user - the harness
is required.

## Step 2 - Install Python dependencies

```bash
pip install --require-hashes -r "$HARNESS_DIR/requirements.lock" 2>&1
```

If pip fails, try with `--user` flag. If that also fails, warn the user
but continue - Python deps are needed for validation scripts and
deterministic scanners, but the core methodology (manual code review) works
without them.

## Step 3 - Check pre-scanner tools

Check which optional tools are available and report status. These improve
audit quality but are NOT required - the methodology works without them.

```bash
echo "=== Pre-scanner tool availability ==="
command -v opengrep   && echo "opengrep: available"   || echo "opengrep: not found (semantic code patterns)"
command -v gitleaks   && echo "gitleaks: available"    || echo "gitleaks: not found (secret detection)"
command -v osv-scanner && echo "osv-scanner: available" || echo "osv-scanner: not found (dependency CVEs)"
command -v govulncheck && echo "govulncheck: available" || echo "govulncheck: not found (Go symbol-level reachability)"
command -v tokei      && echo "tokei: available"       || echo "tokei: not found (lines of code - will use fallback)"
command -v syft       && echo "syft: available"        || echo "syft: not found (SBOM generation)"
command -v grype      && echo "grype: available"       || echo "grype: not found (known-CVE scan)"
```

Report the availability summary to the user. Missing tools are fine  - 
record each as `"skipped: not installed"` in the report's
`metadata.additional.deterministic_steps`.

## Step 4 - Set up paths and output directory

```bash
TARGET_DIR="$(pwd)"
REPO_NAME="$(basename "$TARGET_DIR")"
OUTPUT_DIR="$TARGET_DIR/security-audit"
mkdir -p "$OUTPUT_DIR"
```

Resolve these once and use throughout:
- `HARNESS_DIR` = `<repo-root>/apm_modules/hybrid-platforms-sec/ai-security-harness`
- `TARGET_DIR` = the current repository root
- `REPO_NAME` = repository basename (e.g. `hypershell`)
- `OUTPUT_DIR` = `$TARGET_DIR/security-audit/`

**Important path substitutions** when following harness SKILL.md
instructions:
- `<harness>` or `ai-security-harness/` → `$HARNESS_DIR`
- `<local-path>` or `<target-dir>` → `$TARGET_DIR`
- `<skill-base>` for threat-model → `$HARNESS_DIR/harnessing/threat-model`
- `<skill-base>` for secure-code-audit → `$HARNESS_DIR/harnessing/secure-code-audit`
- Scripts like `scripts/validate_report.py` → `$HARNESS_DIR/scripts/validate_report.py`
- Rule packs like `opengrep-rules/` → `$HARNESS_DIR/harnessing/secure-code-audit/opengrep-rules/`
- Report output → `$OUTPUT_DIR/`

## Step 5 - Run threat model (if scope includes it)

Read the threat-model skill instructions:

```
$HARNESS_DIR/harnessing/threat-model/SKILL.md
```

Follow the **bootstrap** mode (no application owner present). Key points:

1. Read `$HARNESS_DIR/harnessing/threat-model/bootstrap.md` and follow it.
2. Read `$HARNESS_DIR/harnessing/threat-model/schema.md` immediately before writing output.
3. Write the model to `$OUTPUT_DIR/$REPO_NAME-threat-model.md`.
4. Run the lint gate: `python3 $HARNESS_DIR/scripts/lint_threat_model.py $OUTPUT_DIR/$REPO_NAME-threat-model.md`
5. Fix any errors and re-run until `Result: ALL PASSED`.

**Adapt for standalone use:** Skip campaign-specific steps (findings-tree
placement, spend declaration, doc-variance emission, standing product
context from `hybrid-platforms-inputs/`). These are for the full campaign
infrastructure and don't apply to standalone audits.

## Step 6 - Run secure code audit (if scope includes it)

Read the secure-code-audit skill instructions:

```
$HARNESS_DIR/harnessing/secure-code-audit/SKILL.md
```

Follow the full methodology against `$TARGET_DIR`. Key adaptations:

1. **Input** - the target is the current repo (`$TARGET_DIR`), already checked out. No cloning needed.
2. **Pre-scanners** - run each available tool using the harness scripts, substituting `$HARNESS_DIR` for script paths:
   - `python3 $HARNESS_DIR/scripts/scan_k8s_hardening.py $TARGET_DIR -o $OUTPUT_DIR/$REPO_NAME-k8s-hardening.json`
   - `python3 $HARNESS_DIR/scripts/run_opengrep.py $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-opengrep.json` (if opengrep available)
   - `python3 $HARNESS_DIR/scripts/run_gitleaks.py --repo $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-gitleaks.json` (if gitleaks available)
   - `python3 $HARNESS_DIR/scripts/run_osv_scanner.py --repo $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-osv-scanner.json` (if osv-scanner available)
   - `python3 $HARNESS_DIR/scripts/expand_config_matrix.py $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-config-matrix.json`
   - `python3 $HARNESS_DIR/scripts/enumerate_route_guards.py $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-route-guards.json`
3. **If `--skip-scanners`** - skip all pre-scanners, record each as skipped.
4. **If `--quick`** - single-pass audit. Otherwise dual-pass (default).
5. **Threat model coverage diff** - if a threat model was generated in Step 5, use it.
6. **Report output** - write to `$OUTPUT_DIR/$REPO_NAME-security-audit.json`.

**Adapt for standalone use:** Skip these campaign-specific steps:
- Report placement in `analysis-results/findings/` tree
- Spend declaration via `model_registry.py`
- FP-precedent cache lookup
- Batch execution patterns
- Finding identity fingerprinting (requires contracts submodule)
- Repo-status stamping from `repo-liveness.json`

## Step 7 - Validate and render

```bash
python3 $HARNESS_DIR/scripts/validate_report.py $OUTPUT_DIR/$REPO_NAME-security-audit.json
```

Fix any validation errors and re-run until `Result: ALL PASSED`.

Then render the human-readable Markdown:

```bash
python3 $HARNESS_DIR/scripts/render_report.py $OUTPUT_DIR/$REPO_NAME-security-audit.json \
  -o $OUTPUT_DIR/$REPO_NAME-security-audit.md
```

## Step 8 - Report to user

Summarize:

1. Path to all generated artifacts
2. Top findings by severity (critical and high)
3. Which pre-scanners ran vs. were skipped
4. Any open questions or areas that need manual follow-up
5. Recommend adding `security-audit/` to `.gitignore` if the user wants to keep reports out of version control

## Notes

- The harness methodology is read-only against the target - it does not modify this repository's code.
- Pre-scanner scripts may fail if the harness's Python deps aren't installed. That's OK - fall back to manual review for those sections and note it in the report.
- The `contracts/` submodule in the harness may not be initialized in a shallow clone. If `validate_report.py` fails because the schema is missing, run `git -C $HARNESS_DIR submodule update --init contracts` first.
- Add `security-audit/` to this repo's `.gitignore` to keep generated reports out of version control.
