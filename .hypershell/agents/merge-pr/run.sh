#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

readonly result_file=/tmp/result.json
trap 'printf "{\"status\":\"failed\",\"error\":\"run.sh exited unexpectedly at line %s\"}\n" "$LINENO" > "$result_file"' ERR

finish() {
  jq -n --arg status "$1" --arg summary "$2" '{status: $status, summary: $summary}' > "$result_file"
  printf '%s\n' "$2"
  exit 0
}

if [[ ! ${REPOSITORY:-} =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] ||
   [[ ! ${ITEM_NUMBER:-} =~ ^[1-9][0-9]*$ ]] ||
   [[ ! ${DRY_RUN:-} =~ ^(true|false)$ ]]; then
  printf '{"status":"failed","error":"Invalid REPOSITORY, ITEM_NUMBER, or DRY_RUN"}\n' > "$result_file"
  exit 1
fi

cant_merge() {
  local reason=$1
  if [[ $DRY_RUN == true ]]; then
    finish success "Dry run: would replace agent/review-code-approved with agent/cant-merge: $reason"
  fi
  gh label create agent/cant-merge --repo "$REPOSITORY" --color B60205 \
    --description 'Approved implementation could not merge' --force
  # Add first: a failed removal leaves the issue discoverable for retry.
  gh issue edit "$ITEM_NUMBER" --repo "$REPOSITORY" --add-label agent/cant-merge
  gh issue edit "$ITEM_NUMBER" --repo "$REPOSITORY" --remove-label agent/review-code-approved
  finish success "Cannot merge: $reason"
}

issue=$(gh issue view "$ITEM_NUMBER" --repo "$REPOSITORY" --json state,labels,body)
if ! jq -e 'any(.labels[]; .name == "agent/review-code-approved")' <<< "$issue" > /dev/null; then
  finish skip 'Issue is no longer approved'
fi

owner=${REPOSITORY%%/*}
repo=${REPOSITORY#*/}
# Paginate references and approval events so sandbox restarts need no local state.
# GraphQL variables are supplied by gh.
# shellcheck disable=SC2016
timeline=$(gh api graphql --paginate --slurp -f owner="$owner" -f repo="$repo" -F number="$ITEM_NUMBER" -f query='
  query($owner: String!, $repo: String!, $number: Int!, $endCursor: String) {
    repository(owner: $owner, name: $repo) {
      issue(number: $number) {
        timelineItems(first: 100, after: $endCursor,
          itemTypes: [CONNECTED_EVENT, CROSS_REFERENCED_EVENT, LABELED_EVENT]) {
          pageInfo { hasNextPage endCursor }
          nodes {
            ... on LabeledEvent { createdAt label { name } }
            ... on ConnectedEvent {
              subject { ... on PullRequest { number repository { nameWithOwner } } }
              source { ... on PullRequest { number repository { nameWithOwner } } }
            }
            ... on CrossReferencedEvent { source { ... on PullRequest { number repository { nameWithOwner } } } }
          }
        }
      }
    }
  }')
approved_at=$(jq -r '[.[].data.repository.issue.timelineItems.nodes[] |
  select(.label.name == "agent/review-code-approved") | .createdAt] | max // ""' <<< "$timeline")
numbers=$(jq -n --argjson timeline "$timeline" --argjson issue "$issue" --arg repo "$REPOSITORY" '
  ([ $timeline[].data.repository.issue.timelineItems.nodes[] | (.subject, .source) |
     select(.repository.nameWithOwner == $repo) | .number ] +
   [ $issue.body | scan("https://github[.]com/" + ($repo | gsub("[.]"; "\\.")) + "/pull/([0-9]+)\\b") | .[0] | tonumber ]) |
  unique[]')
[[ -n $numbers ]] || finish skip 'No linked PR found'

prs='[]'
while IFS= read -r number; do
  pr=$(gh pr view "$number" --repo "$REPOSITORY" --json number,state,id,headRefOid,mergeable,isDraft)
  prs=$(jq --argjson pr "$pr" '. + [$pr]' <<< "$prs")
done <<< "$numbers"
open_prs=$(jq '[.[] | select(.state == "OPEN")]' <<< "$prs")
count=$(jq length <<< "$open_prs")
if (( count == 0 )); then
  if jq -e 'any(.[]; .state == "MERGED")' <<< "$prs" > /dev/null; then
    if [[ $DRY_RUN == true ]]; then
      finish success 'Dry run: would replace agent/review-code-approved with agent/merged'
    fi
    gh label create agent/merged --repo "$REPOSITORY" --color 0E8A16 \
      --description 'Approved implementation has merged' --force
    gh issue edit "$ITEM_NUMBER" --repo "$REPOSITORY" --add-label agent/merged
    gh issue edit "$ITEM_NUMBER" --repo "$REPOSITORY" --remove-label agent/review-code-approved
    finish success 'Linked PR merged; applied agent/merged'
  fi
  finish skip 'Linked PRs are closed without merging'
elif (( count > 1 )); then
  finish skip 'Multiple open linked PRs; cannot choose deterministically'
fi
if [[ $(jq -r .state <<< "$issue") != OPEN ]]; then
  finish skip 'Issue is closed and its linked PR has not merged'
fi
pr=$(jq '.[0]' <<< "$open_prs")
number=$(jq -r .number <<< "$pr")
id=$(jq -r .id <<< "$pr")
head=$(jq -r .headRefOid <<< "$pr")

# GraphQL variables are supplied by gh.
# shellcheck disable=SC2016
queue=$(gh api graphql --paginate --slurp -f id="$id" -f query='
  query($id: ID!, $endCursor: String) {
    node(id: $id) { ... on PullRequest {
      mergeQueueEntry { id }
      timelineItems(first: 100, after: $endCursor,
        itemTypes: [ADDED_TO_MERGE_QUEUE_EVENT, REMOVED_FROM_MERGE_QUEUE_EVENT]) {
        pageInfo { hasNextPage endCursor }
        nodes {
          __typename
          ... on AddedToMergeQueueEvent { createdAt }
          ... on RemovedFromMergeQueueEvent { createdAt }
        }
      }
    } }
  }')
if jq -e --arg approved "$approved_at" '
  select(.[0].data.node.mergeQueueEntry == null) |
  [.[].data.node.timelineItems.nodes[]] | sort_by(.createdAt) | last |
  .__typename == "RemovedFromMergeQueueEvent" and .createdAt >= $approved
' <<< "$queue" > /dev/null; then
  cant_merge "PR #$number was removed from the merge queue"
fi
if [[ $(jq -r .mergeable <<< "$pr") == CONFLICTING ]]; then
  cant_merge "PR #$number has merge conflicts"
fi

# gh pr checks paginates both check runs and legacy commit statuses.
checks_status=0
checks=$(gh pr checks "$number" --repo "$REPOSITORY" --json bucket) || checks_status=$?
if (( checks_status != 0 && checks_status != 1 && checks_status != 8 )); then
  printf 'Unable to read CI checks for PR #%s.\n' "$number" >&2
  false
fi
jq -e 'type == "array"' <<< "$checks" > /dev/null
if jq -e 'any(.[]; .bucket == "fail" or .bucket == "cancel")' <<< "$checks" > /dev/null; then
  cant_merge "PR #$number has failing CI"
fi
if jq -e '.[0].data.node.mergeQueueEntry != null' <<< "$queue" > /dev/null; then
  finish skip "PR #$number is already in the merge queue"
fi
if jq -e 'any(.[]; .bucket == "pending")' <<< "$checks" > /dev/null; then
  finish skip "PR #$number is waiting for CI"
fi
if jq -e '.isDraft or .mergeable == "UNKNOWN"' <<< "$pr" > /dev/null; then
  finish skip "PR #$number is a draft or mergeability is not yet known"
fi
if [[ $DRY_RUN == true ]]; then
  finish success "Dry run: would enqueue PR #$number"
fi

# Use the queue mutation directly: never fall back to a direct merge or bypass.
# Pin the head to avoid enqueueing a commit pushed after the checks above.
# GraphQL variables are supplied by gh.
# shellcheck disable=SC2016
enqueued=$(gh api graphql -f id="$id" -f head="$head" -f query='
  mutation($id: ID!, $head: GitObjectID!) {
    enqueuePullRequest(input: {pullRequestId: $id, expectedHeadOid: $head}) {
      mergeQueueEntry { id }
    }
  }')
jq -e '.data.enqueuePullRequest.mergeQueueEntry.id != null' <<< "$enqueued" > /dev/null
finish success "Enqueued PR #$number"
