#!/usr/bin/env bash
# Verifies: SDLC-HOOK-07.02, SDLC-HOOK-07.03
# Realistic gh responses: error bodies remain on stdout with a nonzero exit;
# successful API responses are filtered by --jq just as the native CLI does.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
mkdir -p "${WORK}/bin"
cat > "${WORK}/bin/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
shift
path="$1"; shift
filter=""; slurp=0
while (( $# )); do
 case "$1" in
 --jq) filter="$2"; shift 2;;
 --slurp) slurp=1; shift;;
 *) shift;;
 esac
done
if [[ "${slurp}" == 1 && -n "${filter}" ]]; then exit 2; fi
case "${path}" in
 */protection/required_status_checks)
  if [[ "${CLASSIC_STATUS}" != 200 ]]; then printf '{"message":"Branch not protected","status":"%s"}' "${CLASSIC_STATUS}"; exit 1; fi
  body="${CLASSIC_BODY}";;
 */rules/branches/*)
  if [[ "${RULES_STATUS}" != 200 ]]; then printf '{"message":"Unavailable","status":"%s"}' "${RULES_STATUS}"; exit 1; fi
  body="${RULE_PAGES}";;
 */pulls/*) body='{"head":{"sha":"abc1234"},"base":{"ref":"main"}}';;
 */commits/*/check-runs) body="${CHECK_PAGES}";;
 */commits/*/status) body="${STATUS_BODY}";;
 *) echo "unexpected gh request: ${path}" >&2; exit 2;;
esac
if [[ -n "${filter}" ]]; then printf '%s' "${body}" | jq -r "${filter}"; else printf '%s' "${body}"; fi
MOCK
chmod +x "${WORK}/bin/gh"
export PATH="${WORK}/bin:${PATH}"
export CLASSIC_STATUS=404 RULES_STATUS=200 CLASSIC_BODY='{"contexts":[],"checks":[]}' RULE_PAGES='[[]]'
export CHECK_PAGES='[{"check_runs":[{"id":1,"name":"CI","status":"completed","conclusion":"success"}]}]' STATUS_BODY='{"statuses":[]}'
source "${HERE}/../lib/github.sh"
expect() {
 local want="$1" got="$2" name="$3"
 if [[ "${got}" != "${want}" ]]; then printf 'FAIL %s: wanted <%s>, got <%s>\n' "${name}" "${want}" "${got}" >&2; exit 1; fi
 printf 'ok - %s\n' "${name}"
}
expect '' "$(gh_required_contexts test/repo main)" '404 body never becomes a context'
expect green "$(gh_pr_checks_state test/repo 5)" '404 plus no rules uses observed green signals'
RULE_PAGES='[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"CI"}]}}]]'
CHECK_PAGES='[{"check_runs":[{"id":1,"name":"CI","status":"completed","conclusion":"success"},{"id":2,"name":"advisory","status":"in_progress","conclusion":null}]}]'
expect CI "$(gh_required_contexts test/repo main)" 'ruleset applies without classic protection'
expect green "$(gh_pr_checks_state test/repo 5)" 'pending advisory cannot block required green check'
RULE_PAGES='[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"CI"},{"context":"Missing"}]}}]]'
expect pending "$(gh_pr_checks_state test/repo 5)" 'unreported ruleset context still defers approval'
CHECK_PAGES='[{"check_runs":[{"id":1,"name":"CI","status":"completed","conclusion":"failure"}]}]'
expect failing "$(gh_pr_checks_state test/repo 5)" 'failing ruleset context dominates missing context'
CLASSIC_STATUS=200 CLASSIC_BODY='{"contexts":["Classic","CI"],"checks":[{"context":"Classic"}]}'
RULE_PAGES='[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"CI"}]}}],[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Later"}]}}]]'
expect $'Classic\nCI\nLater' "$(gh_required_contexts test/repo main)" 'classic and paginated ruleset contexts are unioned and deduplicated'
CLASSIC_STATUS=403 RULES_STATUS=403
expect '' "$(gh_required_contexts test/repo main)" 'permission error bodies are discarded from both sources'
expect failing "$(gh_pr_checks_state test/repo 5)" 'unreadable protection falls back to all signals'
CHECK_PAGES='[{"check_runs":[]}]'
expect pending "$(gh_pr_checks_state test/repo 5)" 'unreadable policy and no signals never grant approval'
CLASSIC_STATUS=200 CLASSIC_BODY='{"contexts":["CI"],"checks":[]}'
CHECK_PAGES='[{"check_runs":[{"id":1,"name":"CI","status":"completed","conclusion":"success"}]}]'
expect green "$(gh_pr_checks_state test/repo 5)" 'classic required checks work when rules API is unavailable'
