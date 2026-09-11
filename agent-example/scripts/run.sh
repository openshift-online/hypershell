#!/usr/bin/env bash
# run.sh - example agent entrypoint
#
# This script demonstrates how to use agent-lib.sh to build a sandbox-based
# agent. It:
#   1. Cleans up stale sandboxes from prior runs
#   2. Verifies required providers are registered
#   3. Creates a sandbox, waits for it, and runs a workload inside it
#   4. Cleans up on exit (via trap registered in register_cleanup)
#
# Replace the "workload" section with your actual task.

set -euo pipefail

source /opt/agent/agent-lib.sh

# ---------------------------------------------------------------------------
# Configuration - override defaults via CronJob env
# ---------------------------------------------------------------------------
SANDBOX_NAME="${SANDBOX_NAME:-example-agent-run}"
SANDBOX_SOURCE="${SANDBOX_SOURCE:-ubi9}"       # workspace sandbox template name
PROVIDER="${PROVIDER:-my-service-provider}"    # provider registered in workspace
WORK_TIMEOUT="${WORK_TIMEOUT:-600}"            # seconds for the workload to finish

# ---------------------------------------------------------------------------
# Setup: isolated config root for this run
# ---------------------------------------------------------------------------
CONFIG_ROOT=$(new_config_root /work "$SANDBOX_NAME")

# ---------------------------------------------------------------------------
# Step 1: authenticate with the gateway
# ---------------------------------------------------------------------------
configure_gateway "$CONFIG_ROOT"
renew_gateway_login "$CONFIG_ROOT"
_start_token_refresh_loop "$CONFIG_ROOT"

# ---------------------------------------------------------------------------
# Step 2: clean up any sandboxes left by previous crashed runs
# ---------------------------------------------------------------------------
cleanup_stale_sandboxes "$CONFIG_ROOT" "managed-by=example-agent"

# ---------------------------------------------------------------------------
# Step 3: verify providers before creating the sandbox
# ---------------------------------------------------------------------------
verify_providers "$CONFIG_ROOT" "$PROVIDER"

# ---------------------------------------------------------------------------
# Step 4: create the sandbox and register cleanup trap
# ---------------------------------------------------------------------------
create_sandbox "$CONFIG_ROOT" "$SANDBOX_NAME" "$SANDBOX_SOURCE" \
    --provider "$PROVIDER" \
    --policy   /etc/agent/policy.yaml \
    --upload   "/opt/agent/run.sh:/tmp/run.sh" \
    --label    "managed-by=example-agent"

register_cleanup "$CONFIG_ROOT" "$SANDBOX_NAME"

wait_for_sandbox "$CONFIG_ROOT" "$SANDBOX_NAME"

# ---------------------------------------------------------------------------
# Step 5: run the workload inside the sandbox
# ---------------------------------------------------------------------------
_log "running workload in sandbox $SANDBOX_NAME"

exec_in_sandbox "$CONFIG_ROOT" "$SANDBOX_NAME" "$WORK_TIMEOUT" \
    --env MY_SERVICE_URL="$MY_SERVICE_URL" \
    -- /bin/sh -c '
        set -eu
        echo "hello from inside the sandbox"
        echo "openshell version: $(openshell version 2>/dev/null || echo n/a)"
        echo "MY_SERVICE_URL=$MY_SERVICE_URL"
        # TODO: replace with your actual workload
        echo "workload complete"
    '

_log "example agent finished successfully"
