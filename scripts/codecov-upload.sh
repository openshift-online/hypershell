#!/usr/bin/env bash
# Upload one Go coverage profile to the App-SRE Codecov instance
# (the codecov-appsre GitHub App installed on openshift-online).
#
# Usage: scripts/codecov-upload.sh <flag> <coverage-file>
#
# Environment:
#   CODECOV_TOKEN           Upload token. When empty the upload is skipped
#                           (fork pull requests get no secrets) and the
#                           script exits 0 so unit tests still pass.
#   CODECOV_ENTERPRISE_URL  Base URL of the Codecov instance. Required when
#                           CODECOV_TOKEN is set.
#   CODECOV_COMMIT_SHA      Commit to attach coverage to (PR head on PRs).
#   CODECOV_BRANCH          Branch name; used for push uploads.
#   CODECOV_PR              Pull request number; empty for push uploads.
#   CODECOV_PARENT_SHA      PR base commit; used with CODECOV_PR.
#   CODECOV_BIN             Optional prebuilt CLI path (tests use a fake).
#
# Generated code is removed from the profile before the upload (see
# GENERATED_CODE_PATTERN), so Codecov never counts it, whatever configuration
# it applies on its side.
set -euo pipefail

CODECOV_VERSION="v11.3.1"
CODECOV_LINUX_SHA256="ca1d64196d2d34771084afe76ea657d581bf628e31d993ff8e52ea09cc88a56d"
SLUG="openshift-online/hypershell"

# Profile lines for generated files: protobuf and gateway stubs, mocks,
# zz_generated files, and the generated OpenAPI client. Keep in step with
# `ignore` in codecov.yml. Filtering here as well is needed because the App-SRE
# Codecov instance has processed reports without the repository's codecov.yml,
# so its `ignore` list did not apply and generated code made up about half of
# the counted lines.
GENERATED_CODE_PATTERN='(\.pb\.go|\.pb\.gw\.go|/mock_[^/]*\.go|_mock\.go|/zz_generated[^/]*\.go):|/components/api-server/pkg/api/openapi/'

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <flag> <coverage-file>" >&2
  exit 2
fi
flag="$1"
file="$2"

if [[ -z "${CODECOV_TOKEN:-}" ]]; then
  echo "::notice::CODECOV_TOKEN is not available; skipping Codecov upload for ${flag}."
  exit 0
fi
if [[ -z "${CODECOV_ENTERPRISE_URL:-}" ]]; then
  echo "error: CODECOV_ENTERPRISE_URL must be set when CODECOV_TOKEN is set" >&2
  exit 1
fi
if [[ ! -s "${file}" ]]; then
  echo "error: coverage file ${file} is missing or empty" >&2
  exit 1
fi

commit="${CODECOV_COMMIT_SHA:-$(git rev-parse HEAD)}"

# Upload a filtered copy; the original profile stays untouched for other tools.
filtered="$(mktemp)"
cleanup_files=("${filtered}")
trap 'rm -rf "${cleanup_files[@]}"' EXIT
# grep exits 1 when it keeps no line; only a real error (2) fails the upload.
grep -Ev -- "${GENERATED_CODE_PATTERN}" "${file}" > "${filtered}" || [[ $? -eq 1 ]]
removed=$(( $(wc -l < "${file}") - $(wc -l < "${filtered}") ))
echo "Excluded ${removed} generated-code blocks from ${file}"

bin="${CODECOV_BIN:-}"
if [[ -z "${bin}" ]]; then
  workdir="$(mktemp -d)"
  cleanup_files+=("${workdir}")
  bin="${workdir}/codecov"
  curl --fail --silent --show-error --location \
    --connect-timeout 10 --max-time 120 --retry 3 --retry-max-time 90 \
    --output "${bin}" "https://cli.codecov.io/${CODECOV_VERSION}/linux/codecov"
  echo "${CODECOV_LINUX_SHA256}  ${bin}" | sha256sum --check --status
  chmod +x "${bin}"
fi

args=(
  --enterprise-url "${CODECOV_ENTERPRISE_URL}"
  upload-process
  --fail-on-error
  --slug "${SLUG}"
  --git-service github
  --flag "${flag}"
  --file "${filtered}"
  --commit-sha "${commit}"
  --disable-search
)
if [[ -n "${CODECOV_PR:-}" ]]; then
  args+=(--pr "${CODECOV_PR}")
  if [[ -n "${CODECOV_PARENT_SHA:-}" ]]; then
    args+=(--parent-sha "${CODECOV_PARENT_SHA}")
  fi
elif [[ -n "${CODECOV_BRANCH:-}" ]]; then
  args+=(--branch "${CODECOV_BRANCH}")
fi

echo "Uploading ${file} to Codecov: flag=${flag} commit=${commit} pr=${CODECOV_PR:-none}"
CODECOV_TOKEN="${CODECOV_TOKEN}" "${bin}" "${args[@]}"
