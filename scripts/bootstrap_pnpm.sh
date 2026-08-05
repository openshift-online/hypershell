#!/usr/bin/env bash

set -euo pipefail

readonly PNPM_BOOTSTRAP_VERSION="11.15.1"
readonly PNPM_BOOTSTRAP_SHA256="27460629b10111604e7f98882753b53398986820c20e0a065f3a4a5e9e7db71f"
readonly PNPM_BOOTSTRAP_URL="https://registry.npmjs.org/pnpm/-/pnpm-${PNPM_BOOTSTRAP_VERSION}.tgz"

bootstrap_directory="$(mktemp -d)"
trap 'rm -rf -- "${bootstrap_directory}"' EXIT

curl --fail --silent --show-error --location \
  --proto '=https' \
  --tlsv1.2 \
  --output "${bootstrap_directory}/pnpm.tgz" \
  "${PNPM_BOOTSTRAP_URL}"

printf '%s  %s\n' "${PNPM_BOOTSTRAP_SHA256}" "${bootstrap_directory}/pnpm.tgz" | sha256sum --check --strict
npm install --global "${bootstrap_directory}/pnpm.tgz" --ignore-scripts
test "$(pnpm --version)" = "${PNPM_BOOTSTRAP_VERSION}"
