#!/usr/bin/env bash
# Unit test for scripts/ci/teardown-pr-env.sh with a stubbed kubectl.
# No cluster required. Run: bash scripts/ci/teardown-pr-env_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PASS=0
FAIL=0

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT
DELETED="${workdir}/deleted.txt"
LIVE="${workdir}/live.txt"
FAIL_DELETE="${workdir}/fail_delete"
KEEP_LIVE="${workdir}/keep_live"
CALLS="${workdir}/calls.txt"
OUTPUT="${workdir}/output.txt"

write_stub() {
  cat > "${workdir}/kubectl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "\$*" >> "${CALLS}"
case "\$1 \$2" in
  "get namespace")
    if [[ "\$*" == *"-l"* ]]; then
      instance=""
      rest="\$*"
      rest="\${rest#*hypershell.redhat.io/instance=}"
      if [[ "\$rest" != "\$*" ]]; then
        instance="\${rest%%,*}"
        instance="\${instance%% *}"
      fi
      if [[ -n "\${instance}" ]]; then
        awk -F '\t' -v inst="\${instance}" '\$2 == inst { print \$1 }' "${workdir}/cp_managed.tsv" 2>/dev/null || true
      fi
      exit 0
    fi
    if [[ -f "${KEEP_LIVE}" ]] && grep -qx "\$3" "${KEEP_LIVE}"; then
      exit 0
    fi
    if grep -qx "\$3" "${DELETED}"; then
      echo "Error from server (NotFound): namespaces \"\$3\" not found" >&2
      exit 1
    fi
    if grep -qx "\$3" "${LIVE}"; then
      exit 0
    fi
    echo "Error from server (NotFound): namespaces \"\$3\" not found" >&2
    exit 1
    ;;
  "delete namespace"|"delete project")
    if [[ -f "${FAIL_DELETE}" ]] && grep -qx "\$3" "${FAIL_DELETE}"; then
      echo "Error from server (Conflict): namespace \$3 is stuck terminating" >&2
      exit 1
    fi
    printf '%s\n' "\$3" >> "${DELETED}"
    ;;
  *)
    exit 0
    ;;
esac
EOF
  chmod +x "${workdir}/kubectl"
  ln -sf "${workdir}/kubectl" "${workdir}/oc"
}

run_teardown() {
  OPENSHIFT_NAMESPACE="$1" PR_ENV_KUBECTL="${workdir}/oc" \
    PR_ENV_DELETE_TIMEOUT_SECONDS="${PR_ENV_DELETE_TIMEOUT_SECONDS:-300}" \
    PR_ENV_DELETE_POLL_SECONDS="${PR_ENV_DELETE_POLL_SECONDS:-2}" \
    PR_ENV_DELETE_STATUS_INTERVAL_SECONDS="${PR_ENV_DELETE_STATUS_INTERVAL_SECONDS:-30}" \
    bash "${SCRIPT_DIR}/teardown-pr-env.sh" >"${OUTPUT}" 2>&1
}

# Already-gone namespaces are success; no delete is issued.
: > "${DELETED}"
: > "${LIVE}"
: > "${CALLS}"
: > "${workdir}/cp_managed.tsv"
rm -f "${KEEP_LIVE}"
write_stub
if run_teardown hypershell-ci-pr-1; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: already-gone namespaces must succeed'
fi
if [[ -s "${DELETED}" ]]; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: already-gone path deleted namespaces'
else
  PASS=$((PASS + 1))
fi

