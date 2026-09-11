#!/usr/bin/env bash
# agent-lib.sh - reusable OpenShell sandbox orchestration library
#
# Source this file in your agent's run.sh:
#   source /opt/agent/agent-lib.sh
#
# Required environment variables (set via CronJob env or Secret):
#   GATEWAY_ENDPOINT   - gateway base URL (e.g. https://gateway.example.com)
#   GATEWAY_NAME       - gateway name registered in the workspace
#   OIDC_CLIENT_ID     - OIDC client identifier
#   OIDC_CLIENT_SECRET - OIDC client secret (mount from Secret)
#   OIDC_ISSUER_URL    - OIDC token endpoint base URL
#
# Optional:
#   OPENSHELL_BIN          - path to openshell binary (default: /tools/openshell)
#   TOKEN_REFRESH_INTERVAL - seconds between login refreshes (default: 300)
#   SANDBOX_TIMEOUT        - seconds to wait for sandbox ready (default: 120)
#   AGENT_LABEL            - label applied to sandboxes for stale cleanup (default: managed-by=agent)

set -euo pipefail

OPENSHELL_BIN="${OPENSHELL_BIN:-/tools/openshell}"
TOKEN_REFRESH_INTERVAL="${TOKEN_REFRESH_INTERVAL:-300}"
SANDBOX_TIMEOUT="${SANDBOX_TIMEOUT:-120}"
AGENT_LABEL="${AGENT_LABEL:-managed-by=agent}"

# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

_log() { echo "[$(date -u +%T)] $*" >&2; }
_die() { _log "ERROR: $*"; exit 1; }

_openshell() {
    local config_root="$1"; shift
    HOME="$config_root" \
    XDG_CONFIG_HOME="$config_root/.config" \
    XDG_STATE_HOME="$config_root/.local/state" \
        "$OPENSHELL_BIN" "$@"
}

# ---------------------------------------------------------------------------
# Gateway lifecycle
# ---------------------------------------------------------------------------

# configure_gateway <config_root>
# Registers the gateway endpoint + name in the isolated config root.
# Safe to call repeatedly - re-registers if already present.
configure_gateway() {
    local config_root="$1"
    _log "configuring gateway: $GATEWAY_NAME @ $GATEWAY_ENDPOINT"
    _openshell "$config_root" gateway add \
        --name    "$GATEWAY_NAME" \
        --url     "$GATEWAY_ENDPOINT" \
        --oidc-issuer     "$OIDC_ISSUER_URL" \
        --oidc-client-id  "$OIDC_CLIENT_ID" \
        --gateway-insecure false
}

# renew_gateway_login <config_root> [force]
# Authenticates (or re-authenticates) against the gateway using OIDC.
# Pass "force" as the second argument to skip the cache check.
renew_gateway_login() {
    local config_root="$1"
    local force="${2:-}"
    _log "authenticating with gateway $GATEWAY_NAME"
    OIDC_CLIENT_SECRET="$OIDC_CLIENT_SECRET" \
        _openshell "$config_root" gateway login \
            --name          "$GATEWAY_NAME" \
            --client-secret "$OIDC_CLIENT_SECRET" \
            ${force:+--force}
}

# _start_token_refresh_loop <config_root>
# Starts a background loop that renews the gateway login every
# TOKEN_REFRESH_INTERVAL seconds. Call once after initial login.
# Stores PID in REFRESH_PID for cleanup.
_start_token_refresh_loop() {
    local config_root="$1"
    (
        while true; do
            sleep "$TOKEN_REFRESH_INTERVAL"
            renew_gateway_login "$config_root" force || \
                _log "WARN token refresh failed; will retry in ${TOKEN_REFRESH_INTERVAL}s"
        done
    ) &
    REFRESH_PID=$!
    _log "token refresh loop started (pid $REFRESH_PID, interval ${TOKEN_REFRESH_INTERVAL}s)"
}

# ---------------------------------------------------------------------------
# Provider verification
# ---------------------------------------------------------------------------

# verify_providers <config_root> <provider> [provider...]
# Checks that every named provider exists in the workspace.
# Exits non-zero if any are missing - call before create_sandbox.
verify_providers() {
    local config_root="$1"; shift
    local missing=0
    for provider in "$@"; do
        if ! _openshell "$config_root" provider get "$provider" &>/dev/null; then
            _log "ERROR missing provider: $provider"
            missing=1
        else
            _log "provider verified: $provider"
        fi
    done
    [[ $missing -eq 0 ]] || _die "required providers are not registered in the workspace"
}

# ---------------------------------------------------------------------------
# Sandbox lifecycle
# ---------------------------------------------------------------------------

# create_sandbox <config_root> <sandbox_name> <sandbox_source>
#   [--provider <name>]... [--policy <path>]
#   [--upload <local:remote>]... [--label <k=v>]...
#
# Creates a sandbox and keeps it alive (--keep) for subsequent exec calls.
# Exits non-zero if creation fails.
create_sandbox() {
    local config_root="$1"
    local sandbox_name="$2"
    local sandbox_source="$3"
    shift 3

    _log "creating sandbox $sandbox_name (source: $sandbox_source)"
    _openshell "$config_root" sandbox create \
        --name   "$sandbox_name" \
        --source "$sandbox_source" \
        --keep \
        "$@"
}

