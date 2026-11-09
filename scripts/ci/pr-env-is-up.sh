#!/usr/bin/env bash
# pr-env-is-up.sh - report whether the OpenShift PR environment namespace is
# currently live.
#
# A "re-run failed jobs" on E2E / OpenShift does not re-run the deploy job
# once it has succeeded, but a prior e2e-openshift attempt's own teardown
# step may have already destroyed the environment on its way out
# (ephemeral-pr-environments.spec.md). e2e-openshift uses this to decide
# whether to redeploy before running the suite instead of failing closed on
# a missing namespace.
#
# Environment:
#   OPENSHIFT_NAMESPACE  platform namespace to check (required)
#   PR_ENV_KUBECTL       kubectl/oc binary (default: oc)
set -euo pipefail

: "${OPENSHIFT_NAMESPACE:?OPENSHIFT_NAMESPACE is required}"
KUBECTL="${PR_ENV_KUBECTL:-oc}"

if "${KUBECTL}" get namespace "${OPENSHIFT_NAMESPACE}" >/dev/null 2>&1; then
  echo "Namespace ${OPENSHIFT_NAMESPACE} exists"
  exit 0
fi

echo "Namespace ${OPENSHIFT_NAMESPACE} does not exist"
exit 1
