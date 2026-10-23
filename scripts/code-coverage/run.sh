#!/usr/bin/env bash
# Run every test layer and build one combined coverage report (make coverage).
# Spec: specs/platform/local-development.spec.md DEV-LOC-18.
#
# Usage:
#   scripts/code-coverage/run.sh          # unit tests + e2e on Kind + report
#   scripts/code-coverage/run.sh report   # rebuild the report from existing data
#
# The e2e part needs a Kind cluster deployed with KIND_COVERAGE=true make kind-up.
#
# Environment:
#   COVERAGE_DIR           Output directory (default: <repo>/coverage)
#   COVERAGE_UNIT          Run Go and frontend unit tests (default: true)
#   COVERAGE_E2E           Run the Go e2e suite and collect cluster data (default: true)
#   COVERAGE_HSCTL_E2E     Also run the hsctl smoke test with a -cover hsctl (default: true)
#   COVERAGE_CONSOLE_E2E   Also run the Playwright console suite (default: true)
#   COVERAGE_CONSOLE_CONTAINER  Run that suite in the Playwright image on the Kind
#                          network: auto (default: when the network exists), true, false
#
# Steps:
#   1. Restart the instrumented pods with an empty coverage volume, so the
#      report covers this run only.
#   2. Go unit tests (api-server, control-plane, cli) with -coverprofile, and
#      frontend unit tests with vitest coverage over all source files.
#   3. make e2e, the hsctl smoke test (tests/e2e/e2e-hsctl.sh) with a
#      coverage-instrumented hsctl, and make e2e-console against the cluster.
#   4. Stop the pods (Go and Node write coverage on a clean exit), copy the
#      data off the Kind node, and start the pods again.
#   5. Convert and merge everything into coverage/report/.
# A failing test step does not stop the run: the report is still built, and the
# script exits non-zero at the end.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../kind/lib.sh
source "${SCRIPT_DIR}/../kind/lib.sh"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"
set +e

: "${COVERAGE_DIR:=${REPO_ROOT}/coverage}"
: "${COVERAGE_UNIT:=true}"
: "${COVERAGE_E2E:=true}"
: "${COVERAGE_HSCTL_E2E:=true}"
: "${COVERAGE_CONSOLE_E2E:=true}"
: "${COVERAGE_CONSOLE_CONTAINER:=auto}"

FAILED=()

# run_step NAME COMMAND...: run COMMAND from the repo root, streaming its output
# to the terminal and to logs/NAME.log. A failure is recorded, not fatal.
run_step() {
  local name="$1"
  shift
  header "${name}"
  (cd "${REPO_ROOT}" && "$@") 2>&1 | tee "${COVERAGE_DIR}/logs/${name}.log"
  local status="${PIPESTATUS[0]}"
  if [[ "${status}" -ne 0 ]]; then
    FAILED+=("${name}")
    warn "${name} failed (exit ${status}); see ${COVERAGE_DIR}/logs/${name}.log"
  fi
  echo ""
  return "${status}"
}

# in_dir DIR COMMAND...: run COMMAND inside DIR (relative to the repo root).
in_dir() {
  local dir="$1"
  shift
  cd "${dir}" && "$@"
}

