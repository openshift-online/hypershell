#!/usr/bin/env bash
# lifecycle_post_test.sh - integration tests for post-lifecycle.sh (SDLC-HOOK-13).
#
# Drives the real shared post-script end to end with a mocked `gh` and
# `fullsend` on PATH, backed by a flat-file label store. It invokes NO local
# dispatcher, shell launcher, or Go program (SDLC-HOOK-13.02): only the native
# shared script and stubbed GitHub I/O. Auto-discovered by
# scripts/run-shell-unit-tests.sh.

set -uo pipefail

_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POST="${_here}/../post-lifecycle.sh"

PASS=0
FAIL=0
_ok() { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
_no() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; [[ -n "${2:-}" ]] && printf '       %s\n' "$2"; }

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
BIN="${WORK}/bin"
mkdir -p "${BIN}"

# ---- Mock gh -------------------------------------------------------------
cat > "${BIN}/gh" <<'MOCK'
#!/usr/bin/env bash
set -uo pipefail
[[ "${1:-}" == "api" ]] || { echo "mock gh: unsupported $*" >&2; exit 2; }
shift
method=GET; path=""; jqexpr=""; declare -a adds=(); want_close=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -X) method="$2"; shift 2;;
    --jq) jqexpr="$2"; shift 2;;
    --paginate|--slurp) shift;;
    -f) case "$2" in
          labels\[\]=*) adds+=("${2#labels[]=}");;
          state=closed) want_close=1;;
        esac; shift 2;;
    repos/*) path="$1"; shift;;
    *) shift;;
  esac
done
_decode() { printf '%s' "${1//\%2F//}"; }
state="${MOCK_STATE}"
case "${path}" in
  */commits/*/check-runs) printf '%s' "${MOCK_CHECKRUNS:-[]}"; exit 0;;
  */commits/*/status) printf '%s' "${MOCK_STATUSES:-[]}"; exit 0;;
  */protection/required_status_checks) printf '%s' "${MOCK_REQUIRED:-}"; exit 0;;
  */timeline) printf '%s' "${MOCK_TIMELINE:-[]}"; exit 0;;
  */pulls/*)
    case "${jqexpr}" in
      *base.ref*) printf '%s\t%s' "${MOCK_HEAD_SHA:-}" "${MOCK_BASE_REF:-main}";;
      .head.sha)  printf '%s' "${MOCK_HEAD_SHA:-}";;
      .state)     printf '%s' "${MOCK_PR_STATE:-open}";;
      *body*)     printf '%s' "${MOCK_PR_BODY:-}";;
    esac
    exit 0;;
esac
case "${method}:${path}" in
  GET:*/issues/*/labels) cat "${state}"; exit 0;;
  POST:*/issues/*/labels)
    for l in "${adds[@]}"; do grep -qxF "${l}" "${state}" || printf '%s\n' "${l}" >> "${state}"; done
    exit 0;;
  DELETE:*/issues/*/labels/*)
    name="$(_decode "${path##*/labels/}")"
    grep -vxF "${name}" "${state}" > "${state}.tmp" || true
    mv "${state}.tmp" "${state}"; exit 0;;
  GET:*/labels/*) exit 0;;            # ensure-label GET: label exists
  POST:*/labels) exit 0;;             # create label
  GET:*/issues/*) printf '%s' "$(cat "${MOCK_ISSUE_STATE:-/dev/null}" 2>/dev/null || echo open)"; exit 0;;
  PATCH:*/issues/*) [[ "${want_close}" == 1 ]] && printf 'closed' > "${MOCK_ISSUE_STATE}"; exit 0;;
esac
echo "mock gh: unhandled ${method} ${path} (jq=${jqexpr})" >&2
exit 3
MOCK
chmod +x "${BIN}/gh"

# ---- Mock fullsend -------------------------------------------------------
# The sticky-comment primitive. MOCK_COMMENT_FAIL=1 simulates a post failure so
# the required-comment hard-fail path can be exercised.
cat > "${BIN}/fullsend" <<'MOCK'
#!/usr/bin/env bash
set -uo pipefail
if [[ "${1:-}" == "issues" && "${2:-}" == "post-comment" ]]; then
  shift 2
  marker=""
  while [[ $# -gt 0 ]]; do
    case "$1" in --marker) marker="$2"; shift 2;; *) shift;; esac
  done
  body="$(cat)"
  if [[ "${MOCK_COMMENT_FAIL:-0}" == 1 ]]; then
    echo "mock fullsend: post-comment failed" >&2
    exit 1
  fi
  printf 'comment\n' >> "${MOCK_COMMENTS:-/dev/null}"
  [[ -n "${MOCK_MARKERS:-}" ]] && printf '%s\n' "${marker}" >> "${MOCK_MARKERS}"
  [[ -n "${MOCK_BODY:-}" ]] && printf '%s\n--END--\n' "${body}" >> "${MOCK_BODY}"
  exit 0
fi
exit 0
MOCK
chmod +x "${BIN}/fullsend"

# Seed a label store file from a list of labels.
_seed() {
  local file="$1"; shift
  : > "${file}"
  local l
  for l in "$@"; do printf '%s\n' "${l}" >> "${file}"; done
}
_has_label() { grep -qxF "$2" "$1"; }

# Run the post-script in an isolated RUN dir with a validated result.
# Usage: _run <phase> <result-json> [extra env KEY=VAL ...]
_run() {
  local phase="$1" result="$2"; shift 2
  local run="${WORK}/run.$$.${RANDOM}"
  mkdir -p "${run}/iteration-1/output"
  printf '%s' "${result}" > "${run}/iteration-1/output/agent-result.json"
  ( cd "${run}" && env -i \
      PATH="${BIN}:/usr/bin:/bin" \
      HOME="${WORK}" \
      LC_ALL=C.UTF-8 \
      GH_TOKEN=x \
      PHASE="${phase}" \
      ISSUE_URL="https://github.com/openshift-online/hypershell/issues/868" \
      "$@" \
      bash "${POST}" )
}

REPO_SCOPE="agent/scope/x"

# ---- Scenario A: happy path spec-review approved (SDLC-HOOK-08.01) --------
S="${WORK}/A.labels"
_seed "${S}" agent/reviewable-spec agent/allowed "${REPO_SCOPE}" agent/execution-record
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"approved","rationale":"good","pr_number":5,"head_sha":"abc1234","comment":"approved"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/A.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/review-spec-approved \
   && ! _has_label "${S}" agent/reviewable-spec \
   && _has_label "${S}" agent/allowed && _has_label "${S}" "${REPO_SCOPE}" \
   && _has_label "${S}" agent/execution-record; then
  _ok "spec-review approved applies and preserves protected labels"
else
  _no "spec-review approved applies and preserves protected labels" "$(cat "${WORK}/A.out")"
fi

# ---- Scenario B: interrupted transition converges on reapply (HOOK-09.02) -
S="${WORK}/B.labels"
_seed "${S}" agent/reviewable-spec agent/review-spec-approved agent/allowed
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"approved","rationale":"g","pr_number":5,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/B.out" 2>&1
if _has_label "${S}" agent/review-spec-approved && ! _has_label "${S}" agent/reviewable-spec; then
  _ok "interrupted transition converges to the expected final state"
else
  _no "interrupted transition converges to the expected final state" "$(cat "${WORK}/B.out")"
fi

# ---- Scenario B2: reapply recovers a stranded verdict (HOOK-09.02) --------
# Simulate a transition interrupted so the reviewable-* label was already removed
# but the verdict label was never added (the old remove-first window, or any
# partial state with no phase label). Reapplying the same validated result must
# land the verdict rather than exit with no change.
S="${WORK}/B2.labels"
_seed "${S}" agent/allowed "${REPO_SCOPE}" agent/execution-record
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"rejected","rationale":"needs work","pr_number":5,"head_sha":"abc1234","comment":"rejected"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/B2.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/review-code-rejected \
   && ! _has_label "${S}" agent/reviewable-code \
   && _has_label "${S}" agent/allowed && _has_label "${S}" "${REPO_SCOPE}"; then
  _ok "reapply recovers a stranded verdict when the reviewable label is already gone"
else
  _no "reapply recovers a stranded verdict when the reviewable label is already gone" "rc=${rc} $(cat "${WORK}/B2.out")"
fi

# ---- Scenario B3: a settled competing verdict is never overridden ---------
# The reviewable-* label is gone and the OTHER verdict is already present (this
# phase settled the other way). A late/duplicate result must not override it.
S="${WORK}/B3.labels"
_seed "${S}" agent/review-code-rejected agent/allowed "${REPO_SCOPE}"
before="$(sort "${S}")"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"approved"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/B3.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" == 0 ]] && [[ "${before}" == "${after}" ]] \
   && _has_label "${S}" agent/review-code-rejected \
   && ! _has_label "${S}" agent/review-code-approved; then
  _ok "a settled competing verdict is not overridden on reapply"
else
  _no "a settled competing verdict is not overridden on reapply" "rc=${rc} $(cat "${WORK}/B3.out")"
fi

# ---- Scenario C: idempotent reapply after success (HOOK-09.01) ------------
# After scenario A, reviewable-spec is gone; reapplying the same result changes nothing.
S="${WORK}/C.labels"
_seed "${S}" agent/review-spec-approved agent/allowed
before="$(sort "${S}")"
COMMENTS="${WORK}/C.comments"; : > "${COMMENTS}"
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"approved","rationale":"g","pr_number":5,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 MOCK_COMMENTS="${COMMENTS}" \
  > "${WORK}/C.out" 2>&1
after="$(sort "${S}")"
if [[ "${before}" == "${after}" ]] && [[ ! -s "${COMMENTS}" ]]; then
  _ok "reapplying a completed result changes nothing and posts no duplicate comment"
else
  _no "reapplying a completed result changes nothing and posts no duplicate comment" "$(cat "${WORK}/C.out")"
fi

# ---- Scenario D: stale review rejected (HOOK-07.01) -----------------------
S="${WORK}/D.labels"
_seed "${S}" agent/reviewable-code agent/allowed
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"g","pr_number":5,"head_sha":"oldsha1","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=newsha2 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=oldsha1 \
  > "${WORK}/D.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/reviewable-code \
   && ! _has_label "${S}" agent/review-code-approved; then
  _ok "stale code review (head moved) applies no verdict"
else
  _no "stale code review (head moved) applies no verdict" "$(cat "${WORK}/D.out")"
fi

# ---- Scenario E: human block added during run is preserved (HOOK-02.02) ---
S="${WORK}/E.labels"
_seed "${S}" agent/review-spec-approved agent/blocked agent/allowed "${REPO_SCOPE}"
_run code-implementation \
  '{"schema_version":"1","phase":"code-implementation","outcome":"completed","rationale":"done","pr_number":5,"head_sha":"abc1234"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/E.out" 2>&1
if _has_label "${S}" agent/blocked && _has_label "${S}" agent/review-spec-approved \
   && _has_label "${S}" agent/reviewable-code; then
  _ok "code-impl completed preserves a human block and the spec approval"
else
  _no "code-impl completed preserves a human block and the spec approval" "$(cat "${WORK}/E.out")"
fi

# ---- Scenario F: invalid outcome is rejected without mutation (HOOK-04.01)-
S="${WORK}/F.labels"
_seed "${S}" agent/reviewable-code agent/allowed
before="$(sort "${S}")"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"bogus","rationale":"g","pr_number":5,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/F.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]]; then
  _ok "an unsupported outcome fails without changing labels"
else
  _no "an unsupported outcome fails without changing labels" "rc=${rc} $(cat "${WORK}/F.out")"
fi

# ---- Scenario G: no validated result does not advance (HOOK-06.02) --------
S="${WORK}/G.labels"
_seed "${S}" agent/reviewable-spec agent/allowed
before="$(sort "${S}")"
run="${WORK}/G.run"; mkdir -p "${run}"   # no iteration-*/output at all
( cd "${run}" && env -i PATH="${BIN}:/usr/bin:/bin" HOME="${WORK}" LC_ALL=C.UTF-8 \
    GH_TOKEN=x PHASE=spec-review \
    ISSUE_URL="https://github.com/openshift-online/hypershell/issues/868" \
    MOCK_STATE="${S}" bash "${POST}" ) > "${WORK}/G.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" == 0 ]] && [[ "${before}" == "${after}" ]]; then
  _ok "a missing validated result does not advance the workflow"
