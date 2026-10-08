#!/usr/bin/env bash

# The review runner supplies the repository, account, and pull request values.
# shellcheck disable=SC2154

load_allowed_authors() {
  local author
  local membership
  local org=${REPOSITORY%%/*}
  local members_file=/tmp/amber-members.txt
  local extra_authors=${COMMENT_ALLOWED_AUTHORS:-}
  local include_org=${COMMENT_ORG_MEMBERS:-true}

  if [[ "$include_org" != true && "$include_org" != false ]]; then
    printf 'COMMENT_ORG_MEMBERS must be true or false.\n' >&2
    return 1
  fi
  if [[ -n "$extra_authors" && ! "$extra_authors" =~ ^[A-Za-z0-9-]+(\[bot\])?(,[A-Za-z0-9-]+(\[bot\])?)*$ ]]; then
    printf 'COMMENT_ALLOWED_AUTHORS must contain comma-separated GitHub account names.\n' >&2
    return 1
  fi

  : >"$members_file"
  if [[ "$include_org" == true ]]; then
    if [[ "$review_actor" == *"[bot]" ]]; then
      # GitHub App bots are not org members; skip user/memberships and member-
      # list bot-identity checks. Still list members for comment-author filtering.
      gh api --paginate "orgs/$org/members?per_page=100" \
        --jq '.[].login' >"$members_file" || return 1
    else
      # A non-member can get a successful but incomplete public membership list.
      membership=$(gh api "user/memberships/orgs/$org" --jq .state) || return 1
      if [[ "$membership" != active ]]; then
        printf 'Amber needs active organization membership to read review history.\n' >&2
        return 1
      fi
      gh api --paginate "orgs/$org/members?per_page=100" \
        --jq '.[].login' >"$members_file" || return 1
      if ! grep --ignore-case --fixed-strings --line-regexp --quiet \
        "$review_actor" "$members_file"; then
        printf 'The organization member list does not contain the review account.\n' >&2
        return 1
      fi
    fi
  fi

  printf '%s\n' "$review_actor" >>"$members_file"
  if [[ -n "$extra_authors" ]]; then
    printf '%s\n' "${extra_authors//,/$'\n'}" >>"$members_file"
  fi
  allowed_author_json='['
  while IFS= read -r author; do
    if [[ ! "$author" =~ ^[A-Za-z0-9-]+(\[bot\])?$ ]]; then
      printf 'The allowed author list contains an invalid account name.\n' >&2
      return 1
    fi
    allowed_author_json+="\"${author,,}\","
  done <"$members_file"
  allowed_author_json="${allowed_author_json%,}]"
}

load_followup_state() {
  local thread_pr=$1
  local root_id
  local root_ids='['
  local roots_file=/tmp/amber-thread-roots.txt
  local followups_file=/tmp/amber-followups.json
  local digest
  local filter

  # Get roots separately so replies on later API pages can match them.
  gh api --paginate "repos/$REPOSITORY/pulls/$thread_pr/comments?per_page=100" \
    --jq ".[] | select(.user.login == \"$review_actor\" and .in_reply_to_id == null) | .id" \
    >"$roots_file" || return 1
  while IFS= read -r root_id; do
    [[ "$root_id" =~ ^[1-9][0-9]*$ ]] || return 1
    root_ids+="$root_id,"
  done <"$roots_file"
  root_ids="${root_ids%,}]"

  : >"$followups_file"
  if [[ "$root_ids" != '[]' ]]; then
    filter="$allowed_author_json as \$allowed | $root_ids as \$roots | .[] | select(.user.login != \"$review_actor\") | select((.user.login // \"\" | ascii_downcase) as \$login | \$allowed | index(\$login)) | select(.in_reply_to_id as \$root | \$roots | index(\$root)) | [.id, .in_reply_to_id, (.user.login | ascii_downcase), .updated_at, .body] | @json"
    gh api --paginate "repos/$REPOSITORY/pulls/$thread_pr/comments?per_page=100" \
      --jq "$filter" >"$followups_file" || return 1
  fi
  # The digest excludes Amber replies, so Amber cannot trigger itself.
  digest=$(sha256sum "$followups_file") || return 1
  digest=${digest%% *}
  [[ "$digest" =~ ^[0-9a-f]{64}$ ]] || return 1
  followup_count=$(wc -l <"$followups_file")
  followup_marker="<!-- amber-followups:v1 sha256=$digest -->"
  followup_review_filter="((.body // \"\") | contains(\"$followup_marker\"))"
  if (( followup_count == 0 )); then
    # Repair a legacy review without another model run when no reply is pending.
    followup_review_filter+=" or ((.body // \"\") | contains(\"<!-- amber-followups:\") | not)"
  fi
}

load_review_history() {
  local endpoint
  local kind
  local filter
  local latest_review_id=""
  local candidate_id
  local review_ids=/tmp/amber-history-review-ids.txt

  load_allowed_authors || return 1

  : >"$history_file"
  for kind in comments reviews inline; do
    case "$kind" in
      comments) endpoint="issues/$PR_NUMBER/comments" ;;
      reviews) endpoint="pulls/$PR_NUMBER/reviews" ;;
      inline) endpoint="pulls/$PR_NUMBER/comments" ;;
    esac
    # Filter within gh. Do not save or print bodies from other authors.
    filter="$allowed_author_json as \$allowed | .[] | select((.user.login // \"\" | ascii_downcase) as \$login | \$allowed | index(\$login))"
    if [[ "$kind" == reviews ]]; then
      filter+=" | select(.state != \"PENDING\" and .user.login != \"$review_actor\")"
    elif [[ "$kind" == comments ]]; then
      filter+=" | select(.user.login != \"$review_actor\" or ((.body // \"\") | contains(\"<!-- amber-review-status:\") | not))"
    fi
    filter+=" | {kind: \"$kind\", id, author: .user.login, body, html_url, path, line, original_line, commit_id, in_reply_to_id, pull_request_review_id, state, created_at, updated_at, submitted_at}"
    gh api --paginate "repos/$REPOSITORY/$endpoint?per_page=100" \
      --jq "$filter" >>"$history_file" || return 1
  done

  # Keep only the latest Amber summary. Keep inline findings from all reviews.
  gh api --paginate "repos/$REPOSITORY/pulls/$PR_NUMBER/reviews?per_page=100" \
    --jq ".[] | select(.user.login == \"$review_actor\" and .state != \"PENDING\") | .id" \
    >"$review_ids" || return 1
  while IFS= read -r candidate_id; do
    [[ "$candidate_id" =~ ^[1-9][0-9]*$ ]] || return 1
    if [[ -z "$latest_review_id" ]] || (( candidate_id > latest_review_id )); then
      latest_review_id=$candidate_id
    fi
  done <"$review_ids"
  if [[ -n "$latest_review_id" ]]; then
    gh api "repos/$REPOSITORY/pulls/$PR_NUMBER/reviews/$latest_review_id" \
      --jq "select(.user.login == \"$review_actor\") | {kind: \"previous_amber_review\", id, author: .user.login, body, html_url, commit_id, state}" \
      >>"$history_file" || return 1
  fi
}

hide_previous_reviews() {
  local candidates=/tmp/amber-old-reviews.tsv
  local candidate_id
  local node_id
  local extra
  local result
  local filter
  # GraphQL variables must remain literal.
  # shellcheck disable=SC2016
  local mutation='mutation AmberHideReview($id: ID!) { minimizeComment(input: {subjectId: $id, classifier: OUTDATED}) { minimizedComment { isMinimized minimizedReason } } }'

  # Require the account, the PR endpoint, an earlier review ID, and an exact
  # marker for the review's own commit. Keep the accepted review visible.
  filter=".[] | select(.user.login == \"$review_actor\" and .state != \"PENDING\" and .id < $completed_review_id) | select((.commit_id // \"\") | test(\"^[0-9a-f]{40}([0-9a-f]{24})?$\")) | select(. as \$review | (.body // \"\" | split(\"\\n\")) | index(\"<!-- amber-review:v1 repo=$REPOSITORY pr=$PR_NUMBER head=\" + \$review.commit_id + \" -->\")) | [.id, .node_id] | @tsv"
  gh api --paginate "repos/$REPOSITORY/pulls/$PR_NUMBER/reviews?per_page=100" \
    --jq "$filter" >"$candidates" || return 1

  while IFS=$'\t' read -r candidate_id node_id extra; do
    [[ "$candidate_id" =~ ^[1-9][0-9]*$ && "$node_id" =~ ^[A-Za-z0-9_=-]+$ && -z "$extra" ]] || return 1
    # Recheck before each write. A partial cleanup is safe to retry.
    load_pull_state || return 1
    if [[ "$remote_state" != open || "$remote_head_sha" != "$EXPECTED_HEAD_SHA" ]] ||
      [[ "$SKIP_DRAFT_PRS" == true && "$remote_draft" == true ]]; then
      printf 'The pull request changed before review cleanup.\n' >&2
      return 1
    fi
    result=$(gh api graphql --raw-field "query=$mutation" --raw-field "id=$node_id" \
      --jq '.data.minimizeComment.minimizedComment | [.isMinimized, .minimizedReason] | @tsv') || return 1
    if [[ "$result" != $'true\toutdated' ]]; then
      printf 'GitHub did not confirm that review %s is hidden as outdated.\n' "$candidate_id" >&2
      return 1
    fi
  done <"$candidates"
}
