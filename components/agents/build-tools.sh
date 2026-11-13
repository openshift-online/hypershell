#!/usr/bin/env bash
# Compile the Go tools that the fullsend-sandbox image builds from source
# (gh and gitleaks), the same way the go-tools-builder stage of
# Containerfile.fullsend-sandbox does: only go.mod and go.sum are copied, and
# `go build -mod=mod` resolves the module graph from them.
#
# Why this exists: a dependency bump that breaks one of these tools is only
# visible when the image is built. A Renovate PR once merged with the Konflux
# image builds cancelled, and the break landed on main. This script gives the
# required Tests CI Gate a cheap way to catch that before merge.
#
# Usage: components/agents/build-tools.sh
# Requires a Go toolchain on PATH. GOTOOLCHAIN=local is forced so the build
# fails if the installed Go is older than the module needs, as in the image.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

export CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local

build_tool() {
  local name="$1" package="$2"
  shift 2
  local src="${root}/components/agents/tools/${name}"
  mkdir -p "${work}/${name}"
  cp "${src}/go.mod" "${src}/go.sum" "${work}/${name}/"
  echo "==> building ${name} (${package})"
  (cd "${work}/${name}" && go build -mod=mod -buildvcs=false "$@" -o "${work}/bin/${name}" "${package}")
}

build_tool gh github.com/cli/cli/v2/cmd/gh -ldflags="-s -w"
build_tool gitleaks github.com/zricethezav/gitleaks/v8 -ldflags="-s -w"

echo "OK: built $(ls "${work}/bin" | tr '\n' ' ')"
