#!/usr/bin/env bash
# Fetch hysh-aws-01/ci/cluster-login from AWS Secrets Manager and oc login.
# AWS credentials must already be in the environment (GitHub OIDC via
# aws-actions/configure-aws-credentials). Masks server and token before any
# later step can echo them (ephemeral-ci-secrets.spec.md).
set -euo pipefail

: "${PR_ENV_CLUSTER_LOGIN_SECRET:=hysh-aws-01/ci/cluster-login}"
: "${AWS_REGION:=us-east-1}"

if ! command -v aws >/dev/null 2>&1; then
  echo "ERROR: aws CLI is required to fetch ${PR_ENV_CLUSTER_LOGIN_SECRET}" >&2
  exit 1
fi
if ! command -v oc >/dev/null 2>&1; then
  echo "ERROR: oc is required for cluster login" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required to parse ${PR_ENV_CLUSTER_LOGIN_SECRET}" >&2
  exit 1
fi

payload="$(aws secretsmanager get-secret-value \
  --secret-id "${PR_ENV_CLUSTER_LOGIN_SECRET}" \
  --region "${AWS_REGION}" \
  --query SecretString \
  --output text)"

server="$(printf '%s' "${payload}" | jq -r '.server // empty')"
token="$(printf '%s' "${payload}" | jq -r '.token // empty')"
unset payload

if [[ -z "${server}" || -z "${token}" ]]; then
  echo "ERROR: ${PR_ENV_CLUSTER_LOGIN_SECRET} is missing JSON properties server and/or token" >&2
  exit 1
fi

if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
  echo "::add-mask::${server}"
  echo "::add-mask::${token}"
fi

if [[ -n "${KUBECONFIG:-}" ]]; then
  mkdir -p "$(dirname "${KUBECONFIG}")"
fi

oc login --server="${server}" --token="${token}" >/dev/null
echo "Logged in as $(oc whoami) at $(oc whoami --show-server)"
unset server token