go_unit() {
  local component="$1"
  local profile="${COVERAGE_DIR}/unit/go/${component}.out"
  case "${component}" in
    api-server)
      # Same environment as `make test`: tests start PostgreSQL with
      # testcontainers. On a podman-only machine, point it at podman's socket.
      local tc_env=()
      read -r -a tc_env <<<"$(testcontainers_env)"
      if [[ ${#tc_env[@]} -gt 0 ]]; then
        info "No Docker socket; running testcontainers through podman (${tc_env[*]})"
      fi
      run_step "unit-go-api-server" in_dir components/api-server \
        env ${tc_env[@]+"${tc_env[@]}"} API_ENV=integration_testing \
        go test -p 1 -count=1 -covermode=atomic -coverprofile="${profile}" ./...
      ;;
    *)
      run_step "unit-go-${component}" in_dir "components/${component}" \
        go test -count=1 -covermode=atomic -coverprofile="${profile}" ./...
      ;;
  esac
}

# frontend_unit NAME DIR INCLUDE: vitest with coverage over every source file
# matching INCLUDE (not only the files the package's CI gate lists), no thresholds.
frontend_unit() {
  local name="$1" dir="$2" include="$3"
  run_step "unit-frontend-${name}" in_dir "${dir}" pnpm exec vitest run --coverage \
    --coverage.provider=v8 \
    --coverage.reporter=json --coverage.reporter=json-summary \
    --coverage.reportsDirectory="${COVERAGE_DIR}/unit/frontend/${name}" \
    --coverage.include="${include}" \
    --coverage.exclude='**/*.test.*' --coverage.exclude='**/*.stories.tsx' \
    --coverage.thresholds.lines=0 --coverage.thresholds.branches=0 \
    --coverage.thresholds.functions=0 --coverage.thresholds.statements=0
}

run_unit_tests() {
  go_unit api-server
  go_unit control-plane
  go_unit cli

  # Node 25+ ships its own localStorage global, which hides jsdom's copy and
  # breaks tests that pass on the pinned Node 24 (.node-version).
  if [[ "$(node -p 'process.versions.node.split(".")[0]')" -ge 25 ]]; then
    export NODE_OPTIONS="--no-experimental-webstorage ${NODE_OPTIONS:-}"
  fi
  run_step "frontend-deps" bash -c \
    "pnpm --filter @openshift-online/hypershell-sdk build && pnpm --filter @openshift-online/hypershell-domain-probes build"
  frontend_unit web-console components/web-console 'app/**/*.{ts,tsx}'
  frontend_unit bff components/web-console/bff 'src/**/*.ts'
  frontend_unit domain-probes components/web-console/domain-probes 'src/**/*.ts'
  frontend_unit gateway-management-ui packages/gateway-management-ui 'src/**/*.{ts,tsx}'
  frontend_unit operational-dashboard-ui packages/operational-dashboard-ui 'src/**/*.{ts,tsx}'
}

# run_hsctl_e2e: hsctl runs on this machine, not in the cluster, so its coverage
# goes to a local GOCOVERDIR instead of the cluster volume. The smoke test keeps
# its hsctl config in a temporary file, so the user's own login is untouched.
run_hsctl_e2e() {
  local bin="${COVERAGE_DIR}/e2e/hsctl-bin/hsctl"
  local data="${COVERAGE_DIR}/e2e/raw/hsctl"
  mkdir -p "$(dirname "${bin}")" "${data}"
  run_step "build-hsctl-cover" in_dir components/cli env CGO_ENABLED=0 \
    go build -cover -covermode=atomic \
    -coverpkg=github.com/openshift-online/hypershell/components/cli/... \
    -o "${bin}" ./cmd/hsctl
  if [[ ! -x "${bin}" ]]; then
    return
  fi
  run_step "e2e-hsctl" env HSCTL="${bin}" GOCOVERDIR="${data}" bash tests/e2e/e2e-hsctl.sh
}

# console_browser_ready: install Playwright's Chromium (as CI does), then start
# it once. `playwright install` only warns when the host lacks Chromium's system
# libraries, so the launch is what proves the console suite can run.
console_browser_ready() {
  local pkg=@openshift-online/hypershell-web-console
  pnpm --filter "${pkg}" exec playwright install chromium &&
    pnpm --filter "${pkg}" exec node -e \
      'require("@playwright/test").chromium.launch().then((b) => b.close())'
}

# console_in_container: whether to run the console suite in the Playwright
# container (tests/e2e/e2e-console-container.sh). auto picks it when the Kind
# nodes' container network exists.
console_in_container() {
  case "${COVERAGE_CONSOLE_CONTAINER}" in
    true) return 0 ;;
    false) return 1 ;;
  esac
  ${CONTAINER_ENGINE} network inspect "${E2E_CONSOLE_KIND_NETWORK:-kind}" >/dev/null 2>&1
}

