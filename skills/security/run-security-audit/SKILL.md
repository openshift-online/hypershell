---
name: run-security-audit
description: >-
  Run a comprehensive security audit on this codebase using the
  ai-security-harness methodology. Checks required and optional tool
  prerequisites, bootstraps the harness repo, then executes threat modeling
  and/or code audit against the current repository. Use when asked to "run a
  security audit", "security review", "threat model", or "vulnerability
  assessment".
argument-hint: "[threat-model|code-audit|full] [--skip-scanners] [--quick]"
user-invocable: true
---

# Run Security Audit

Perform a security audit of this repository using the Red Hat
ai-security-harness methodology. This skill bootstraps the harness as a
standalone tool, then runs its audit skills against the current codebase.

## Step 0 - Check prerequisites

Run this immediately when the skill is invoked, before any other step.
Show the checklist to the user first. **Do not proceed to Step 1** if any
required tool is marked ❌ - stop the skill, tell the user which required
dependencies are missing, and ask them to install those tools and re-run.

```bash
_prereq_status() {
  local label="$1"
  local required="$2"
  shift 2
  for cmd in "$@"; do
    if command -v "$cmd" &>/dev/null; then
      echo "✅ $label"
      return 0
    fi
  done
  if [ "$required" = "required" ]; then
    echo "❌ $label"
    return 1
  else
    echo "⚠️ $label"
    return 0
  fi
}

missing_required=0
echo "pre-requisites ---"
_prereq_status git required git || missing_required=1
_prereq_status python3 required python3 || missing_required=1
_prereq_status pip required pip3 pip uv || missing_required=1
_prereq_status uv optional uv
_prereq_status opengrep optional opengrep
_prereq_status gitleaks optional gitleaks
_prereq_status osv-scanner optional osv-scanner
_prereq_status govulncheck optional govulncheck
_prereq_status tokei optional tokei cloc scc
_prereq_status syft optional syft
_prereq_status grype optional grype

if [ "$missing_required" -ne 0 ]; then
  echo ""
  echo "STOP: install required dependencies marked ❌, then re-run /run-security-audit."
  exit 1
fi
```

If the script exits non-zero, stop immediately. Reply to the user with:

1. The prerequisite checklist (including ❌ lines)
2. A short install hint for each missing required tool:
   - `git` - your platform package manager (e.g. `apt install git`, `dnf install git`)
   - `python3` - your platform package manager (e.g. `apt install python3`)
   - `pip` - bundled with Python 3; try `python3 -m ensurepip --upgrade`, or install `uv` as an alternative
3. Ask the user to install the missing tools and re-run the skill

Legend:
- ✅ installed
- ❌ required but not installed - **stop here; do not continue**
- ⚠️ optional but not installed - audit proceeds with reduced scanner coverage

Required tools: `git`, `python3`, and `pip`/`pip3` or `uv`. Optional tools
improve scanner coverage during the harness code audit.

Only after all required tools show ✅, record which optional pre-scanners
are missing and continue to Step 1. Reuse that list in Step 6 when deciding
which harness scanner scripts to run.

## Step 1 - Parse arguments

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

## Step 2 - Bootstrap the harness

Clone or update the ai-security-harness repo. The harness is a standalone
tool with Python scripts, rule packs, and schemas needed by the audit.

Authenticate with the `GITLAB_HOST` and `GITLAB_APM_PAT` variables documented
in `README.md` (PAT needs `read_repository` on the harness project). Git does
not read arbitrary `GITLAB_*` variables, so wire them through an ephemeral
`GIT_ASKPASS` helper for clone/fetch instead of embedding the token in the URL.

