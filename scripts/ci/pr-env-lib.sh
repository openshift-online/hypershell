#!/usr/bin/env bash
# pr-env-lib.sh - pure helpers for the ephemeral pull-request environment
# workflow and its out-of-band reaper (ephemeral-pr-environments.spec.md,
# HYPERSHELL-240).
#
# This file holds ONLY pure, side-effect-free functions so both the CI workflow
# (scripts/ci/*.sh) and the in-cluster reaper (scripts/ci/reap-pr-environments.sh,
# shipped to the cluster via deploy/e2e/reaper) can share one definition of the
# per-PR namespace naming, the timebox, the ownership labels, and the reaper
# match predicate. It performs no cluster calls, so it is unit-tested without a
# cluster by scripts/ci/pr-env-lib_test.sh. Source it; do not execute it.

# --- Ownership labels + timebox annotation (must match the OpenShift lifecycle
# driver in scripts/cluster/drivers/openshift.sh so status and cleanup tooling
# stay one selector set). ---
PR_ENV_NS_PREFIX="hypershell-ci-pr-"
# Push-to-main environments are per-commit so a cancelled older run's
# teardown cannot delete a newer deploy's namespace. The 7-char short SHA
# keeps the platform name within 54 characters (full SHA would make
# -keycloak exceed 63).
PR_ENV_MAIN_NS_PREFIX="hypershell-ci-main-"
PR_ENV_MAIN_SHA_LEN=7
PR_ENV_OWNED_LABEL="hypershell.redhat.io/owned"
PR_ENV_ENVIRONMENT_LABEL="hypershell.redhat.io/environment"
PR_ENV_MANAGED_LABEL="app.kubernetes.io/managed-by"
PR_ENV_MANAGED_VALUE="hypershell-lifecycle"
PR_ENV_PART_OF_LABEL="app.kubernetes.io/part-of"
PR_ENV_PART_OF_VALUE="hypershell"
PR_ENV_EXPIRES_ANNOTATION="hypershell.redhat.io/expires-at"
# Selects CI-owned Keycloak namespaces for ESO ClusterExternalSecret
# projection of standing CI secrets (ephemeral-ci-secrets.spec.md).
PR_ENV_CI_KEYCLOAK_LABEL="hypershell.redhat.io/ci-keycloak"
PR_ENV_CI_KEYCLOAK_VALUE="true"
# Control-plane stamps on gateway namespaces. Must match
# components/control-plane/internal/gateway/namespace.go. Distinct from
# PR_ENV_MANAGED_VALUE, which marks the platform/keycloak namespace group.
PR_ENV_CP_MANAGED_LABEL="hypershell.redhat.io/managed"
PR_ENV_CP_MANAGED_VALUE="true"
PR_ENV_CP_MANAGED_BY_LABEL="app.kubernetes.io/managed-by"
PR_ENV_CP_MANAGED_BY_VALUE="hypershell-control-plane"
PR_ENV_CP_INSTANCE_LABEL="hypershell.redhat.io/instance"

# Default max lifetime in hours for a retained pull request. The workflow
# overrides this from vars.PR_ENV_RETAINED_MAX_HOURS. 72 hours is the former
# 3-day inactivity window.
: "${PR_ENV_RETAINED_MAX_HOURS:=72}"
# Default max lifetime in hours for an unretained (ephemeral) deploy.
# In-run teardown is the primary path; this is what the reaper uses if that
# teardown does not complete. Overridden by vars.PR_ENV_UNRETAINED_MAX_HOURS.
: "${PR_ENV_UNRETAINED_MAX_HOURS:=24}"

# Durable cache of the latest authorized /pr-extend vs /pr-destroy decision.
# The pull request's command history is authoritative; this label is a cache
# so later synchronize runs can read retained state without rescanning comments.
PR_ENV_RETAINED_LABEL="pr-environment/pr-extended"
PR_ENV_COMMAND_EXTEND="/pr-extend"
PR_ENV_COMMAND_DESTROY="/pr-destroy"

# Stable hidden marker so later runs update the one access comment rather than
# post a new comment per run.
PR_ENV_COMMENT_MARKER='<!-- hypershell-pr-environment -->'

# pr_env_namespace <pr-number> -> the platform namespace name.
pr_env_namespace() {
  printf '%s%s' "${PR_ENV_NS_PREFIX}" "$1"
}

