# /run-security-audit

Run a comprehensive security audit on this codebase using the
[ai-security-harness](https://gitlab.cee.redhat.com/hybrid-platforms-sec/ai-security-harness)
methodology. The skill bootstraps the harness repo on demand - no manual
setup beyond the prerequisites below.

## Prerequisites

### Environment variables

The harness is hosted on internal GitLab. These must be set before running
(e.g. in `~/.zshrc`):

```bash
export GITLAB_HOST=gitlab.cee.redhat.com
export GITLAB_APM_PAT="<your-gitlab-personal-access-token>"
```

The PAT needs `read_repository` scope on the
`hybrid-platforms-sec/ai-security-harness` project.

### Model recommendation

The harness audit skills are rated **mythos-class** - they require a strong
reasoning model to follow the multi-framework methodology accurately.
Claude Opus or equivalent is recommended. Lighter models will produce
lower-quality findings and may skip methodology steps.

### Optional pre-scanner tools

These are not required but significantly improve audit quality when
available on `PATH`:

| Tool | What it adds |
|---|---|
| `opengrep` | Semantic code pattern matching with the harness rule pack |
| `gitleaks` | Secret and credential detection including git history |
| `osv-scanner` | Multi-ecosystem dependency CVE scanning |
| `govulncheck` | Go symbol-level vulnerability reachability analysis |
| `tokei` (or `cloc`/`scc`) | Accurate lines-of-code measurement |
| `syft` + `grype` | SBOM generation and known-CVE dependency scanning |

The skill reports which tools are available at startup and records skipped
scanners in the report metadata.

## Usage

```
/run-security-audit full              # Threat model + code audit (recommended first run)
/run-security-audit threat-model      # Threat model only (bootstrap mode)
/run-security-audit code-audit        # Code audit only
/run-security-audit full --quick      # Single-pass audit (faster, lower recall)
/run-security-audit full --skip-scanners  # Skip deterministic pre-scanners
```

## What it does

1. **Bootstraps the harness** - clones (or updates) `ai-security-harness`
   to `apm_modules/hybrid-platforms-sec/ai-security-harness/` (already
   gitignored) and installs its Python dependencies.

2. **Checks pre-scanner tools** - reports availability of optional
   scanners and records each as ran/skipped in the report.

3. **Runs the threat model** (`full` or `threat-model` mode) - reads the
   codebase and produces a structured threat model covering attack
   surfaces, trust boundaries, and threat rankings. Output:
   `security-audit/<repo>-threat-model.md`.

4. **Runs the secure code audit** (`full` or `code-audit` mode) - applies
   the multi-framework methodology (OWASP ASVS v5.0, OWASP Kubernetes
   Top 10, CIS Kubernetes Benchmark, DISA STIG, SLSA, OpenSSF Scorecard,
   PEACH tenant isolation) with available pre-scanners. Default is
   dual-pass for higher recall. Output:
   `security-audit/<repo>-security-audit.json` and `.md`.

5. **Validates and renders** - runs the harness validation scripts on the
   JSON report and renders a human-readable Markdown companion.

Reports are written to `security-audit/` in the repo root.

## Further reading

The upstream harness repo has extensive documentation on the audit
methodology, report schema, finding lifecycle, and continuous scanning:

- [Getting started](https://gitlab.cee.redhat.com/hybrid-platforms-sec/ai-security-harness/-/blob/main/docs/getting-started.md)
- [Skill reference](https://gitlab.cee.redhat.com/hybrid-platforms-sec/ai-security-harness/-/blob/main/docs/skills.md)
- [Report structure](https://gitlab.cee.redhat.com/hybrid-platforms-sec/ai-security-harness/-/blob/main/docs/report-structure.md)
- [AGENTS.md](https://gitlab.cee.redhat.com/hybrid-platforms-sec/ai-security-harness/-/blob/main/AGENTS.md) (full methodology and constraints)
