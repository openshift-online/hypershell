#!/usr/bin/env bash
# Wait until a Kubernetes Secret exists and has every named data key.
# Does not print secret values. Fail-closed (ephemeral-ci-secrets.spec.md,
# ephemeral-test-credentials.spec.md).
#
# Usage: wait-for-secret-keys.sh <namespace> <secret-name> <key> [<key> ...]
set -euo pipefail

if [[ "$#" -lt 3 ]]; then
  echo "Usage: $0 <namespace> <secret-name> <key> [<key> ...]" >&2
  exit 1
fi

namespace="$1"
secret_name="$2"
shift 2
keys=("$@")

: "${PR_ENV_SECRET_WAIT_SECONDS:=300}"
: "${PR_ENV_KUBECTL:=oc}"

deadline=$((SECONDS + PR_ENV_SECRET_WAIT_SECONDS))
while (( SECONDS < deadline )); do
  missing=()
  if ! "${PR_ENV_KUBECTL}" get secret "${secret_name}" -n "${namespace}" >/dev/null 2>&1; then
    missing+=("(secret absent)")
  else
    for key in "${keys[@]}"; do
      encoded="$("${PR_ENV_KUBECTL}" get secret "${secret_name}" -n "${namespace}" \
        -o "jsonpath={.data.${key}}" 2>/dev/null || true)"
      if [[ -z "${encoded}" ]]; then
        missing+=("${key}")
      fi
    done
  fi
  if [[ "${#missing[@]}" -eq 0 ]]; then
    echo "Secret ${secret_name} in ${namespace} is Ready"
    exit 0
  fi
  sleep 5
done

echo "ERROR: Secret ${secret_name} in ${namespace} was not Ready within ${PR_ENV_SECRET_WAIT_SECONDS}s (missing: ${missing[*]})" >&2
exit 1