else
  _no "a missing validated result does not advance the workflow" "rc=${rc} $(cat "${WORK}/G.out")"
fi

# ---- Scenario H: pr_number mismatch is rejected (HOOK-04.03) --------------
S="${WORK}/H.labels"
_seed "${S}" agent/reviewable-code agent/allowed
before="$(sort "${S}")"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"g","pr_number":999,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/H.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]]; then
  _ok "a mismatched pr_number fails without changing labels"
else
  _no "a mismatched pr_number fails without changing labels" "rc=${rc} $(cat "${WORK}/H.out")"
fi

# ---- Scenario I: required checks gate an approval (HOOK-07.02) ------------
S="${WORK}/I.labels"
_seed "${S}" agent/reviewable-code agent/allowed
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"g","pr_number":5,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_CHECKRUNS='[{"status":"completed","conclusion":"failure"}]' \
  > "${WORK}/I.out" 2>&1
if _has_label "${S}" agent/reviewable-code && ! _has_label "${S}" agent/review-code-approved; then
  _ok "an approval with failing required checks is not applied"
else
  _no "an approval with failing required checks is not applied" "$(cat "${WORK}/I.out")"
fi

# ---- Scenario J: third spec rejection blocks the record (HOOK-12.01) ------
S="${WORK}/J.labels"
_seed "${S}" agent/reviewable-spec agent/allowed
TL='[{"event":"labeled","label":{"name":"agent/review-spec-rejected"}},{"event":"labeled","label":{"name":"agent/review-spec-rejected"}}]'
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"rejected","rationale":"no","pr_number":5,"head_sha":"abc1234","comment":"bad"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 MOCK_TIMELINE="${TL}" \
  > "${WORK}/J.out" 2>&1
