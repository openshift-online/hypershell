#!/usr/bin/env bash
# lifecycle_pre_test.sh - integration tests for pre-lifecycle.sh (SDLC-HOOK-03,
# SDLC-HOOK-13).
#
# Drives the real shared pre-script with a mocked `gh` on PATH, backed by a
# flat-file label store. It invokes NO local dispatcher, shell launcher, or Go
# program (SDLC-HOOK-13.02): only the native shared script and stubbed GitHub I/O.
# Auto-discovered by scripts/run-shell-unit-tests.sh.
#
# The pre-script relays KEY=VALUE lines to $FULLSEND_PRESCRIPT_OUTPUT and requests
# a skip with a `skip=<reason>` line. Each test inspects that relay file.

set -uo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PRE="${_here}/../pre-lifecycle.sh"

PASS=0
FAIL=0
_ok() { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
_no() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; [[ -n "${2:-}" ]] && printf '       %s\n' "$2"; }

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
BIN="${WORK}/bin"
mkdir -p "${BIN}"

# ---- Mock gh (read-only) -------------------------------------------------
cat > "${BIN}/gh" <<'MOCK'
#!/usr/bin/env bash
set -uo pipefail
[[ "${1:-}" == "api" ]] || { echo "mock gh: unsupported $*" >&2; exit 2; }
shift
path=""; jqexpr=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --jq) jqexpr="$2"; shift 2;;
    --paginate|--slurp) shift;;
    -X) shift 2;;
    -f) shift 2;;
    repos/*) path="$1"; shift;;
    *) shift;;
  esac
done
case "${path}" in
  */issues/*/labels) cat "${MOCK_STATE}"; exit 0;;
  */timeline) printf '%s' "${MOCK_TIMELINE:-}"; exit 0;;   # already in post-jq form (candidate numbers)
  */pulls/*)
    case "${jqexpr}" in
      .head.sha) printf '%s' "${MOCK_HEAD_SHA:-}";;
      .state)    printf '%s' "${MOCK_PR_STATE:-open}";;
      *body*)    printf '%s' "${MOCK_PR_BODY:-}";;
    esac
    exit 0;;
esac
echo "mock gh: unhandled ${path} (jq=${jqexpr})" >&2
exit 3
MOCK
chmod +x "${BIN}/gh"

_seed() { local f="$1"; shift; : > "${f}"; local l; for l in "$@"; do printf '%s\n' "${l}" >> "${f}"; done; }

# _runpre <relayfile> <phase> [env KEY=VAL ...]
# Returns the pre-script exit code; relay lines land in <relayfile>.
_runpre() {
  local relay="$1" phase="$2"; shift 2
  : > "${relay}"
  ( env -i \
      PATH="${BIN}:/usr/bin:/bin" HOME="${WORK}" LC_ALL=C.UTF-8 \
      GH_TOKEN=x PHASE="${phase}" \
      ISSUE_URL="https://github.com/openshift-online/hypershell/issues/868" \
      FULLSEND_PRESCRIPT_OUTPUT="${relay}" \
      "$@" \
      bash "${PRE}" )
}
_relayed() { grep -q "^$2=" "$1"; }
_skipped() { grep -q "^skip=" "$1"; }

# ---- A: spec-implementation bootstraps a fresh issue with no PR (BLOCKER) --
R="${WORK}/A.relay"; S="${WORK}/A.labels"
_seed "${S}" agent/workable agent/allowed agent/execution-record
_runpre "${R}" spec-implementation MOCK_STATE="${S}" MOCK_TIMELINE="" > "${WORK}/A.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && ! _skipped "${R}" \
   && _relayed "${R}" LIFECYCLE_PHASE && ! _relayed "${R}" LIFECYCLE_PR_NUMBER; then
  _ok "spec-implementation does not skip a fresh issue that has no PR yet"
else
  _no "spec-implementation does not skip a fresh issue that has no PR yet" "rc=${rc} $(cat "${WORK}/A.out") ; relay=$(cat "${R}")"
fi

# ---- B: spec-implementation records an existing PR (idempotent rerun) ------
R="${WORK}/B.relay"; S="${WORK}/B.labels"
_seed "${S}" agent/workable agent/allowed
_runpre "${R}" spec-implementation \
  MOCK_STATE="${S}" MOCK_TIMELINE="5" MOCK_PR_STATE=open MOCK_PR_BODY="Closes #868" MOCK_HEAD_SHA=deadbee \
  > "${WORK}/B.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && ! _skipped "${R}" \
   && grep -q '^LIFECYCLE_PR_NUMBER=5$' "${R}" && grep -q '^LIFECYCLE_PR_HEAD_SHA=deadbee$' "${R}"; then
  _ok "spec-implementation records a pre-existing linked PR and its head"
else
  _no "spec-implementation records a pre-existing linked PR and its head" "rc=${rc} $(cat "${WORK}/B.out") ; relay=$(cat "${R}")"
fi

# ---- C: lost authorization skips the run (HOOK-03.01) ---------------------
R="${WORK}/C.relay"; S="${WORK}/C.labels"
_seed "${S}" agent/reviewable-code   # no agent/allowed
_runpre "${R}" code-review MOCK_STATE="${S}" > "${WORK}/C.out" 2>&1
if _skipped "${R}"; then _ok "missing agent/allowed skips the run"
else _no "missing agent/allowed skips the run" "relay=$(cat "${R}")"; fi

# ---- D: lost scope skips the run ------------------------------------------
R="${WORK}/D.relay"; S="${WORK}/D.labels"
_seed "${S}" agent/workable agent/allowed   # scope label absent
_runpre "${R}" spec-implementation MOCK_STATE="${S}" LABEL_SCOPE=agent/scope/x > "${WORK}/D.out" 2>&1
if _skipped "${R}"; then _ok "missing scope label skips the run"
else _no "missing scope label skips the run" "relay=$(cat "${R}")"; fi

# ---- E: a human block skips every phase -----------------------------------
R="${WORK}/E.relay"; S="${WORK}/E.labels"
_seed "${S}" agent/reviewable-code agent/allowed agent/blocked
_runpre "${R}" code-review MOCK_STATE="${S}" MOCK_TIMELINE="5" MOCK_PR_STATE=open MOCK_PR_BODY="Closes #868" MOCK_HEAD_SHA=abc \
  > "${WORK}/E.out" 2>&1
if _skipped "${R}"; then _ok "a human block skips the run"
else _no "a human block skips the run" "relay=$(cat "${R}")"; fi

# ---- F: a PR phase with no linked PR skips --------------------------------
R="${WORK}/F.relay"; S="${WORK}/F.labels"
_seed "${S}" agent/reviewable-code agent/allowed
_runpre "${R}" code-review MOCK_STATE="${S}" MOCK_TIMELINE="" > "${WORK}/F.out" 2>&1
if _skipped "${R}"; then _ok "code-review with no linked PR skips the run"
else _no "code-review with no linked PR skips the run" "relay=$(cat "${R}")"; fi

# ---- G: a review phase records the PR head (HOOK-03.02) --------------------
R="${WORK}/G.relay"; S="${WORK}/G.labels"
_seed "${S}" agent/reviewable-code agent/allowed
_runpre "${R}" code-review \
  MOCK_STATE="${S}" MOCK_TIMELINE="5" MOCK_PR_STATE=open MOCK_PR_BODY="Closes #868" MOCK_HEAD_SHA=cafef00 \
  > "${WORK}/G.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && ! _skipped "${R}" \
   && grep -q '^LIFECYCLE_PR_NUMBER=5$' "${R}" && grep -q '^LIFECYCLE_PR_HEAD_SHA=cafef00$' "${R}"; then
  _ok "code-review records the PR number and head commit"
else
  _no "code-review records the PR number and head commit" "rc=${rc} relay=$(cat "${R}")"
fi

# ---- I: spec-review without a reviewable spec skips ------------------------
R="${WORK}/I.relay"; S="${WORK}/I.labels"
_seed "${S}" agent/review-spec-approved agent/allowed   # not reviewable-spec
_runpre "${R}" spec-review MOCK_STATE="${S}" > "${WORK}/I.out" 2>&1
if _skipped "${R}"; then _ok "spec-review without agent/reviewable-spec skips the run"
else _no "spec-review without agent/reviewable-spec skips the run" "relay=$(cat "${R}")"; fi

# ---- J: code-implementation without a trigger label skips -----------------
R="${WORK}/J.relay"; S="${WORK}/J.labels"
_seed "${S}" agent/workable agent/allowed   # neither review-spec-approved nor review-code-rejected
_runpre "${R}" code-implementation MOCK_STATE="${S}" > "${WORK}/J.out" 2>&1
if _skipped "${R}"; then _ok "code-implementation without a trigger label skips the run"
else _no "code-implementation without a trigger label skips the run" "relay=$(cat "${R}")"; fi

# ---- H: an already-triaged issue skips triage -----------------------------
R="${WORK}/H.relay"; S="${WORK}/H.labels"
_seed "${S}" agent/workable agent/allowed
_runpre "${R}" triage MOCK_STATE="${S}" > "${WORK}/H.out" 2>&1
if _skipped "${R}"; then _ok "triage skips an issue that already has a phase label"
else _no "triage skips an issue that already has a phase label" "relay=$(cat "${R}")"; fi

printf '\n%d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