# wait_for_sandbox <config_root> <sandbox_name> [timeout_seconds]
# Polls until the sandbox ready-file appears or timeout elapses.
# Exits non-zero on timeout.
wait_for_sandbox() {
    local config_root="$1"
    local sandbox_name="$2"
    local timeout="${3:-$SANDBOX_TIMEOUT}"
    local deadline=$(( $(date +%s) + timeout ))

    _log "waiting for sandbox $sandbox_name (timeout ${timeout}s)"
    while true; do
        local status
        status=$(_openshell "$config_root" sandbox get "$sandbox_name" \
                     --output jsonpath='{.status.phase}' 2>/dev/null || true)
        if [[ "$status" == "Running" ]]; then
            _log "sandbox $sandbox_name is ready"
            return 0
        fi
        if [[ $(date +%s) -ge $deadline ]]; then
            _log "ERROR sandbox $sandbox_name did not become ready within ${timeout}s (phase: ${status:-unknown})"
            return 1
        fi
        sleep 5
    done
}

# exec_in_sandbox <config_root> <sandbox_name> <timeout_seconds>
#   [--env KEY=VALUE]... -- <command> [args...]
#
# Runs a command inside the sandbox. Streams output to stderr.
# For long-running workloads, use exec_detached_in_sandbox instead.
exec_in_sandbox() {
    local config_root="$1"
    local sandbox_name="$2"
    local timeout="$3"
    shift 3

    _log "exec in sandbox $sandbox_name (timeout ${timeout}s): $*"
    _openshell "$config_root" sandbox exec \
        --name    "$sandbox_name" \
        --timeout "${timeout}s" \
        "$@"
}

# exec_detached_in_sandbox <config_root> <sandbox_name> <status_file>
#   <poll_interval> <timeout_seconds> [--env KEY=VALUE]... -- <command> [args...]
#
# Runs command detached (setsid --fork) so it survives exec timeouts.
# Polls <status_file> (written by the command) for completion.
# status_file must contain "done" or "error:<message>" when finished.
exec_detached_in_sandbox() {
    local config_root="$1"
    local sandbox_name="$2"
    local status_file="$3"
    local poll_interval="$4"
    local timeout="$5"
    shift 5

    _log "detached exec in sandbox $sandbox_name (timeout ${timeout}s)"

    # Launch detached - returns quickly once setsid forks
    _openshell "$config_root" sandbox exec \
        --name    "$sandbox_name" \
        --timeout "30s" \
        -- setsid --fork "$@" &

    # Poll status_file from outside
    local deadline=$(( $(date +%s) + timeout ))
    while true; do
        sleep "$poll_interval"
        local result
        result=$(_openshell "$config_root" sandbox exec \
                     --name "$sandbox_name" --timeout "10s" \
                     -- cat "$status_file" 2>/dev/null || true)
        case "$result" in
            done)
                _log "detached exec completed successfully"
                return 0
                ;;
            error:*)
                _die "detached exec failed: ${result#error:}"
                ;;
        esac
        if [[ $(date +%s) -ge $deadline ]]; then
            _die "detached exec timed out after ${timeout}s"
        fi
    done
}

# delete_sandbox <config_root> <sandbox_name>
# Deletes the sandbox. Tolerates not-found (idempotent).
delete_sandbox() {
    local config_root="$1"
    local sandbox_name="$2"
    _log "deleting sandbox $sandbox_name"
    _openshell "$config_root" sandbox delete --name "$sandbox_name" \
        2>/dev/null || _log "WARN sandbox $sandbox_name not found (already deleted?)"
}

# ---------------------------------------------------------------------------
# Stale sandbox cleanup
# ---------------------------------------------------------------------------

# cleanup_stale_sandboxes <config_root> <label_selector>
# Lists sandboxes matching the selector and deletes them.
# Call at the start of a CronJob run to remove leftovers from prior runs
# that were interrupted before their cleanup traps fired.
cleanup_stale_sandboxes() {
    local config_root="$1"
    local selector="$2"

    _log "cleaning up stale sandboxes matching: $selector"
    local names
    names=$(_openshell "$config_root" sandbox list \
                --selector "$selector" \
                --output jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
                2>/dev/null || true)

    if [[ -z "$names" ]]; then
        _log "no stale sandboxes found"
        return 0
    fi

    while IFS= read -r name; do
        [[ -n "$name" ]] || continue
        delete_sandbox "$config_root" "$name"
    done <<< "$names"
}

# ---------------------------------------------------------------------------
# Cleanup traps
# ---------------------------------------------------------------------------

# register_cleanup <config_root> <sandbox_name>
# Registers EXIT/INT/TERM traps that delete the sandbox and stop the
# token refresh loop. Call once after create_sandbox succeeds.
register_cleanup() {
    local config_root="$1"
    local sandbox_name="$2"

    _cleanup() {
        _log "cleanup triggered"
        delete_sandbox "$config_root" "$sandbox_name" || true
        if [[ -n "${REFRESH_PID:-}" ]]; then
            kill "$REFRESH_PID" 2>/dev/null || true
        fi
    }

    trap _cleanup EXIT INT TERM
}

# ---------------------------------------------------------------------------
# Isolated config root helpers
# ---------------------------------------------------------------------------

# new_config_root <base_dir> <sandbox_name>
# Creates a fresh, isolated HOME/config root under base_dir.
# Returns the path by printing it. Each sandbox gets its own root so
# parallel agents don't stomp each other's gateway state.
new_config_root() {
    local base_dir="$1"
    local sandbox_name="$2"
    local root="$base_dir/$sandbox_name"
    mkdir -p "$root/.config" "$root/.local/state"
    printf '%s' "$root"
}