if _has_label "${S}" agent/review-spec-rejected && _has_label "${S}" agent/blocked \
   && ! _has_label "${S}" agent/reviewable-spec; then
  _ok "third spec rejection adds agent/blocked from timeline evidence"
else
  _no "third spec rejection adds agent/blocked from timeline evidence" "$(cat "${WORK}/J.out")"
fi

# ---- Scenario K: triage duplicate adds label and closes the issue --------
S="${WORK}/K.labels"
_seed "${S}" agent/allowed "${REPO_SCOPE}" agent/execution-record
IST="${WORK}/K.issuestate"; printf 'open' > "${IST}"
_run triage \
  '{"schema_version":"1","phase":"triage","outcome":"duplicate","rationale":"dupe","comment":"closing"}' \
  MOCK_STATE="${S}" MOCK_ISSUE_STATE="${IST}" \
  > "${WORK}/K.out" 2>&1
if _has_label "${S}" agent/duplicate && [[ "$(cat "${IST}")" == "closed" ]]; then
  _ok "triage duplicate labels and closes the issue"
else
  _no "triage duplicate labels and closes the issue" "$(cat "${WORK}/K.out")"
fi

# ---- Scenario L: code-review approved with green checks applies (HOOK-07.02)
S="${WORK}/L.labels"
_seed "${S}" agent/reviewable-code agent/review-spec-approved agent/allowed
LCOMMENTS="${WORK}/L.comments"; : > "${LCOMMENTS}"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"approved"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_CHECKRUNS='[{"status":"completed","conclusion":"success"}]' MOCK_COMMENTS="${LCOMMENTS}" \
  > "${WORK}/L.out" 2>&1
