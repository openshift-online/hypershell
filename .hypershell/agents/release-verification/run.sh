#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

readonly result_file=/tmp/result.json
readonly claude_output_file=/tmp/release-verify-claude-output.txt

trap 'printf "{\"status\":\"failed\",\"error\":\"run.sh exited unexpectedly at line %s\"}\n" "$LINENO" > "$result_file"' ERR

required_variables=(REPOSITORY ITEM_URL ITEM_NUMBER RELEASE_BUNDLE GITOPS_REPO DRY_RUN)
for variable in "${required_variables[@]}"; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Required variable %s is empty.\n' "$variable" >&2
    printf '{"status":"failed","error":"Required variable %s is empty"}\n' "$variable" > "$result_file"
    exit 1
  fi
done

readonly checkout_dir="${HYPERSHELL_CHECKOUT:-/sandbox/hypershell}"
readonly skill_file="$checkout_dir/.hypershell/agents/release-verification/SKILL.md"

if [[ ! -r "$skill_file" ]]; then
  printf 'Skill file not found: %s\n' "$skill_file" >&2
  printf '{"status":"failed","error":"Skill file not found: %s"}\n' "$skill_file" > "$result_file"
  exit 1
fi

printf 'Release-verification agent: issue %s bundle %s (DRY_RUN=%s).\n' \
  "$ITEM_URL" "$RELEASE_BUNDLE" "$DRY_RUN"

read -r -d '' runtime_context <<EOF || true
## Runtime context (supplied by the harness -- treat these as the parameters for this run)
- REPOSITORY: $REPOSITORY
- GITHUB_ISSUE_URL: $ITEM_URL
- RELEASE_BUNDLE: $RELEASE_BUNDLE
- GITOPS_REPO: $GITOPS_REPO
- KUBECONFIG: ${KUBECONFIG:-<not set -- skip cluster checks>}
- RELEASE_STRATEGY_OVERLAY: ${RELEASE_STRATEGY_OVERLAY:-<not set>}
- DRY_RUN: $DRY_RUN
- RESULT_FILE: $result_file
- Environment: on-cluster agent sandbox. Use gh and the GitHub REST API for all GitHub actions; do not use MCP tools.
EOF

prompt=$(printf '%s\n\n%s\n' "$runtime_context" "$(cat "$skill_file")")

set +e
ANTHROPIC_BASE_URL=https://inference.local \
  ANTHROPIC_API_KEY=unused \
  claude \
  --model "$CLAUDE_MODEL" \
  --dangerously-skip-permissions \
  --verbose \
  --output-format stream-json \
  -p "$prompt" \
  2>&1 | tee "$claude_output_file"
claude_status=${PIPESTATUS[0]}
set -e

if (( claude_status != 0 )); then
  printf '{"status":"failed","error":"claude exited %s"}\n' "$claude_status" > "$result_file"
  exit "$claude_status"
fi

if [[ ! -s "$result_file" ]]; then
  printf '{"status":"success","summary":"release-verification complete for issue %s"}\n' "$ITEM_NUMBER" > "$result_file"
fi