```bash
HARNESS_DIR="$(git rev-parse --show-toplevel)/apm_modules/hybrid-platforms-sec/ai-security-harness"
GITLAB_HOST="${GITLAB_HOST:-gitlab.cee.redhat.com}"
HARNESS_URL="https://${GITLAB_HOST}/hybrid-platforms-sec/ai-security-harness.git"
# Pin for reproducible audits. Override when intentionally upgrading the harness.
HARNESS_REF="${AI_SECURITY_HARNESS_REF:-dac1533bde98179367fa2ee3cd2486bb81292140}"

if [ -z "${GITLAB_APM_PAT:-}" ]; then
  echo "STOP: GITLAB_APM_PAT is not set." >&2
  echo "  export GITLAB_HOST=${GITLAB_HOST}" >&2
  echo "  export GITLAB_APM_PAT=<PAT with read_repository on the harness project>" >&2
  exit 1
fi

GIT_ASKPASS_SCRIPT="$(mktemp)"
cat > "$GIT_ASKPASS_SCRIPT" <<'ASKPASS'
#!/bin/sh
case "$1" in
  *Username*) echo oauth2 ;;
  *Password*) printf '%s' "$GITLAB_APM_PAT" ;;
esac
ASKPASS
chmod +x "$GIT_ASKPASS_SCRIPT"
export GIT_ASKPASS="$GIT_ASKPASS_SCRIPT"
export GIT_TERMINAL_PROMPT=0

_harness_git_cleanup() {
  rm -f "$GIT_ASKPASS_SCRIPT"
  unset GIT_ASKPASS GIT_TERMINAL_PROMPT
}
trap _harness_git_cleanup EXIT

if [ -d "$HARNESS_DIR/.git" ]; then
  git -C "$HARNESS_DIR" fetch origin "$HARNESS_REF" --depth 1
  git -C "$HARNESS_DIR" reset --hard FETCH_HEAD
else
  mkdir -p "$(dirname "$HARNESS_DIR")"
  git clone "$HARNESS_URL" "$HARNESS_DIR"
  git -C "$HARNESS_DIR" fetch origin "$HARNESS_REF" --depth 1
  git -C "$HARNESS_DIR" reset --hard FETCH_HEAD
fi
```

If the clone fails (network, auth), stop and tell the user - the harness
is required. Confirm `GITLAB_APM_PAT` has `read_repository` on
`hybrid-platforms-sec/ai-security-harness` and that `GITLAB_HOST` matches the
GitLab instance hostname.

## Step 3 - Install Python dependencies

```bash
pip install --require-hashes -r "$HARNESS_DIR/requirements.lock" 2>&1
```

If pip fails, try with `--user` flag. If that also fails, warn the user
but continue - Python deps are needed for validation scripts and
deterministic scanners, but the core methodology (manual code review) works
without them.

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
2. **Pre-scanners** - use the Step 0 prerequisite results. Run each
   available tool using the harness scripts, substituting `$HARNESS_DIR` for
   script paths:
   - `python3 $HARNESS_DIR/scripts/scan_k8s_hardening.py $TARGET_DIR -o $OUTPUT_DIR/$REPO_NAME-k8s-hardening.json`
   - `python3 $HARNESS_DIR/scripts/run_opengrep.py $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-opengrep.json` (if opengrep available)
   - `python3 $HARNESS_DIR/scripts/run_gitleaks.py --repo $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-gitleaks.json` (if gitleaks available)
   - `python3 $HARNESS_DIR/scripts/run_osv_scanner.py --repo $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-osv-scanner.json` (if osv-scanner available)
   - `python3 $HARNESS_DIR/scripts/expand_config_matrix.py $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-config-matrix.json`
   - `python3 $HARNESS_DIR/scripts/enumerate_route_guards.py $TARGET_DIR --out $OUTPUT_DIR/$REPO_NAME-route-guards.json`
3. **If `--skip-scanners`** - skip all pre-scanners, record each as skipped.
4. **If `--quick`** - single-pass audit. Otherwise dual-pass (default).
5. **Threat model coverage diff** - if a threat model was generated in Step 5,
   use it. For each pre-scanner not installed in Step 0, record
   `"skipped: not installed"` in `metadata.additional.deterministic_steps`.
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

## Notes

- The harness methodology is read-only against the target - it does not modify this repository's code.
- Pre-scanner scripts may fail if the harness's Python deps aren't installed. That's OK - fall back to manual review for those sections and note it in the report.
- The `contracts/` submodule in the harness may not be initialized in a shallow clone. If `validate_report.py` fails because the schema is missing, run `git -C $HARNESS_DIR submodule update --init contracts` first (reuse the Step 2 `GIT_ASKPASS` setup if submodule fetch needs GitLab auth).
- Generated reports under `security-audit/` are already ignored by this repo's `.gitignore`.