if _has_label "${S}" agent/review-code-approved && ! _has_label "${S}" agent/reviewable-code \
   && _has_label "${S}" agent/review-spec-approved && [[ -s "${LCOMMENTS}" ]]; then
  _ok "code-review approved with green checks applies, preserves the spec approval, and posts the verdict comment"
else
  _no "code-review approved with green checks applies, preserves the spec approval, and posts the verdict comment" "$(cat "${WORK}/L.out")"
fi

# ---- Scenario M: approval blocked when CI has not reported (no signals) ----
S="${WORK}/M.labels"
_seed "${S}" agent/reviewable-code agent/allowed
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_CHECKRUNS='[]' MOCK_STATUS_TOTAL=0 \
  > "${WORK}/M.out" 2>&1
if _has_label "${S}" agent/reviewable-code && ! _has_label "${S}" agent/review-code-approved; then
  _ok "approval is not applied when no checks have reported"
else
  _no "approval is not applied when no checks have reported" "$(cat "${WORK}/M.out")"
fi

# ---- Scenario N: spec-impl completed with a recorded PR (HOOK-01 bootstrap) --
S="${WORK}/N.labels"
_seed "${S}" agent/workable agent/allowed "${REPO_SCOPE}" agent/execution-record
_run spec-implementation \
  '{"schema_version":"1","phase":"spec-implementation","outcome":"completed","rationale":"spec pushed","pr_number":869,"head_sha":"abc1234"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=869 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/N.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/reviewable-spec \
   && ! _has_label "${S}" agent/workable \
   && _has_label "${S}" agent/allowed && _has_label "${S}" "${REPO_SCOPE}"; then
  _ok "spec-impl completed with a recorded PR opens spec review"
