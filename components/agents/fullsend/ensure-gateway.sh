#!/usr/bin/env bash
# ensure-gateway.sh - PLATFORM layer (script 1 of 2).
#
# Idempotently ensure a reusable OpenShell gateway exists on hyp0, authenticate
# to it, and provision the workspace + providers. Writes a gateway env file that
# fullsend-run.sh (script 2) sources.
#
# "Maybe it exists already, maybe it needs creating": we find-or-create a gateway
# by a STABLE name (default: fullsend-shared) via the HyperShell REST API, then
# mint a fresh service-account identity on it for this session.
#
# Reuses run.sh's proven helpers (sourced from the deployed coordinator script) so
# we don't re-implement gateway/SA/workspace provisioning.
#
# Inputs (env; most come from the AgentRuntime/cronjob env already):
#   FULLSEND_GATEWAY_NAME  stable gateway name to find-or-create (default fullsend-shared)
#   GW_ENV_FILE            where to write the gateway env (default /tmp/gw.env)
#   RUN_SH_PATH            path to the coordinator run.sh (default /opt/sdlc-coordinator/run.sh)
#   CONFIG_ROOT            openshell config dir to use (default /tmp/fs-run-config)
#   + GATEWAY_CLUSTER_ID GATEWAY_IMAGE GATEWAY_SUPERVISOR_IMAGE API_SERVER_URL
#     OPENSHELL_WORKSPACE PROVISIONER_* (from the pod env)
set -uo pipefail

FULLSEND_GATEWAY_NAME="${FULLSEND_GATEWAY_NAME:-fullsend-shared}"
GW_ENV_FILE="${GW_ENV_FILE:-/tmp/gw.env}"
RUN_SH_PATH="${RUN_SH_PATH:-/opt/sdlc-coordinator/run.sh}"
CONFIG_ROOT="${CONFIG_ROOT:-/tmp/fs-run-config}"
GATEWAY_PROVISION_TIMEOUT_SECONDS="${GATEWAY_PROVISION_TIMEOUT_SECONDS:-300}"

# Pull in run.sh's helpers + globals (functions only; the file is unguarded so we
# take everything before the '# --- main' marker). Also sources agent-runtime.sh.
main_line="$(grep -n '^# --- main' "$RUN_SH_PATH" | head -1 | cut -d: -f1)"
sed -n "1,$((main_line - 1))p" "$RUN_SH_PATH" > /tmp/gwlib.sh
# shellcheck disable=SC1091
source /tmp/gwlib.sh

log "ensure-gateway: target name '$FULLSEND_GATEWAY_NAME' on cluster $GATEWAY_CLUSTER_ID."
hsctl_login

# 1. Find an existing Running gateway with our stable name.
gateway_id="$(curl -sf -H "Authorization: Bearer $api_token" \
  "$API_SERVER_URL/api/hypershell/v1/gateways?size=200" 2>/dev/null \
  | jq -r --arg n "$FULLSEND_GATEWAY_NAME" \
      '[(.items // .gateways // [])[] | select(.name==$n and .phase=="Running")][0].id // empty' || true)"

if [[ -n "$gateway_id" ]]; then
  log "ensure-gateway: reusing existing gateway $gateway_id ($FULLSEND_GATEWAY_NAME)."
else
  # 2. Create it with the stable name (mirrors create_gateway, fixed name).
  log "ensure-gateway: no Running gateway named '$FULLSEND_GATEWAY_NAME'; creating one."
  body="$(jq -n --arg n "$FULLSEND_GATEWAY_NAME" --arg c "$GATEWAY_CLUSTER_ID" \
    --arg img "$GATEWAY_IMAGE" --arg sup "$GATEWAY_SUPERVISOR_IMAGE" --arg route '{"enabled":true}' \
    '{name:$n, cluster_id:$c, image:$img, supervisor_image:$sup, route:$route}')"
  gateway_id="$(printf '%s' "$body" | hsctl create gateway --body /dev/stdin 2>&1 | jq -r '.id // empty' || true)"
  [[ -n "$gateway_id" ]] || { echo "ensure-gateway: failed to create gateway" >&2; exit 1; }
  deadline=$((SECONDS + GATEWAY_PROVISION_TIMEOUT_SECONDS))
  while (( SECONDS < deadline )); do
    phase="$(hsctl get gateway "$gateway_id" 2>/dev/null | jq -r '.phase // empty' || true)"
    [[ "$phase" == Running ]] && break
    sleep 5
  done
  [[ "$phase" == Running ]] || { echo "ensure-gateway: gateway $gateway_id not Running in time" >&2; exit 1; }
  log "ensure-gateway: created gateway $gateway_id (Running)."
fi

# 3. Mint a fresh session identity (SA) on the gateway and register + log in.
#    create_gateway_sa reads $gateway_id and exports GATEWAY_ENDPOINT/NAME/OIDC_*/
#    OPENSHELL_OIDC_CLIENT_SECRET. (TODO: revoke stale SAs on a persistent gateway.)
create_gateway_sa
configure_gateway "$CONFIG_ROOT"
login_gateway "$CONFIG_ROOT"
provision_workspace_providers "$CONFIG_ROOT"
log "ensure-gateway: authenticated + workspace/providers ready."

# 4. Emit the gateway env for script 2. Secret-bearing → mode 0600.
umask 077
{
  echo "GATEWAY_ID=$gateway_id"
  echo "GATEWAY_NAME=$GATEWAY_NAME"
  echo "GATEWAY_ENDPOINT=$GATEWAY_ENDPOINT"
  echo "OPENSHELL_WORKSPACE=$OPENSHELL_WORKSPACE"
  echo "OPENSHELL_OIDC_CLIENT_SECRET=$OPENSHELL_OIDC_CLIENT_SECRET"
  echo "CONFIG_ROOT=$CONFIG_ROOT"
} > "$GW_ENV_FILE"
log "ensure-gateway: wrote $GW_ENV_FILE (gateway $GATEWAY_NAME / $gateway_id)."
