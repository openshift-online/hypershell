#!/usr/bin/env bash
# lifecycle_schema_test.sh - tests the native read-only validation layer
# (SDLC-HOOK-04, SDLC-HOOK-06, SDLC-HOOK-13.01) by running the generic
# validate-output-schema.sh against the per-phase result schemas. Validation is
# read-only and never mutates a label. Auto-discovered by
# scripts/run-shell-unit-tests.sh. Skips cleanly if python3 jsonschema is absent.

set -uo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS="${_here}/.."
SCHEMAS="${SCRIPTS}/../schemas"
VALIDATE="${SCRIPTS}/validate-output-schema.sh"

if ! python3 -c "import jsonschema" 2>/dev/null; then
  echo "SKIP: python3 jsonschema not installed"
  exit 0
fi

PASS=0
FAIL=0
_ok() { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
_no() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; [[ -n "${2:-}" ]] && printf '       %s\n' "$2"; }

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

# _expect <pass|fail> <schema-file> <description> <json>
_expect() {
  local want="$1" schema="$2" desc="$3" json="$4"
  local dir="${WORK}/case.${RANDOM}"
  mkdir -p "${dir}/output"
  printf '%s' "${json}" > "${dir}/output/agent-result.json"
  local rc=0
  ( cd "${dir}" && FULLSEND_OUTPUT_SCHEMA="${SCHEMAS}/${schema}" bash "${VALIDATE}" >/dev/null 2>&1 ) || rc=$?
  if [[ "${want}" == pass && "${rc}" == 0 ]]; then _ok "${desc}"
  elif [[ "${want}" == fail && "${rc}" != 0 ]]; then _ok "${desc}"
  else _no "${desc}" "want ${want}, rc=${rc}"; fi
}

# Valid results for each phase.
_expect pass triage-result.schema.json "valid triage result" \
  '{"schema_version":"1","phase":"triage","outcome":"workable","rationale":"clear"}'
_expect pass spec-review-result.schema.json "valid spec-review result" \
  '{"schema_version":"1","phase":"spec-review","outcome":"approved","rationale":"ok","pr_number":5,"head_sha":"abc1234","comment":"good"}'
_expect pass code-review-result.schema.json "valid code-review result" \
  '{"schema_version":"1","phase":"code-review","outcome":"rejected","rationale":"ci red","pr_number":5,"head_sha":"abc1234","comment":"fix ci"}'
_expect pass code-implementation-result.schema.json "valid code-implementation result" \
  '{"schema_version":"1","phase":"code-implementation","outcome":"completed","rationale":"done","pr_number":5,"head_sha":"abc1234"}'

# Unsupported outcome (SDLC-HOOK-04.01).
_expect fail spec-review-result.schema.json "spec-review rejects an unsupported outcome" \
  '{"schema_version":"1","phase":"spec-review","outcome":"maybe","rationale":"x","pr_number":5,"head_sha":"abc1234","comment":"y"}'

# Arbitrary label name / extra property is rejected (SDLC-HOOK-04.02):
# additionalProperties is false, so a result cannot smuggle a label field.
_expect fail code-review-result.schema.json "code-review rejects an arbitrary extra field" \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"x","pr_number":5,"head_sha":"abc1234","comment":"y","labels":["agent/anything"]}'

# Wrong phase constant is rejected.
_expect fail triage-result.schema.json "triage rejects a wrong phase constant" \
  '{"schema_version":"1","phase":"code-review","outcome":"workable","rationale":"x"}'

# Missing required evidence for a review verdict.
_expect fail code-review-result.schema.json "code-review requires pr_number and head_sha" \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"x","comment":"y"}'

# completed code-implementation requires evidence; blocked does not.
_expect fail code-implementation-result.schema.json "completed code-impl requires evidence" \
  '{"schema_version":"1","phase":"code-implementation","outcome":"completed","rationale":"x"}'
_expect pass code-implementation-result.schema.json "blocked code-impl needs no evidence" \
  '{"schema_version":"1","phase":"code-implementation","outcome":"blocked","rationale":"no PR"}'

# Malformed JSON is rejected (SDLC-HOOK-06.01).
_expect fail triage-result.schema.json "malformed JSON is rejected" \
  '{"schema_version":"1","phase":"triage",'

printf '\n%d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