else
  _no "spec-impl completed with a recorded PR opens spec review" "rc=${rc} $(cat "${WORK}/N.out")"
fi

# ---- Scenario O: spec-impl completed on a fresh issue, agent PR verified ------
# No recorded LIFECYCLE_PR_NUMBER (fresh issue the agent just opened a PR for);
# the agent's pr_number is adopted after gh_pr_is_linked confirms it (HOOK-04.03).
S="${WORK}/O.labels"
_seed "${S}" agent/workable agent/allowed "${REPO_SCOPE}"
_run spec-implementation \
  '{"schema_version":"1","phase":"spec-implementation","outcome":"completed","rationale":"spec pushed","pr_number":869,"head_sha":"abc1234"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 MOCK_PR_STATE=open MOCK_PR_BODY="Closes #868" \
  > "${WORK}/O.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/reviewable-spec && ! _has_label "${S}" agent/workable; then
  _ok "spec-impl completed on a fresh issue adopts the agent's verified PR"
else
  _no "spec-impl completed on a fresh issue adopts the agent's verified PR" "rc=${rc} $(cat "${WORK}/O.out")"
fi

# ---- Scenario O2: colon closing form "Closes: #868" links (regex fix) ---------
S="${WORK}/O2.labels"
_seed "${S}" agent/workable agent/allowed "${REPO_SCOPE}"
_run spec-implementation \
  '{"schema_version":"1","phase":"spec-implementation","outcome":"completed","rationale":"spec pushed","pr_number":869,"head_sha":"abc1234"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 MOCK_PR_STATE=open MOCK_PR_BODY="Closes: #868" \
  > "${WORK}/O2.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/reviewable-spec && ! _has_label "${S}" agent/workable; then
  _ok "colon closing form (Closes: #868) is recognized as linking the PR"
else
  _no "colon closing form (Closes: #868) is recognized as linking the PR" "rc=${rc} $(cat "${WORK}/O2.out")"
fi

# ---- Scenario P: spec-impl completed, agent PR not linked -> rejected ---------
S="${WORK}/P.labels"
_seed "${S}" agent/workable agent/allowed
before="$(sort "${S}")"
_run spec-implementation \
  '{"schema_version":"1","phase":"spec-implementation","outcome":"completed","rationale":"x","pr_number":999,"head_sha":"abc1234"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 MOCK_PR_STATE=open MOCK_PR_BODY="unrelated body" \
  > "${WORK}/P.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]]; then
  _ok "spec-impl with an unverifiable PR fails without changing labels"
else
  _no "spec-impl with an unverifiable PR fails without changing labels" "rc=${rc} $(cat "${WORK}/P.out")"
fi

# ---- Scenario P2: spec-impl pr_number mismatches the recorded PR -> rejected --
S="${WORK}/P2.labels"
_seed "${S}" agent/workable agent/allowed
before="$(sort "${S}")"
_run spec-implementation \
  '{"schema_version":"1","phase":"spec-implementation","outcome":"completed","rationale":"x","pr_number":999,"head_sha":"abc1234"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=869 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/P2.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]]; then
  _ok "spec-impl pr_number mismatching the recorded PR fails without changing labels"
else
  _no "spec-impl pr_number mismatching the recorded PR fails without changing labels" "rc=${rc} $(cat "${WORK}/P2.out")"
fi

