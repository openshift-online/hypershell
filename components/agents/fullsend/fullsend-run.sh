#!/usr/bin/env bash
# fullsend-run.sh - EXECUTION layer (script 2 of 2).
#
# Consume the gateway env produced by ensure-gateway.sh, wire the Vertex + GitHub
# credentials, keep the gateway login fresh, and run one `fullsend run`.
#
# Inputs (env):
#   FULLSEND_AGENT   agent/harness name to run (e.g. update-openshell, code-review)
#   FULLSEND_DIR     path to the .fullsend config dir (default /opt/fullsend/.fullsend)
#   REPOSITORY       target repo "owner/name" (default openshift-online/hypershell)
#   TARGET_REPO      local checkout path for the agent (default /tmp/target-repo; cloned if absent)
#   DRY_RUN          "true"/"false" (passed through to the harness/skill; default false)
#   GW_ENV_FILE      gateway env file from script 1 (default /tmp/gw.env)
#   RUN_SH_PATH      coordinator run.sh (for openshell_for + mint_github_app_token; default /opt/sdlc-coordinator/run.sh)
#   GATEWAY_LOGIN_REFRESH_SECONDS  re-login cadence (default 120)
#   + VERTEX_AI_PROJECT_ID VERTEX_AI_REGION GOOGLE_APPLICATION_CREDENTIALS (pod env/mounts)
set -uo pipefail

FULLSEND_AGENT="${FULLSEND_AGENT:?set FULLSEND_AGENT}"
FULLSEND_DIR="${FULLSEND_DIR:-/opt/fullsend/.fullsend}"
REPOSITORY="${REPOSITORY:-openshift-online/hypershell}"
TARGET_REPO="${TARGET_REPO:-/tmp/target-repo}"
DRY_RUN="${DRY_RUN:-false}"
GW_ENV_FILE="${GW_ENV_FILE:-/tmp/gw.env}"
RUN_SH_PATH="${RUN_SH_PATH:-/opt/sdlc-coordinator/run.sh}"
GATEWAY_LOGIN_REFRESH_SECONDS="${GATEWAY_LOGIN_REFRESH_SECONDS:-120}"
FULLSEND_BIN="${FULLSEND_BIN:-fullsend}"

[[ -r "$GW_ENV_FILE" ]] || { echo "fullsend-run: $GW_ENV_FILE missing; run ensure-gateway.sh first" >&2; exit 1; }
# shellcheck disable=SC1090
source "$GW_ENV_FILE"   # GATEWAY_NAME, GATEWAY_ENDPOINT, OPENSHELL_WORKSPACE, OPENSHELL_OIDC_CLIENT_SECRET, CONFIG_ROOT

# openshell_for + mint_github_app_token come from run.sh's helper section.
main_line="$(grep -n '^# --- main' "$RUN_SH_PATH" | head -1 | cut -d: -f1)"
sed -n "1,$((main_line - 1))p" "$RUN_SH_PATH" > /tmp/gwlib.sh
# shellcheck disable=SC1091
source /tmp/gwlib.sh

REFRESHER_PID=""
cleanup() { set +e; [[ -n "$REFRESHER_PID" ]] && kill "$REFRESHER_PID" 2>/dev/null; }
trap cleanup EXIT
# NOTE: we deliberately do NOT delete the gateway here - it is persistent/reused.

# Keep the client-credentials gateway token fresh for the whole run (openshell
# does not auto-refresh it; mirrors run.sh renew_gateway_login).
( while true; do sleep "$GATEWAY_LOGIN_REFRESH_SECONDS"
    openshell_for "$CONFIG_ROOT" gateway login "$GATEWAY_NAME" >/dev/null 2>&1 || true
  done ) &
REFRESHER_PID=$!

# Credentials for fullsend: Vertex via ADC, GitHub via a minted App token.
export HOME="$CONFIG_ROOT/home" XDG_CONFIG_HOME="$CONFIG_ROOT/config" XDG_STATE_HOME="$CONFIG_ROOT/state"
export OPENSHELL_GATEWAY="$GATEWAY_NAME" OPENSHELL_GATEWAY_INSECURE=true
export OPENSHELL_WORKSPACE OPENSHELL_OIDC_CLIENT_SECRET
export GOOGLE_APPLICATION_CREDENTIALS="${GOOGLE_APPLICATION_CREDENTIALS:-/etc/sdlc-gcp/application_default_credentials.json}"
export ANTHROPIC_VERTEX_PROJECT_ID="${ANTHROPIC_VERTEX_PROJECT_ID:-$VERTEX_AI_PROJECT_ID}"
export CLOUD_ML_REGION="${CLOUD_ML_REGION:-$VERTEX_AI_REGION}"
mint_github_app_token
export GH_TOKEN="$github_app_token" GITHUB_TOKEN="$github_app_token"
export REPOSITORY REPO_FULL_NAME="$REPOSITORY" DRY_RUN

# Ensure a target-repo checkout exists for the agent.
if [[ ! -d "$TARGET_REPO/.git" ]]; then
  rm -rf "$TARGET_REPO"
  git clone --quiet "https://x-access-token:${GH_TOKEN}@github.com/${REPOSITORY}.git" "$TARGET_REPO"
fi

echo "fullsend-run: agent=$FULLSEND_AGENT repo=$REPOSITORY dry_run=$DRY_RUN gateway=$GATEWAY_NAME"
"$FULLSEND_BIN" run "$FULLSEND_AGENT" --fullsend-dir "$FULLSEND_DIR" --target-repo "$TARGET_REPO" </dev/null
rc=$?
echo "fullsend-run: exit=$rc"
exit "$rc"
