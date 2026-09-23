#!/usr/bin/env bash
# Unit tests for wait-for-secret-keys.sh and login-openshift-from-aws.sh.
# Also locks the workflow contract: no standing Actions secrets, Tests / E2E
# grants id-token: write, reusable e2e.yml inherits it
# (ephemeral-ci-secrets.spec.md).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "${label}" "${want}" "${got}"
  fi
}

assert_ok() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected success)\n' "${label}"
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected failure)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

# --- wait-for-secret-keys.sh ---
cat >"${WORKDIR}/oc-ready" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == get && "$2" == secret ]]; then
  if [[ "$*" == *jsonpath* ]]; then
    printf '%s' 'Zg=='
  fi
  exit 0
fi
exit 1
EOF
chmod +x "${WORKDIR}/oc-ready"
assert_ok "wait succeeds when secret keys exist" \
  env PR_ENV_KUBECTL="${WORKDIR}/oc-ready" PR_ENV_SECRET_WAIT_SECONDS=1 \
  bash "${SCRIPT_DIR}/wait-for-secret-keys.sh" ns hypershell-github-oauth client_id

cat >"${WORKDIR}/oc-missing" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${WORKDIR}/oc-missing"
assert_fail "wait fails closed when the secret is absent" \
  env PR_ENV_KUBECTL="${WORKDIR}/oc-missing" PR_ENV_SECRET_WAIT_SECONDS=1 \
  bash "${SCRIPT_DIR}/wait-for-secret-keys.sh" ns hypershell-github-oauth client_id

# --- login-openshift-from-aws.sh ---
mkdir -p "${WORKDIR}/bin"
cat >"${WORKDIR}/bin/aws" <<'EOF'
#!/usr/bin/env bash
printf '%s' '{"server":"https://api.example.com:6443","token":"sha-cluster-token"}'
EOF
cat >"${WORKDIR}/bin/oc" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == login ]]; then
  printf '%s\n' "$*" >"${LOGIN_CAPTURE}"
  exit 0
fi
if [[ "$1" == whoami ]]; then
  if [[ "${2:-}" == --show-server ]]; then
    printf '%s' 'https://api.example.com:6443'
  else
    printf '%s' 'ci-sa'
  fi
  exit 0
fi
exit 1
EOF
chmod +x "${WORKDIR}/bin/aws" "${WORKDIR}/bin/oc"
LOGIN_CAPTURE="${WORKDIR}/oc-login"
login_out="${WORKDIR}/login.out"
if PATH="${WORKDIR}/bin:${PATH}" LOGIN_CAPTURE="${LOGIN_CAPTURE}" \
  GITHUB_ACTIONS=true \
  bash "${SCRIPT_DIR}/login-openshift-from-aws.sh" >"${login_out}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: cluster login fetches AWS JSON and oc logs in'
fi
if grep -q 'api.example.com:6443' "${LOGIN_CAPTURE}" && grep -q 'sha-cluster-token' "${LOGIN_CAPTURE}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: oc login did not receive server and token'
fi
if grep -q '::add-mask::https://api.example.com:6443' "${login_out}" \
  && grep -q '::add-mask::sha-cluster-token' "${login_out}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: cluster login did not mask server and token'
fi

cat >"${WORKDIR}/bin/aws" <<'EOF'
#!/usr/bin/env bash
printf '%s' '{"server":"https://api.example.com:6443"}'
EOF
chmod +x "${WORKDIR}/bin/aws"
assert_fail "cluster login fails closed without token" \
  env PATH="${WORKDIR}/bin:${PATH}" LOGIN_CAPTURE="${LOGIN_CAPTURE}" \
  bash "${SCRIPT_DIR}/login-openshift-from-aws.sh"

# --- workflow contract ---
if grep -R --include='*.yml' --include='*.yaml' -E 'secrets\.(OPENSHIFT_PR_ENV_|PR_ENV_GITHUB_OAUTH_)' \
  "${REPO_ROOT}/.github" >/dev/null; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: workflows still read standing cluster or OAuth Actions secrets'
else
  PASS=$((PASS + 1))
fi

if grep -A20 'name: E2E$' "${REPO_ROOT}/.github/workflows/tests.yml" | grep -q 'id-token: write'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: Tests / E2E caller does not set id-token: write'
fi

if grep -q '^  e2e-fork:' "${REPO_ROOT}/.github/workflows/tests.yml"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: Tests.yml still has a second E2E fork caller'
else
  PASS=$((PASS + 1))
fi

if grep -A8 'Refuse fork pull requests' \
  "${REPO_ROOT}/.github/actions/openshift-cluster-login/action.yml" \
  | grep -q 'head.repo.full_name != github.repository'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: cluster-login action does not refuse fork pull_request jobs'
fi

if grep -q 'role/hypershell-ci-hysh-aws-01-cluster-login' \
  "${REPO_ROOT}/.github/workflows/e2e.yml" \
  "${REPO_ROOT}/.github/workflows/pr-environment-destroy.yml" \
  "${REPO_ROOT}/.github/workflows/pr-environment-commands.yml"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: PR-environment workflows do not default PR_ENV_AWS_ROLE_ARN'
fi

if grep -q 'id-token: write' "${REPO_ROOT}/.github/workflows/pr-environment-destroy.yml" \
  && grep -q 'id-token: write' "${REPO_ROOT}/.github/workflows/pr-environment-commands.yml"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: destroy/command jobs do not set id-token: write'
fi

# Declaring a workflow-level permissions block on reusable e2e.yml that
# omits id-token strips the caller's OIDC grant; including it would mint a
# token for Kind. Job-level grants keep OIDC on OpenShift jobs only.
if grep -E '^permissions:' "${REPO_ROOT}/.github/workflows/e2e.yml"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: reusable e2e.yml must not declare workflow-level permissions'
else
  PASS=$((PASS + 1))
fi

job_header() {
  local job="$1"
  awk -v job="${job}" '
    $0 ~ "^  " job ":" { p=1; next }
    p && /^    steps:/ { exit }
    p { print }
  ' "${REPO_ROOT}/.github/workflows/e2e.yml"
}

if job_header e2e-kind | grep -q 'id-token: write'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: Kind job must omit id-token: write'
else
  PASS=$((PASS + 1))
fi

if job_header plan-images | grep -q 'id-token: write'; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: plan-images job must omit id-token: write'
else
  PASS=$((PASS + 1))
fi

if job_header deploy | grep -q 'id-token: write' \
  && job_header e2e-openshift | grep -q 'id-token: write'; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: deploy and OpenShift jobs must set id-token: write'
fi

printf 'ephemeral-ci-secrets tests: %d passed, %d failed\n' "${PASS}" "${FAIL}"
[[ "${FAIL}" -eq 0 ]]
