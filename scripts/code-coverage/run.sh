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
#   COVERAGE_CONSOLE_E2E   Also run the Playwright console suite (default: true)
#
# Steps:
#   1. Restart the instrumented pods with an empty coverage volume, so the
#      report covers this run only.
#   2. Go unit tests (api-server, control-plane, cli) with -coverprofile, and
#      frontend unit tests with vitest coverage over all source files.
#   3. make e2e and make e2e-console against the cluster.
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
: "${COVERAGE_CONSOLE_E2E:=true}"

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
      # Same environment as `make test`: tests start PostgreSQL with testcontainers.
      run_step "unit-go-api-server" in_dir components/api-server env API_ENV=integration_testing \
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

run_e2e_tests() {
  # -count=1: the suite tests live cluster state, so a cached result from an
  # earlier run would skip it and leave no e2e coverage.
  run_step "e2e-go" env GOFLAGS="-count=1 ${GOFLAGS:-}" make e2e
  if [[ "${COVERAGE_CONSOLE_E2E}" == "true" ]]; then
    run_step "e2e-console" make e2e-console
  fi
}

build_report() {
  header "Building coverage report"
  local raw="${COVERAGE_DIR}/e2e/raw"
  if compgen -G "${raw}/go/covmeta.*" >/dev/null; then
    go tool covdata textfmt -i="${raw}/go" -o="${COVERAGE_DIR}/e2e/go.out" ||
      FAILED+=("report-go-e2e")
  fi
  if compgen -G "${raw}/node/*.json" >/dev/null && [[ -d "${COVERAGE_DIR}/e2e/bff-dist" ]]; then
    node "${SCRIPT_DIR}/bff_report.mjs" "${REPO_ROOT}" "${COVERAGE_DIR}" ||
      FAILED+=("report-bff")
  fi
  python3 "${SCRIPT_DIR}/report.py" "${COVERAGE_DIR}" || FAILED+=("report-summary")
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
