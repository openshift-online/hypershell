#!/usr/bin/env bash
# pr-env-commands.sh - resolve /pr-extend and /pr-destroy against a pull
# request's command history (ephemeral-pr-environments.spec.md: Extend and
# Destroy Controls).
#
# The pull request's own authorized command comments are authoritative, not
# the order in which comment-triggered runs execute. This script:
#   1. Checks the triggering commenter's GitHub permission (write/maintain/admin)
#      before any cluster credential is used.
#   2. Acknowledges an unauthorized trigger rather than acting silently.
#   3. Scans every issue comment, counts only authorized command comments, and
#      takes the latest by created_at.
#   4. Reconciles the pr-environment/pr-extended label to that decision.
#   5. Prints GITHUB_OUTPUT so the workflow can deploy, tear down, or no-op.
#
# Requires `gh` (authenticated via GH_TOKEN) and `jq`.
#
# Environment:
#   GH_REPO / GITHUB_REPOSITORY   owner/repo
#   PR_NUMBER                     pull-request number (required)
#   TRIGGER_USER                  commenter login (required)
#   TRIGGER_BODY                  comment body (required)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=pr-env-lib.sh
source "${SCRIPT_DIR}/pr-env-lib.sh"

: "${PR_NUMBER:?PR_NUMBER is required}"
: "${TRIGGER_USER:?TRIGGER_USER is required}"
: "${TRIGGER_BODY:?TRIGGER_BODY is required}"
repo="${GH_REPO:-${GITHUB_REPOSITORY:?GH_REPO or GITHUB_REPOSITORY is required}}"

write_output() {
  local key="$1" value="$2"
  printf '%s=%s\n' "${key}" "${value}"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf '%s=%s\n' "${key}" "${value}" >> "${GITHUB_OUTPUT}"
  fi
}

slash_for() {
  case "$1" in
    extend) printf '%s' "${PR_ENV_COMMAND_EXTEND}" ;;
    destroy) printf '%s' "${PR_ENV_COMMAND_DESTROY}" ;;
    *) printf '%s' "$1" ;;
  esac
}

# GitHub collaborator permission for a user. 404 / non-collaborator -> none.
repo_permission() {
  local user="$1"
  local perm
  perm="$(gh api "repos/${repo}/collaborators/${user}/permission" --jq .permission 2>/dev/null || true)"
  printf '%s' "${perm:-none}"
}

origin_head() {
  gh api "repos/${repo}/pulls/${PR_NUMBER}" --jq '{head: .head.repo.full_name, sha: .head.sha}'
}

has_retained_label() {
  gh api "repos/${repo}/issues/${PR_NUMBER}/labels" \
    --jq --arg name "${PR_ENV_RETAINED_LABEL}" '[.[].name] | index($name) != null'
}

add_retained_label() {
  gh api --method POST "repos/${repo}/issues/${PR_NUMBER}/labels" \
    --input - <<< "{\"labels\":[\"${PR_ENV_RETAINED_LABEL}\"]}" >/dev/null
}

remove_retained_label() {
  local encoded
  encoded="$(printf '%s' "${PR_ENV_RETAINED_LABEL}" | jq -sRr @uri)"
  gh api --method DELETE "repos/${repo}/issues/${PR_NUMBER}/labels/${encoded}" >/dev/null
}

reconcile_label() {
  local action="$1"
  local has_label
  has_label="$(has_retained_label)"
  case "${action}" in
    extend)
      if [[ "${has_label}" != "true" ]]; then
        add_retained_label
        echo "Added label ${PR_ENV_RETAINED_LABEL}"
      else
        echo "Label ${PR_ENV_RETAINED_LABEL} already present"
      fi
      ;;
    destroy|none)
      if [[ "${has_label}" == "true" ]]; then
        remove_retained_label
        echo "Removed label ${PR_ENV_RETAINED_LABEL}"
      else
        echo "Label ${PR_ENV_RETAINED_LABEL} already absent"
      fi
      ;;
  esac
}

acknowledge_refusal() {
  local command="$1"
  local body
  body="$(pr_env_refusal_comment "${TRIGGER_USER}" "${command}")"
  gh api --method POST "repos/${repo}/issues/${PR_NUMBER}/comments" \
    -f body="${body}" >/dev/null
  echo "Acknowledged unauthorized ${command} from ${TRIGGER_USER}"
}

# Emit created_at<TAB>user<TAB>permission<TAB>body for every issue comment.
# Body newlines become spaces so the TSV stays one row per comment.
list_comment_rows() {
  local comments users user perm created body
  comments="$(gh api --paginate "repos/${repo}/issues/${PR_NUMBER}/comments")"
  users="$(printf '%s' "${comments}" | jq -r '.[].user.login' | sort -u)"
  declare -A perms=()
  while IFS= read -r user; do
    [[ -n "${user}" ]] || continue
    perms["${user}"]="$(repo_permission "${user}")"
  done <<< "${users}"
  while IFS=$'\t' read -r created user body; do
    [[ -n "${created}" ]] || continue
    perm="${perms[${user}]:-none}"
    printf '%s\t%s\t%s\t%s\n' "${created}" "${user}" "${perm}" "${body}"
  done < <(printf '%s' "${comments}" | jq -r '.[] | [.created_at, .user.login, (.body // "" | gsub("\n"; " "))] | @tsv')
}

trigger_cmd="$(pr_env_command_from_body "${TRIGGER_BODY}")"
if [[ -z "${trigger_cmd}" ]]; then
  echo "Trigger body is not a /pr-extend or /pr-destroy command; nothing to do"
  write_output action none
  write_output origin false
  write_output retained false
  write_output head_sha ""
  write_output trigger_authorized false
  exit 0
fi

trigger_perm="$(repo_permission "${TRIGGER_USER}")"
trigger_authorized=false
if pr_env_permission_is_authorized "${trigger_perm}"; then
  trigger_authorized=true
else
  acknowledge_refusal "$(slash_for "${trigger_cmd}")"
fi

pr_json="$(origin_head)"
head_repo="$(printf '%s' "${pr_json}" | jq -r .head)"
sha="$(printf '%s' "${pr_json}" | jq -r .sha)"
origin=false
if [[ "${head_repo}" == "${repo}" ]]; then
  origin=true
fi

action="$(list_comment_rows | pr_env_select_latest_command)"
# Fork PRs never receive cluster credentials. Still reconcile the label from
# authorized command history so the cache stays honest, but report origin=false
# so the workflow skips deploy/teardown.
if [[ "${origin}" == "true" ]]; then
  reconcile_label "${action}"
else
  echo "Pull request head ${head_repo} is not the origin ${repo}; no cluster action"
  action=none
fi

retained=false
if [[ "${action}" == "extend" ]]; then
  retained=true
fi

write_output action "${action}"
write_output origin "${origin}"
write_output retained "${retained}"
write_output head_sha "${sha}"
write_output trigger_authorized "${trigger_authorized}"
echo "Resolved action=${action} origin=${origin} retained=${retained} trigger_authorized=${trigger_authorized}"
