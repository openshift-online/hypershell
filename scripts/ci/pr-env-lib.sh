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
PR_ENV_OWNED_LABEL="hypershell.redhat.io/owned"
PR_ENV_ENVIRONMENT_LABEL="hypershell.redhat.io/environment"
PR_ENV_MANAGED_LABEL="app.kubernetes.io/managed-by"
PR_ENV_MANAGED_VALUE="hypershell-lifecycle"
PR_ENV_PART_OF_LABEL="app.kubernetes.io/part-of"
PR_ENV_PART_OF_VALUE="hypershell"
PR_ENV_EXPIRES_ANNOTATION="hypershell.redhat.io/expires-at"

# Default timebox in days. The workflow overrides this from a single documented
# setting (vars.PR_ENV_TIMEBOX_DAYS); the constant keeps the default in one place.
: "${PR_ENV_TIMEBOX_DAYS:=3}"

# Stable hidden marker so later runs update the one access comment rather than
# post a new comment per run.
PR_ENV_COMMENT_MARKER='<!-- hypershell-pr-environment -->'

# pr_env_namespace <pr-number> -> the platform namespace name.
pr_env_namespace() {
  printf '%s%s' "${PR_ENV_NS_PREFIX}" "$1"
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

# pr_env_expires_at <days> [now-epoch] -> RFC 3339 UTC timestamp <days> in the
# future. now-epoch is injectable for deterministic tests.
pr_env_expires_at() {
  local days="$1"
  local now="${2:-$(pr_env_now_epoch)}"
  pr_env_epoch_to_rfc3339 $(( now + days * 86400 ))
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

# pr_env_comment_body <pr-number> <head-sha> <platform-ns> <keycloak-ns> \
#                     <console-url> <api-url> <web-url> <cluster-api-url> <updated>
#
# Render the pull-request access comment (Pull-Request Comment requirement).
# Carries the hidden marker so later runs find and update this comment, presents
# the same non-secret access facts `make openshift-up` prints, and contains no
# credential -- the `oc login` template uses `--web` against the OpenShift
# cluster API (not the HyperShell API Route) so OpenShift handles token
# retrieval interactively. <updated> is "true" for the per-commit update
# wording, "false" for the initial comment.
pr_env_comment_body() {
  local pr_number="$1" head_sha="$2" platform_ns="$3" keycloak_ns="$4"
  local console_url="$5" api_url="$6" web_url="$7" cluster_api_url="$8" updated="$9"
  local short_sha="${head_sha:0:7}"
  local heading
  if [[ "${updated}" == "true" ]]; then
    heading="HyperShell environment updated to commit \`${short_sha}\`"
  else
    heading="HyperShell environment ready"
  fi
  cat <<EOF
${PR_ENV_COMMENT_MARKER}
## ${heading}

This pull request has a live ephemeral OpenShift environment running commit
\`${short_sha}\`.

| Fact | Value |
|------|-------|
| Namespaces | Platform: \`${platform_ns}\` Keycloak: \`${keycloak_ns}\` |
| OpenShift console | ${console_url} |
| API | ${api_url} |
| Web console | ${web_url} |

Log in through the web console with your GitHub account (you must be a member of
the configured organization or on its allowlist). The environment is time-boxed
and refreshed on every new commit.

<details><summary>CLI access</summary>

\`\`\`
oc login --server=${cluster_api_url} --web
\`\`\`

</details>
EOF
}
