#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

readonly result_file=/tmp/result.json
readonly claude_output_file=/tmp/review-response-claude-output.txt

trap 'printf "{\"status\":\"failed\",\"error\":\"run.sh exited unexpectedly at line %s\"}\n" "$LINENO" > "$result_file"' ERR

required_variables=(REPOSITORY ITEM_URL ITEM_NUMBER DRY_RUN)
for variable in "${required_variables[@]}"; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Required variable %s is empty.\n' "$variable" >&2
    printf '{"status":"failed","error":"Required variable %s is empty"}\n' "$variable" > "$result_file"
    exit 1
  fi
done

if [[ ! "$REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  printf 'REPOSITORY is not valid.\n' >&2
  exit 1
fi

if [[ ! "$ITEM_NUMBER" =~ ^[0-9]+$ ]]; then
  printf 'ITEM_NUMBER must be numeric.\n' >&2
  exit 1
fi

if [[ ! "$DRY_RUN" =~ ^(true|false)$ ]]; then
  printf 'DRY_RUN must be true or false.\n' >&2
  exit 1
fi

readonly hypershell_checkout=${HYPERSHELL_CHECKOUT:-/sandbox/hypershell}
readonly skill_file="$hypershell_checkout/.hypershell/agents/review-response/SKILL.md"

if [[ ! -r "$skill_file" ]]; then
  printf 'review-response SKILL.md not found at %s.\n' "$skill_file" >&2
  printf '{"status":"failed","error":"skill file not found: %s"}\n' "$skill_file" > "$result_file"
  exit 1
fi

printf 'Review-response agent: PR %s (DRY_RUN=%s).\n' "$ITEM_URL" "$DRY_RUN"

read -r -d '' runtime_context <<EOF || true
## Runtime context (supplied by the harness -- treat these as the parameters for this run)
- REPOSITORY: $REPOSITORY
- GITHUB_PR_URL: $ITEM_URL
- PR_NUMBER: $ITEM_NUMBER
- HYPERSHELL_REF: main
- DRY_RUN: $DRY_RUN
- RESULT_FILE: $result_file
- Working tree: clean checkout of $REPOSITORY in $hypershell_checkout; check out the PR head branch with \`gh pr checkout $ITEM_NUMBER\` before amending. It is the current working directory.
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
  printf '{"status":"success","summary":"review-response complete for PR %s"}\n' "$ITEM_NUMBER" > "$result_file"
fi