# ---- Scenario Q: a failing NON-required check does not block approval ---------
# Required contexts from branch protection = {ci-required}. The advisory check is
# failing but is not required, so the approval is still eligible (HOOK-07.02).
S="${WORK}/Q.labels"
_seed "${S}" agent/reviewable-code agent/review-spec-approved agent/allowed
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"approved"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_BASE_REF=main MOCK_REQUIRED='ci-required' \
  MOCK_CHECKRUNS='[{"name":"ci-required","id":2,"status":"completed","conclusion":"success"},{"name":"advisory","id":1,"status":"completed","conclusion":"failure"}]' \
  > "${WORK}/Q.out" 2>&1
if _has_label "${S}" agent/review-code-approved && ! _has_label "${S}" agent/reviewable-code; then
  _ok "a failing non-required check does not block an approval"
else
  _no "a failing non-required check does not block an approval" "$(cat "${WORK}/Q.out")"
fi

# ---- Scenario R: a required check that has not reported blocks the approval ---
S="${WORK}/R.labels"
_seed "${S}" agent/reviewable-code agent/allowed
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"a"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_BASE_REF=main MOCK_REQUIRED='ci-required' \
  MOCK_CHECKRUNS='[{"name":"advisory","id":1,"status":"completed","conclusion":"success"}]' \
  > "${WORK}/R.out" 2>&1
if _has_label "${S}" agent/reviewable-code && ! _has_label "${S}" agent/review-code-approved; then
  _ok "an unreported required check keeps an approval pending"
else
  _no "an unreported required check keeps an approval pending" "$(cat "${WORK}/R.out")"
fi

# ---- Scenario S: a failed REQUIRED review comment blocks the transition -------
# The comment is posted before any label change, so a failure to post the required
# review record leaves the labels untouched and fails visibly (SDLC-HOOK-10).
S="${WORK}/S.labels"
_seed "${S}" agent/reviewable-spec agent/allowed "${REPO_SCOPE}"
before="$(sort "${S}")"
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"approved","rationale":"good","pr_number":5,"head_sha":"abc1234","comment":"approved"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_COMMENT_FAIL=1 \
  > "${WORK}/S.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]] \
   && grep -q "required spec-review review comment" "${WORK}/S.out"; then
  _ok "a failed required review comment blocks the transition and fails visibly"
else
  _no "a failed required review comment blocks the transition and fails visibly" "rc=${rc} $(cat "${WORK}/S.out")"
fi

# ---- Scenario T: a failed OPTIONAL comment still applies the transition --------
# code-implementation's comment is optional; a failed post is a warning only and
# the reviewable-code transition still lands.
S="${WORK}/T.labels"
_seed "${S}" agent/review-spec-approved agent/allowed "${REPO_SCOPE}"
_run code-implementation \
  '{"schema_version":"1","phase":"code-implementation","outcome":"completed","rationale":"done","pr_number":5,"head_sha":"abc1234","comment":"fyi"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_COMMENT_FAIL=1 \
  > "${WORK}/T.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/reviewable-code \
   && _has_label "${S}" agent/review-spec-approved; then
  _ok "a failed optional comment warns but still applies the transition"
else
  _no "a failed optional comment warns but still applies the transition" "rc=${rc} $(cat "${WORK}/T.out")"
fi

# ---- Scenario U: reapply in the add-first partial window does not double-count -
# An interrupted rejection left reviewable-spec in place AND the verdict label
# already added. The timeline therefore already records this rejection. On reapply
# the block count must NOT add the current rejection a second time: with one real
# prior rejection plus this one, the true count is 2, below the spec threshold of
# 3, so agent/blocked must NOT be added (SDLC-HOOK-09, SDLC-HOOK-12.01).
S="${WORK}/U.labels"
_seed "${S}" agent/reviewable-spec agent/review-spec-rejected agent/allowed "${REPO_SCOPE}"
# Two rejected events in the timeline: one genuine prior, one from the interrupted
# attempt that already added the label now present on the record.
TL='[{"event":"labeled","label":{"name":"agent/review-spec-rejected"}},{"event":"labeled","label":{"name":"agent/review-spec-rejected"}}]'
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"rejected","rationale":"no","pr_number":5,"head_sha":"abc1234","comment":"bad"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 MOCK_TIMELINE="${TL}" \
  > "${WORK}/U.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/review-spec-rejected \
   && ! _has_label "${S}" agent/blocked \
   && ! _has_label "${S}" agent/reviewable-spec; then
  _ok "reapply in the partial window does not double-count the rejection into a block"
