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

write_stub() {
  cat > "${workdir}/kubectl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
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
    if grep -qx "\$3" "${LIVE}"; then
      exit 0
    fi
    exit 1
    ;;
  "delete namespace")
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
}

run_teardown() {
  OPENSHIFT_NAMESPACE="$1" PR_ENV_KUBECTL="${workdir}/kubectl" \
    bash "${SCRIPT_DIR}/teardown-pr-env.sh" >/dev/null
}

# Already-gone namespaces are success; no delete is issued.
: > "${DELETED}"
: > "${LIVE}"
: > "${workdir}/cp_managed.tsv"
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

# A failed oc/kubectl delete must fail the script (fail closed).
: > "${DELETED}"
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

printf 'teardown-pr-env tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]]