run_e2e_tests() {
  # -count=1: the suite tests live cluster state, so a cached result from an
  # earlier run would skip it and leave no e2e coverage.
  run_step "e2e-go" env GOFLAGS="-count=1 ${GOFLAGS:-}" make e2e
  if [[ "${COVERAGE_HSCTL_E2E}" == "true" ]]; then
    run_hsctl_e2e
  fi
  if [[ "${COVERAGE_CONSOLE_E2E}" == "true" ]]; then
    # CI runs the console suite in short mode; match it unless E2E_MODE is set.
    local console_mode="${E2E_MODE:-short}"
    if console_in_container; then
      # The Playwright image brings Chromium and its libraries, and the Kind
      # network reaches the gateway on 443: no host port forwarding needed.
      run_step "e2e-console" env E2E_MODE="${console_mode}" E2E_CONSOLE_CONTAINER=true make e2e-console
    elif run_step "e2e-console-browser" console_browser_ready; then
      # Same browser setup CI runs before the console suite. A Linux host may
      # also lack Chromium's system libraries; Playwright's message in this
      # step's log names the command to install them.
      run_step "e2e-console" env E2E_MODE="${console_mode}" make e2e-console
    else
      warn "Skipping e2e-console: Chromium is not usable (see e2e-console-browser.log)"
    fi
  fi
}

build_report() {
  header "Building coverage report"
  local raw="${COVERAGE_DIR}/e2e/raw"
  # Cluster services (raw/go) and hsctl (raw/hsctl) merge into one e2e profile.
  local dir go_dirs=()
  for dir in "${raw}/go" "${raw}/hsctl"; do
    if compgen -G "${dir}/covmeta.*" >/dev/null; then
      go_dirs+=("${dir}")
    fi
  done
  if [[ ${#go_dirs[@]} -gt 0 ]]; then
    local inputs
    inputs="$(IFS=,; echo "${go_dirs[*]}")"
    go tool covdata textfmt -i="${inputs}" -o="${COVERAGE_DIR}/e2e/go.out" ||
      FAILED+=("report-go-e2e")
  fi
  if compgen -G "${raw}/node/*.json" >/dev/null && [[ -d "${COVERAGE_DIR}/e2e/bff-dist" ]]; then
    node "${SCRIPT_DIR}/bff_report.mjs" "${REPO_ROOT}" "${COVERAGE_DIR}" ||
      FAILED+=("report-bff")
  fi
  python3 "${SCRIPT_DIR}/report.py" "${COVERAGE_DIR}" --failed-steps "${FAILED[*]-}" ||
    FAILED+=("report-summary")
  local component
  for component in api-server control-plane cli; do
    [[ -f "${COVERAGE_DIR}/report/go/${component}.out" ]] || continue
    (cd "${REPO_ROOT}/components/${component}" &&
      go tool cover -html="${COVERAGE_DIR}/report/go/${component}.out" \
        -o "${COVERAGE_DIR}/report/go/${component}.html") ||
      FAILED+=("report-html-${component}")
  done
}

finish() {
  info "Summary:      ${COVERAGE_DIR}/report/summary.md"
  info "Go HTML:      ${COVERAGE_DIR}/report/go/<component>.html"
  info "BFF e2e HTML: ${COVERAGE_DIR}/report/bff-e2e/index.html"
  if [[ ${#FAILED[@]} -gt 0 ]]; then
    error "Failed steps: ${FAILED[*]}"
    exit 1
  fi
  success "All coverage steps passed"
}

main() {
  if [[ "${1:-}" == "report" ]]; then
    mkdir -p "${COVERAGE_DIR}/logs"
    build_report
    finish
    return
  fi

  if [[ "${COVERAGE_E2E}" == "true" ]]; then
    require_coverage_cluster || exit 1
  fi
  rm -rf "${COVERAGE_DIR}/unit" "${COVERAGE_DIR}/e2e" "${COVERAGE_DIR}/report" "${COVERAGE_DIR}/logs"
  mkdir -p "${COVERAGE_DIR}/unit/go" "${COVERAGE_DIR}/unit/frontend" \
    "${COVERAGE_DIR}/e2e/raw" "${COVERAGE_DIR}/report" "${COVERAGE_DIR}/logs"

  if [[ "${COVERAGE_E2E}" == "true" ]]; then
    header "Resetting cluster coverage data"
    reset_cluster_coverage || exit 1
    echo ""
  fi
  if [[ "${COVERAGE_UNIT}" == "true" ]]; then
    run_unit_tests
  fi
  if [[ "${COVERAGE_E2E}" == "true" ]]; then
    run_e2e_tests
    header "Collecting cluster coverage data"
    collect_cluster_coverage "${COVERAGE_DIR}/e2e/raw" || FAILED+=("collect-cluster")
    extract_bff_dist "${COVERAGE_DIR}/e2e/bff-dist" || FAILED+=("collect-bff-dist")
    echo ""
  fi
  build_report
  finish
}

main "$@"