else
  _no "reapply in the partial window does not double-count the rejection into a block" "rc=${rc} $(cat "${WORK}/U.out")"
fi

# ---- Scenario V: a review verdict with an empty comment is refused ------------
# The post-script re-verifies independently of the schema: a review phase requires
# a review record, so an empty comment must fail the transition with no label
# change rather than applying the verdict with no record (SDLC-HOOK-10).
S="${WORK}/V.labels"
_seed "${S}" agent/reviewable-spec agent/allowed "${REPO_SCOPE}"
before="$(sort "${S}")"
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"rejected","rationale":"no","pr_number":5,"head_sha":"abc1234","comment":""}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/V.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]] \
   && ! _has_label "${S}" agent/review-spec-rejected \
   && grep -q "requires a review comment" "${WORK}/V.out"; then
  _ok "a review verdict with an empty comment is refused with no label change"
else
  _no "a review verdict with an empty comment is refused with no label change" "rc=${rc} $(cat "${WORK}/V.out")"
fi

# ---- Scenario W: a deferred approval posts a visible record, no label change ---
# An approval whose required checks are not yet green must not be a log-only exit:
# agent/reviewable-code is left in place AND a visible deferral record is posted so
# the approval is recoverable by reapply once the checks are green (SDLC-HOOK-07,
# SDLC-HOOK-10). The sticky marker is the phase marker, so the later green-check
# verdict edits this record in place with no duplicate (SDLC-HOOK-09).
S="${WORK}/W.labels"
_seed "${S}" agent/reviewable-code agent/allowed "${REPO_SCOPE}"
WCOMMENTS="${WORK}/W.comments"; : > "${WCOMMENTS}"
WMARKERS="${WORK}/W.markers"; : > "${WMARKERS}"
WBODY="${WORK}/W.body"; : > "${WBODY}"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"approved by the reviewer"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_CHECKRUNS='[{"status":"completed","conclusion":"failure"}]' \
  MOCK_COMMENTS="${WCOMMENTS}" MOCK_MARKERS="${WMARKERS}" MOCK_BODY="${WBODY}" \
  > "${WORK}/W.out" 2>&1
rc=$?
# The deferral record must carry the SAME phase sticky marker the later verdict
# uses, so a reapply once checks are green edits it in place with no duplicate
# (SDLC-HOOK-09), and its body must carry the deferral banner and the reviewer's
# own comment.
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/reviewable-code \
   && ! _has_label "${S}" agent/review-code-approved \
   && grep -qxF '<!-- fullsend:code-review-agent -->' "${WMARKERS}" \
   && grep -q "deferred pending required checks" "${WBODY}" \
   && grep -q "approved by the reviewer" "${WBODY}" \
   && grep -q "deferred" "${WORK}/W.out"; then
  _ok "a deferred approval posts a visible marked record and leaves the item reviewable"
else
  _no "a deferred approval posts a visible marked record and leaves the item reviewable" "rc=${rc} marker=$(cat "${WMARKERS}") $(cat "${WORK}/W.out")"
fi

# ---- Scenario W2: a deferred approval converges to the verdict once checks green -
# Reapplying the SAME validated result after the checks turn green must apply the
# approval, and the verdict comment reuses the same phase marker as the deferral
# record so no duplicate comment is produced (SDLC-HOOK-09 convergence).
# Continues from scenario W's label store (still agent/reviewable-code).
W2MARKERS="${WORK}/W2.markers"; : > "${W2MARKERS}"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":"approved by the reviewer"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_CHECKRUNS='[{"status":"completed","conclusion":"success"}]' \
  MOCK_MARKERS="${W2MARKERS}" \
  > "${WORK}/W2.out" 2>&1
rc=$?
if [[ "${rc}" == 0 ]] && _has_label "${S}" agent/review-code-approved \
   && ! _has_label "${S}" agent/reviewable-code \
   && grep -qxF '<!-- fullsend:code-review-agent -->' "${W2MARKERS}"; then
  _ok "a deferred approval converges to the verdict (same marker) once checks are green"
