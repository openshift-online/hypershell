#!/usr/bin/env bash
# Unit test for scripts/ci/reap-pr-environments.sh with a stubbed kubectl.
# No cluster required. Run: bash scripts/ci/reap-pr-environments_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

PASS=0
FAIL=0

now="$(pr_env_now_epoch)"
PAST="$(pr_env_epoch_to_rfc3339 $((now - 3600)))"
FUTURE="$(pr_env_epoch_to_rfc3339 $((now + 3600)))"

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT
DELETED="${workdir}/deleted.txt"
: > "${DELETED}"

# Canned namespace table the stub returns for `get namespaces`. Columns:
# name<TAB>owned<TAB>environment<TAB>expires-at. Covers: expired PR pair (reap
# both halves), active PR (future expiry, retain), local openshift-up env
# (uuid id, retain), and an unlabeled-ish foreign env.
cat > "${workdir}/rows.tsv" <<EOF
hypershell-ci-pr-232	true	pr-232	${PAST}
hypershell-ci-pr-232-keycloak	true	pr-232	${PAST}
hypershell-ci-pr-999	true	pr-999	${FUTURE}
hypershell-ci-pr-500	true	3f9a1c2e-uuid	${PAST}
some-dev-namespace	true	pr-1	${PAST}
EOF

# Stub kubectl: `get namespaces` -> canned rows; `delete namespace` -> record.
cat > "${workdir}/kubectl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
case "\$1 \$2" in
  "get namespaces")
    cat "${workdir}/rows.tsv"
    ;;
  "delete namespace")
    printf '%s\n' "\$3" >> "${DELETED}"
    ;;
  *)
    # delete clusterrolebinding/clusterrole and anything else: succeed quietly.
    exit 0
    ;;
esac
EOF
chmod +x "${workdir}/kubectl"

PR_ENV_KUBECTL="${workdir}/kubectl" bash "${SCRIPT_DIR}/reap-pr-environments.sh" >/dev/null

deleted_sorted="$(sort "${DELETED}" | tr '\n' ' ')"
expected='hypershell-ci-pr-232 hypershell-ci-pr-232-keycloak '
if [[ "${deleted_sorted}" == "${expected}" ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  printf 'FAIL: reaper deleted the wrong set (got=%q want=%q)\n' "${deleted_sorted}" "${expected}"
fi

# Dry-run must delete nothing.
: > "${DELETED}"
PR_ENV_KUBECTL="${workdir}/kubectl" PR_ENV_REAP_DRY_RUN=true bash "${SCRIPT_DIR}/reap-pr-environments.sh" >/dev/null
if [[ -s "${DELETED}" ]]; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: dry-run deleted namespaces'
else
  PASS=$((PASS + 1))
fi

printf 'reaper tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
