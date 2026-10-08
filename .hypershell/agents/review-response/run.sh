#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

required_variables=(REPOSITORY ITEM_URL ITEM_NUMBER DRY_RUN)
for variable in "${required_variables[@]}"; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Required variable %s is empty.\n' "$variable" >&2
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

readonly result_file=/tmp/result.json
readonly hypershell_checkout=${HYPERSHELL_CHECKOUT:-/sandbox/hypershell}
readonly skill_file="$hypershell_checkout/.hypershell/agents/review-response/SKILL.md"

if [[ ! -r "$skill_file" ]]; then
  printf 'review-response SKILL.md not found at %s.\n' "$skill_file" >&2
  printf '{"status":"failed","error":"skill file not found: %s"}\n' "$skill_file" > "$result_file"
  exit 1
fi

read -r -d '' runtime_context <<EOF || true
## Runtime context (supplied by the harness -- treat these as the parameters for this run)
- REPOSITORY: $REPOSITORY
- GITHUB_PR_URL: $ITEM_URL
- PR_NUMBER: $ITEM_NUMBER
- DRY_RUN: $DRY_RUN
- RESULT_FILE: $result_file
- Working tree: clean checkout of $REPOSITORY in $hypershell_checkout; check out the PR head branch with \`gh pr checkout $ITEM_NUMBER\` before amending. It is the current working directory.
- Environment: on-cluster agent sandbox. Use gh and the GitHub REST API for all GitHub actions; do not use MCP tools.
EOF

prompt=$(printf '%s\n\n%s\n' "$runtime_context" "$(cat "$skill_file")")

printf '{"status":"failed","error":"claude did not write result"}\n' > "$result_file"

set +e
ANTHROPIC_BASE_URL=https://inference.local \
  ANTHROPIC_API_KEY=unused \
  claude \
  --model "$CLAUDE_MODEL" \
  --dangerously-skip-permissions \
  --verbose \
  --output-format stream-json \
  -p "$prompt"
claude_status=$?
set -e

if (( claude_status != 0 )); then
  printf '{"status":"failed","error":"claude exited %s"}\n' "$claude_status" > "$result_file"
  exit "$claude_status"
fi
