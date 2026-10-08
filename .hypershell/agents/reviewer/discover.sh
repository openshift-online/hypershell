#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

required_variables=(REPOSITORY SKIP_DRAFT_PRS MAX_REVIEWS_PER_RUN)
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

if [[ ! "$MAX_REVIEWS_PER_RUN" =~ ^[1-9][0-9]*$ ]]; then
  printf 'MAX_REVIEWS_PER_RUN is not valid.\n' >&2
  exit 1
fi

if [[ "$SKIP_DRAFT_PRS" != true && "$SKIP_DRAFT_PRS" != false ]]; then
  printf 'SKIP_DRAFT_PRS must be true or false.\n' >&2
  exit 1
fi

# shellcheck source-path=SCRIPTDIR
# shellcheck source=history.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/history.sh"

readonly open_pulls_file=/tmp/amber-open-pulls.tsv
readonly review_bodies_file=/tmp/amber-review-bodies.txt
readonly history_marker='<!-- amber-review-history:v1 -->'
readonly labels_marker='<!-- amber-review-labels:v1 -->'
readonly status_comment_ids_file=/tmp/amber-status-comment-ids.txt

# GitHub App installation tokens cannot call GET /user (403), and GET /app
# requires a JWT (not an installation token). Use GITHUB_APP_SLUG env var
# (set to <slug> in cronjob.yaml) to construct the bot login <slug>[bot].
if review_actor=$(gh api user --jq .login 2>/dev/null); then
  : # PAT or user-token auth
elif [[ -n "${GITHUB_APP_SLUG:-}" ]]; then
  review_actor="${GITHUB_APP_SLUG}[bot]"
else
  printf 'Cannot read the GitHub review account.\n' >&2
  exit 1
fi
if [[ ! "$review_actor" =~ ^[A-Za-z0-9-]+(\[bot\])?$ ]]; then
  printf 'The GitHub review account name is not valid.\n' >&2
  exit 1
fi
readonly review_actor
load_allowed_authors

has_amber_review() {
  local pr_number=$1
  local marker=$2
  local jq_filter
  local candidate_id
  local latest_review_id=""

  jq_filter=".[] | select(.user.login == \"$review_actor\" and .state != \"PENDING\" and ((.body // \"\") | contains(\"$marker\"))) | .id"
  if ! gh api --paginate \
    "repos/$REPOSITORY/pulls/$pr_number/reviews?per_page=100" \
    --jq "$jq_filter" >"$review_bodies_file"; then
    printf 'Cannot read reviews for %s#%s.\n' "$REPOSITORY" "$pr_number" >&2
    return 2
  fi
  while IFS= read -r candidate_id; do
    [[ "$candidate_id" =~ ^[1-9][0-9]*$ ]] || return 2
    if [[ -z "$latest_review_id" ]] || (( candidate_id > latest_review_id )); then
      latest_review_id=$candidate_id
    fi
  done <"$review_bodies_file"
  [[ -n "$latest_review_id" ]] || return 1

  if ! gh api "repos/$REPOSITORY/pulls/$pr_number/reviews/$latest_review_id" \
    --jq "select($followup_review_filter) | .body // \"\"" >"$review_bodies_file"; then
    return 2
  fi
  grep --fixed-strings --quiet -- "$marker" "$review_bodies_file"
}

has_completed_status_comment() {
  local pr_number=$1
  local review_marker=$2
  local status_marker=$3
  local jq_filter

  jq_filter=".[] | select(.user.login == \"$review_actor\" and ((.body // \"\") | contains(\"$status_marker\")) and ((.body // \"\") | contains(\"$review_marker\")) and ((.body // \"\") | contains(\"$labels_marker\")) and ((.body // \"\") | contains(\"$history_marker\")) and ((.body // \"\") | contains(\"$followup_marker\"))) | .id"
  if ! gh api --paginate \
    "repos/$REPOSITORY/issues/$pr_number/comments?per_page=100" \
    --jq "$jq_filter" >"$status_comment_ids_file"; then
    printf 'Cannot read status comments for %s#%s.\n' \
      "$REPOSITORY" "$pr_number" >&2
    return 2
  fi

  if [[ -s "$status_comment_ids_file" ]]; then
    return 0
  fi

  return 1
}

printf 'Use GitHub account %s for Amber reviews.\n' "$review_actor" >&2
if ! gh api --paginate \
  "repos/$REPOSITORY/pulls?state=open&sort=created&direction=asc&per_page=100" \
  --jq '.[] | [.number, .head.sha, .draft] | @tsv' >"$open_pulls_file"; then
  printf 'Cannot list open pull requests for %s.\n' "$REPOSITORY" >&2
  exit 1
fi

mapfile -t open_pulls <"$open_pulls_file"
selected_count=0
already_complete_count=0
skipped_draft_count=0

for pull_row in "${open_pulls[@]}"; do
  IFS=$'\t' read -r pr_number head_sha is_draft extra_value <<<"$pull_row"
  if [[ ! "$pr_number" =~ ^[1-9][0-9]*$ ]] ||
    [[ ! "$head_sha" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]] ||
    [[ ! "$is_draft" =~ ^(true|false)$ ]] ||
    [[ -n "${extra_value:-}" ]]; then
    printf 'GitHub returned invalid pull request data.\n' >&2
    exit 1
  fi

  if [[ "$SKIP_DRAFT_PRS" == true && "$is_draft" == true ]]; then
    printf 'Skip draft pull request %s#%s at %s.\n' \
      "$REPOSITORY" "$pr_number" "$head_sha" >&2
    ((skipped_draft_count += 1))
    continue
  fi

  review_marker="<!-- amber-review:v1 repo=$REPOSITORY pr=$pr_number head=$head_sha -->"
  status_marker="<!-- amber-review-status:v2 repo=$REPOSITORY pr=$pr_number -->"
  load_followup_state "$pr_number"
  review_status=0
  has_amber_review "$pr_number" "$review_marker" || review_status=$?
  case "$review_status" in
    0)
      status_result=0
      has_completed_status_comment \
        "$pr_number" "$review_marker" "$status_marker" || status_result=$?
      case "$status_result" in
        0)
          printf 'Amber already completed %s#%s at %s.\n' \
            "$REPOSITORY" "$pr_number" "$head_sha" >&2
          ((already_complete_count += 1))
          continue
          ;;
        1)
          printf 'The Amber status for %s#%s at %s needs repair.\n' \
            "$REPOSITORY" "$pr_number" "$head_sha" >&2
          ;;
        *)
          exit 1
          ;;
      esac
      ;;
    1)
      ;;
    *)
      exit 1
      ;;
  esac

  printf '%s\t%s\n' "$pr_number" "$head_sha"
  ((selected_count += 1))
  if (( selected_count >= MAX_REVIEWS_PER_RUN )); then
    break
  fi
done

printf 'Amber discovery summary: %s selected, %s complete, and %s draft.\n' \
  "$selected_count" "$already_complete_count" "$skipped_draft_count" >&2