# pr_env_main_namespace <commit-sha> -> the push-to-main platform namespace.
# Uses the first 7 hex characters so two in-flight main runs never share a
# namespace, while -keycloak stays an RFC 1123 name under 63 characters.
pr_env_main_namespace() {
  local sha
  sha="$(printf '%s' "${1:?commit SHA is required}" | tr '[:upper:]' '[:lower:]')"
  if [[ ${#sha} -lt "${PR_ENV_MAIN_SHA_LEN}" ]]; then
    echo "commit SHA is shorter than ${PR_ENV_MAIN_SHA_LEN} characters" >&2
    return 1
  fi
  printf '%s%s' "${PR_ENV_MAIN_NS_PREFIX}" "${sha:0:${PR_ENV_MAIN_SHA_LEN}}"
}

# pr_env_keycloak_namespace <platform-namespace> -> the companion Keycloak
# namespace, matching keycloak_namespace_for in scripts/cluster/lib.sh.
pr_env_keycloak_namespace() {
  printf '%s-keycloak' "$1"
}

# pr_env_environment_id <pr-number> -> the environment identifier stamped into
# hypershell.redhat.io/environment. The pr- prefix distinguishes pull-request
# environments from local `make openshift-up` environments (opaque ids).
pr_env_environment_id() {
  printf 'pr-%s' "$1"
}

# pr_env_is_reserved_namespace <name> - true for cluster-reserved namespaces the
# reaper must never delete. Mirrors is_reserved_cluster_namespace in
# scripts/cluster/lib.sh; duplicated (not sourced) so the reaper container needs
# only this one file.
pr_env_is_reserved_namespace() {
  case "$1" in
    default|openshift|kube-system|kube-public|kube-node-lease) return 0 ;;
    kube-*|openshift-*) return 0 ;;
  esac
  return 1
}

# pr_env_epoch_to_rfc3339 <epoch-seconds> -> RFC 3339 UTC timestamp, portable
# across GNU date (Linux/CI/reaper container) and BSD date (macOS).
pr_env_epoch_to_rfc3339() {
  local epoch="$1"
  if date -u -r "${epoch}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null; then
    return 0
  fi
  date -u -d "@${epoch}" +%Y-%m-%dT%H:%M:%SZ
}

# pr_env_rfc3339_to_epoch <rfc3339> -> epoch seconds, or empty + non-zero when
# the timestamp cannot be parsed. Portable across GNU and BSD date.
pr_env_rfc3339_to_epoch() {
  local ts="$1"
  [[ -n "${ts}" ]] || return 1
  if date -u -d "${ts}" +%s 2>/dev/null; then
    return 0
  fi
  date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "${ts}" +%s 2>/dev/null
}

pr_env_now_epoch() {
  date -u +%s
}

# pr_env_expires_at_seconds <seconds> [now-epoch] -> RFC 3339 UTC timestamp
# <seconds> in the future. now-epoch is injectable for deterministic tests.
pr_env_expires_at_seconds() {
  local seconds="$1"
  local now="${2:-$(pr_env_now_epoch)}"
  pr_env_epoch_to_rfc3339 $(( now + seconds ))
}

# pr_env_expires_at_hours <hours> [now-epoch] -> RFC 3339 UTC timestamp <hours>
# in the future. Used for both retained and unretained max-lifetime stamps.
pr_env_expires_at_hours() {
  pr_env_expires_at_seconds $(( ${1} * 3600 )) "${2:-}"
}

# pr_env_inactivity_expires_at <retained> [now-epoch] -> RFC 3339 UTC expiry
# for the dual timebox. retained=true uses PR_ENV_RETAINED_MAX_HOURS,
# otherwise PR_ENV_UNRETAINED_MAX_HOURS. now-epoch is injectable for tests.
pr_env_inactivity_expires_at() {
  local retained="${1:-false}"
  if [[ "${retained}" == "true" ]]; then
    pr_env_expires_at_hours "${PR_ENV_RETAINED_MAX_HOURS}" "${2:-}"
  else
    pr_env_expires_at_hours "${PR_ENV_UNRETAINED_MAX_HOURS}" "${2:-}"
  fi
}

# pr_env_command_from_body <body> -> "extend", "destroy", or empty.
# A command matches when the body is, or begins with, /pr-extend or /pr-destroy
# (optional leading whitespace, including blank lines). GitHub's web UI stores
# comments with CRLF; strip CR so `/pr-extend\r` does not fail the match.
# /pr-extended does not match /pr-extend.
pr_env_command_from_body() {
  local body="${1:-}"
  local first
  first="$(printf '%s' "${body}" | tr -d '\r' | awk 'NF { print $1; exit }')"
  case "${first}" in
    "${PR_ENV_COMMAND_EXTEND}") printf 'extend' ;;
    "${PR_ENV_COMMAND_DESTROY}") printf 'destroy' ;;
  esac
}