# Successful deletes exit 0 and remove the group plus instance workloads.
: > "${DELETED}"
: > "${CALLS}"
cat > "${LIVE}" <<EOF
hypershell-ci-pr-1
hypershell-ci-pr-1-keycloak
openshell-aaa
EOF
cat > "${workdir}/cp_managed.tsv" <<EOF
openshell-aaa	hypershell-ci-pr-1
EOF
rm -f "${FAIL_DELETE}"
write_stub
if run_teardown hypershell-ci-pr-1; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: successful deletes must succeed'
fi
deleted_sorted="$(sort -u "${DELETED}" | tr '\n' ' ')"
expected='hypershell-ci-pr-1 hypershell-ci-pr-1-keycloak openshell-aaa '
if [[ "${deleted_sorted}" == "${expected}" ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  printf 'FAIL: teardown deleted the wrong set (got=%q want=%q)\n' "${deleted_sorted}" "${expected}"
fi
if [[ "$(wc -l <"${OUTPUT}" | tr -d ' ')" == "2" ]] &&
   ! grep -qE 'requested deletion of|waiting for namespace|deleted namespace|instance workload' "${OUTPUT}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: successful teardown must log only its start and completion summary'
  cat "${OUTPUT}"
fi

# A failed oc/kubectl delete must fail the script (fail closed).
: > "${DELETED}"
: > "${CALLS}"
cat > "${LIVE}" <<EOF
hypershell-ci-pr-1
hypershell-ci-pr-1-keycloak
EOF
: > "${workdir}/cp_managed.tsv"
printf '%s\n' 'hypershell-ci-pr-1' > "${FAIL_DELETE}"
write_stub
if run_teardown hypershell-ci-pr-1; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: a failed delete must fail the script'
else
  PASS=$((PASS + 1))
fi

# Tier ordering (CICD-PREN-02.08): the control-plane namespace group is deleted
# before the instance-managed gateway namespaces, so the instance namespace appears
# after the group in the deletion log.
: > "${DELETED}"
: > "${CALLS}"
cat > "${LIVE}" <<EOF
hypershell-ci-pr-1
hypershell-ci-pr-1-keycloak
openshell-aaa
EOF
cat > "${workdir}/cp_managed.tsv" <<EOF
openshell-aaa	hypershell-ci-pr-1
EOF
rm -f "${FAIL_DELETE}"
write_stub
if run_teardown hypershell-ci-pr-1; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: tiered teardown must succeed'
fi
if [[ "$(tail -n1 "${DELETED}")" == "openshell-aaa" ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: instance namespace must be deleted after the control-plane group; got order:'
  cat "${DELETED}"
fi

# Namespace deletion must be requested without oc's built-in wait. The live
# cluster can remove the Namespace while `oc delete project --wait=true` remains
# blocked until its 300-second timeout, so teardown polls the Namespace API
# directly instead.
if grep -q -- '--wait=true' "${CALLS}"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: namespace deletion used the stale oc --wait=true path'
  grep 'delete ' "${CALLS}"
elif grep -qE '^delete (project|namespace) .*--wait=false' "${CALLS}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: namespace deletion did not issue a nonblocking delete request'
fi

# A successful delete request is not completion. If the Namespace remains,
# teardown must fail at its single polling deadline instead of reporting success
# or issuing a second blocking delete.
: > "${DELETED}"
: > "${CALLS}"
cat > "${LIVE}" <<EOF
hypershell-ci-pr-1
EOF
: > "${workdir}/cp_managed.tsv"
printf '%s\n' 'hypershell-ci-pr-1' > "${KEEP_LIVE}"
write_stub
if PR_ENV_DELETE_TIMEOUT_SECONDS=0 PR_ENV_DELETE_POLL_SECONDS=1 run_teardown hypershell-ci-pr-1; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: teardown succeeded while the namespace remained live'
else
  PASS=$((PASS + 1))
fi
if grep -q 'waiting for namespace hypershell-ci-pr-1:' "${OUTPUT}" &&
   grep -q 'ERROR timed out after 0s waiting for namespace hypershell-ci-pr-1 deletion' "${OUTPUT}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: a teardown timeout must show the final Namespace state and an explicit error'
  cat "${OUTPUT}"
fi
delete_requests="$(grep -cE '^delete (project|namespace) hypershell-ci-pr-1( |$)' "${CALLS}" || true)"
if [[ "${delete_requests}" == "1" ]]; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo "FAIL: teardown issued ${delete_requests} delete requests after one successful request"
fi

# Polling and status intervals must remain positive so an override cannot turn
# a stuck Namespace wait into a tight API/logging loop.
if PR_ENV_DELETE_POLL_SECONDS=0 run_teardown hypershell-ci-pr-1; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: a zero polling interval must be rejected'
else
  PASS=$((PASS + 1))
fi
if PR_ENV_DELETE_STATUS_INTERVAL_SECONDS=0 run_teardown hypershell-ci-pr-1; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: a zero status interval must be rejected'
else
  PASS=$((PASS + 1))
fi

printf 'teardown-pr-env tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