else
  _no "a deferred approval converges to the verdict (same marker) once checks are green" "rc=${rc} $(cat "${WORK}/W2.out")"
fi

# ---- Scenario W3: a deferred approval with an empty comment is refused ----------
# The deferral path enforces the required-review-comment rule like the apply path:
# a code-review approval with no comment must fail with no record, not post a
# banner-only deferral and exit 0 (SDLC-HOOK-10 defense in depth).
S3="${WORK}/W3.labels"
_seed "${S3}" agent/reviewable-code agent/allowed "${REPO_SCOPE}"
before="$(sort "${S3}")"
W3COMMENTS="${WORK}/W3.comments"; : > "${W3COMMENTS}"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"abc1234","comment":""}' \
  MOCK_STATE="${S3}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_CHECKRUNS='[{"status":"completed","conclusion":"failure"}]' MOCK_COMMENTS="${W3COMMENTS}" \
  > "${WORK}/W3.out" 2>&1
rc=$?
after="$(sort "${S3}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]] && [[ ! -s "${W3COMMENTS}" ]]; then
  _ok "a deferred approval with an empty comment fails with no record and no label change"
else
  _no "a deferred approval with an empty comment fails with no record and no label change" "rc=${rc} $(cat "${WORK}/W3.out")"
fi

# ---- Scenario X: a non-hex / glob agent head_sha is treated as stale -----------
# The agent head_sha is untrusted and, on the fallback path, has not passed the
# JSON Schema. A value like "*" must not be used as a glob pattern that matches any
# recorded head: the post-script re-validates it as 7-40 hex and, when it does not
# match, treats the review result as stale with no verdict and no label change.
S="${WORK}/X.labels"
_seed "${S}" agent/reviewable-code agent/allowed "${REPO_SCOPE}"
before="$(sort "${S}")"
_run code-review \
  '{"schema_version":"1","phase":"code-review","outcome":"approved","rationale":"lgtm","pr_number":5,"head_sha":"*","comment":"approved"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  > "${WORK}/X.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" == 0 ]] && [[ "${before}" == "${after}" ]] \
   && _has_label "${S}" agent/reviewable-code \
   && ! _has_label "${S}" agent/review-code-approved \
   && grep -q "not a 7-40 char hex commit sha" "${WORK}/X.out"; then
  _ok "a non-hex (glob) agent head_sha is treated as stale with no verdict or label change"
else
  _no "a non-hex (glob) agent head_sha is treated as stale with no verdict or label change" "rc=${rc} $(cat "${WORK}/X.out")"
fi

# ---- Scenario Y: an unparseable timeline fails the rejection closed ------------
# The block decision for a rejection depends on a numeric prior-rejection count
# from the timeline (SDLC-HOOK-12). If that count cannot be computed (a jq/read
# failure yields a non-numeric value) the transition must abort before any mutation
# rather than default to 0 and silently skip a possibly-required agent/blocked.
S="${WORK}/Y.labels"
_seed "${S}" agent/reviewable-spec agent/allowed "${REPO_SCOPE}"
before="$(sort "${S}")"
_run spec-review \
  '{"schema_version":"1","phase":"spec-review","outcome":"rejected","rationale":"no","pr_number":5,"head_sha":"abc1234","comment":"bad"}' \
  MOCK_STATE="${S}" MOCK_HEAD_SHA=abc1234 LIFECYCLE_PR_NUMBER=5 LIFECYCLE_PR_HEAD_SHA=abc1234 \
  MOCK_TIMELINE='this is not json' \
  > "${WORK}/Y.out" 2>&1
rc=$?
after="$(sort "${S}")"
if [[ "${rc}" != 0 ]] && [[ "${before}" == "${after}" ]] \
   && ! _has_label "${S}" agent/review-spec-rejected \
   && grep -q "could not compute a numeric prior-rejection count" "${WORK}/Y.out"; then
  _ok "an unparseable timeline fails the rejection closed with no label change"
else
  _no "an unparseable timeline fails the rejection closed with no label change" "rc=${rc} $(cat "${WORK}/Y.out")"
fi

printf '\n%d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