# pr_env_permission_is_authorized <permission> - true for write, maintain, or
# admin on the origin repository. GitHub's collaborator permission API returns
# these strings; author_association is not consulted.
pr_env_permission_is_authorized() {
  case "${1:-}" in
    write|maintain|admin) return 0 ;;
  esac
  return 1
}

# pr_env_label_list_has <name>
#
# Read a GitHub issue/PR labels JSON array from stdin and print "true" if a
# label named <name> is present, otherwise "false". Pure jq so callers can
# pipe `gh api .../labels` into it; gh --jq does not accept jq --arg.
pr_env_label_list_has() {
  local name="$1"
  jq -r --arg name "${name}" '[.[].name] | index($name) != null'
}

# pr_env_select_latest_command
#
# Read TSV rows from stdin: created_at<TAB>user<TAB>permission<TAB>body.
# Only authorized command comments count. The latest by created_at wins.
# Prints extend, destroy, or none (no authorized command exists).
# created_at is compared as a string (RFC 3339 / GitHub ISO 8601 sorts
# lexicographically).
pr_env_select_latest_command() {
  local created user perm body cmd latest_created="" latest_cmd="none"
  while IFS=$'\t' read -r created user perm body; do
    [[ -n "${created}" ]] || continue
    pr_env_permission_is_authorized "${perm}" || continue
    cmd="$(pr_env_command_from_body "${body}")"
    [[ -n "${cmd}" ]] || continue
    if [[ -z "${latest_created}" || "${created}" > "${latest_created}" ]]; then
      latest_created="${created}"
      latest_cmd="${cmd}"
    fi
  done
  printf '%s' "${latest_cmd}"
}

# pr_env_refusal_comment <user> <command> -> acknowledgement body posted when
# an unauthorized commenter issues /pr-extend or /pr-destroy. The workflow
# must not act silently.
pr_env_refusal_comment() {
  local user="$1" command="$2"
  cat <<EOF
Refused \`${command}\` from @${user}: this command requires write, maintain, or admin permission on the origin repository. The pull request's retained state is unchanged.
EOF
}

# pr_env_is_reapable <name> <owned> <env-id> <expires-at> [now-epoch]
#
# The single reaper match predicate (Timebox and Reaping requirement). Returns 0
# (delete this namespace group) only when ALL hold:
#   - name is prefixed hypershell-ci-pr-
#   - name is not a reserved cluster namespace
#   - hypershell.redhat.io/owned == true
#   - hypershell.redhat.io/environment == pr-<number>
#   - hypershell.redhat.io/expires-at is present, parseable, and has passed
# Anything else (local openshift-up envs, unlabeled namespaces, an env id that is
# not pr-*, a still-in-the-future or missing expiry) is retained. Because every
# deploying run refreshes expires-at, an actively worked pull request never
# satisfies the expiry clause and is never reaped mid-flight.
pr_env_is_reapable() {
  local name="$1" owned="$2" env_id="$3" expires_at="$4"
  local now="${5:-$(pr_env_now_epoch)}"
  [[ "${name}" == "${PR_ENV_NS_PREFIX}"* ]] || return 1
  if pr_env_is_reserved_namespace "${name}"; then
    return 1
  fi
  [[ "${owned}" == "true" ]] || return 1
  [[ "${env_id}" =~ ^pr-[0-9]+$ ]] || return 1
  local exp
  exp="$(pr_env_rfc3339_to_epoch "${expires_at}")" || return 1
  [[ -n "${exp}" ]] || return 1
  (( now >= exp ))
}

# pr_env_is_pr_platform_namespace <name> - true for hypershell-ci-pr-<digits>
# only, not the companion -keycloak namespace.
pr_env_is_pr_platform_namespace() {
  [[ "$1" =~ ^hypershell-ci-pr-[0-9]+$ ]]
}

# pr_env_should_reap_instance_workload <workload-ns> <instance> <platform-exists>
#
# True when a control-plane-managed namespace is leftover from a pull-request
# platform project that no longer exists. platform-exists is the string "true"
# when kubectl can still get that instance's platform namespace. Local
# openshift-up instances (alice, hyp4, hyp5) and a still-live PR platform are
# retained.
pr_env_should_reap_instance_workload() {
  local workload="$1" instance="$2" platform_exists="$3"
  pr_env_is_pr_platform_namespace "${instance}" || return 1
  [[ "${platform_exists}" == "true" ]] && return 1
  [[ "${workload}" != "${instance}" ]] || return 1
  [[ "${workload}" != "${instance}-keycloak" ]] || return 1
  if pr_env_is_reserved_namespace "${workload}"; then
    return 1
  fi
  return 0
}

# pr_env_run_link
#
# Markdown link to the GitHub Actions run currently posting the comment, so a
# developer watching /pr-extend (or a synchronize deploy) can jump straight to
# its logs instead of hunting the Actions tab for an issue_comment-triggered
# run, which never surfaces as a PR check. Reads the default GITHUB_SERVER_URL
# / GITHUB_REPOSITORY / GITHUB_RUN_ID env vars every job gets for free; empty
# outside a run (e.g. under test) so callers must tolerate a blank result.
pr_env_run_link() {
  if [[ -z "${GITHUB_RUN_ID:-}" || -z "${GITHUB_REPOSITORY:-}" ]]; then
    return 0
  fi
  printf '[Track this deploy](%s/%s/actions/runs/%s)' \
    "${GITHUB_SERVER_URL:-https://github.com}" "${GITHUB_REPOSITORY}" "${GITHUB_RUN_ID}"
}

# pr_env_comment_access_facts <body>
#
# Print the access-fact table and everything after it from an existing marked
# comment, or return non-zero when the body has no table. Namespaces, console
# URL, API Route URL, web-console Route URL, Keycloak hypershell-realm admin
# console URL, and the CLI login do not change from reconcile to reconcile, so
# a later deploying edit keeps this block instead of replacing the comment
# with the first-deploy placeholder.
pr_env_comment_access_facts() {
  local body="${1:-}"
  local prefix="${body%%"| Fact | Value |"*}"
  [[ "${prefix}" != "${body}" ]] || return 1
  printf '%s' "| Fact | Value |${body#*"| Fact | Value |"}"
}

# pr_env_comment_lifetime <retained>
#
# Lifetime paragraph for access comments. Unretained environments advertise
# /pr-extend (keep if still up, redeploy if already destroyed) and state they
# are destroyed once e2e testing concludes. Retained environments are renewed
# on each commit and reclaimed after the inactivity timebox; the paragraph
# includes the UTC expiry so the comment matches the stamped
# hypershell.redhat.io/expires-at. PR_ENV_EXPIRES_AT overrides the computed
# timestamp so the deploying comment, the namespace stamp, and the ready
# comment all show the same instant.
pr_env_comment_lifetime() {
  if [[ "${1:-}" == "true" ]]; then
    local expires_at="${PR_ENV_EXPIRES_AT:-$(pr_env_expires_at_hours "${PR_ENV_RETAINED_MAX_HOURS}")}"
    cat <<EOF
This environment is retained and renewed on every commit. It is reclaimed after the inactivity timebox (\`${expires_at}\` UTC) unless you comment \`/pr-destroy\` or the pull request is closed.
EOF
  else
    cat <<EOF
Comment \`/pr-extend\` to keep this environment active. Otherwise it is destroyed once e2e testing concludes. If it has already been destroyed, \`/pr-extend\` redeploys it.
EOF
  fi
}

# pr_env_comment_deploying_body <head-sha> [existing-body] [retained]
#
# Render the in-progress comment a deploy run posts immediately on start,
# before cluster login, deploy, or e2e. Carries the same hidden marker as
# pr_env_comment_body, so the later "ready" update edits this comment in
# place rather than posting a second one. Always states how to /pr-extend
# (or that the environment is already retained) so the first comment a
# developer sees is not silent about lifetime.
#
# First deploy (no existing-body, or an existing-body with no access-fact
# table): a placeholder with the head SHA and no access facts. That is
# normally the first comment this workflow ever adds, which keeps the access
# comment near the top of the pull request's timeline.
#
# Later reconcile (existing-body already has the access-fact table): the
# heading states the environment is updating to the new commit, and the
# existing table is retained so login details stay visible while the swap
# runs.
pr_env_comment_deploying_body() {
  local head_sha="$1"
  local existing="${2:-}"
  local retained="${3:-false}"
  local short_sha="${head_sha:0:7}"
  local lifetime run_line link facts=""
  lifetime="$(pr_env_comment_lifetime "${retained}")"
  run_line=""
  if link="$(pr_env_run_link)" && [[ -n "${link}" ]]; then
    run_line=$'\n\n'"${link}"
  fi
  if facts="$(pr_env_comment_access_facts "${existing}")"; then
    cat <<EOF
${PR_ENV_COMMENT_MARKER}
## HyperShell environment updating to commit \`${short_sha}\`

Updating the ephemeral OpenShift environment to commit \`${short_sha}\`. The environment may not be fully responsive during the update. This comment will update in place once the environment is ready.${run_line}

${lifetime}

${facts}
EOF
    return 0
  fi
  cat <<EOF
${PR_ENV_COMMENT_MARKER}
## HyperShell environment deploying

Deploying commit \`${short_sha}\` to an ephemeral OpenShift environment. This comment will update in place once the environment is ready.${run_line}

${lifetime}
EOF
}

# pr_env_comment_body <pr-number> <head-sha> <platform-ns> <keycloak-ns> \
#                     <console-url> <api-url> <web-url> <cluster-api-url> \
#                     <updated> [retained]
#
# Render the pull-request access comment (Pull-Request Comment requirement).
# Carries the hidden marker so later runs find and update this comment, presents
# the same non-secret access facts `make openshift-up` prints, and contains no
# credential -- the `oc login` template uses `--web` against the OpenShift
# cluster API (not the HyperShell API Route) so OpenShift handles token
# retrieval interactively. <updated> is "true" for the per-commit update
# wording, "false" for the initial comment. <retained> is "true" when the
# pull request carries pr-environment/pr-extended.
# KEYCLOAK_URL (optional env) is the Keycloak Route origin; when set, the
# table includes the hypershell-realm admin console (GitHub login +
# impersonation), not master-realm /admin/.
pr_env_comment_body() {
  local pr_number="$1" head_sha="$2" platform_ns="$3" keycloak_ns="$4"
  local console_url="$5" api_url="$6" web_url="$7" cluster_api_url="$8" updated="$9"
  local retained="${10:-false}"
  local short_sha="${head_sha:0:7}"
  local heading lifetime kc_url kc_admin_row
  if [[ "${updated}" == "true" ]]; then
    heading="HyperShell environment updated to commit \`${short_sha}\`"
  else
    heading="HyperShell environment ready"
  fi
  lifetime="$(pr_env_comment_lifetime "${retained}")"
  kc_url="${KEYCLOAK_URL:-}"
  kc_url="${kc_url%/}"
  kc_admin_row=""
  if [[ -n "${kc_url}" ]]; then
    kc_admin_row="| Keycloak admin console | ${kc_url}/admin/hypershell/console/ |"
  fi
  cat <<EOF
${PR_ENV_COMMENT_MARKER}
## ${heading}

This pull request has a live ephemeral OpenShift environment running commit \`${short_sha}\`.

${lifetime}

| Fact | Value |
|------|-------|
| Namespaces | Platform: \`${platform_ns}\` Keycloak: \`${keycloak_ns}\` |
| OpenShift console | ${console_url} |
| API | ${api_url} |
| Web console | ${web_url} |
${kc_admin_row}

Log in through the web console with your GitHub account (you must be a member of the configured organization or on its allowlist). To test as \`developer\` or \`platform-admin\`, open the Keycloak admin console, sign in with GitHub, and impersonate that user.

<details><summary>CLI access</summary>

\`\`\`
oc login --server=${cluster_api_url} --web
\`\`\`

</details>
EOF
}

# pr_env_comment_destroyed_body
#
# Render the marked comment after the environment has been torn down (in-run
# unretained teardown or /pr-destroy). Keeps the one access comment current
# so it does not keep claiming a live environment, and tells the developer
# to /pr-extend to redeploy.
pr_env_comment_destroyed_body() {
  cat <<EOF
${PR_ENV_COMMENT_MARKER}
## HyperShell environment destroyed

This ephemeral OpenShift environment has been destroyed. Comment \`/pr-extend\` to redeploy it.
EOF
}
