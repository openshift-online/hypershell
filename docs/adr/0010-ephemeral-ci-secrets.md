# ADR-0010: Ephemeral CI Secrets design decisions

Rationale relocated from [`specs/platform/ephemeral-ci-secrets.spec.md`](../../specs/platform/ephemeral-ci-secrets.spec.md) (CICD-SEC). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Secondary spec, not a fold-in to test-tier credentials | Test-tier passwords, cluster kubeconfig, and the GitHub OAuth App are different credential classes with different consumers. One spec per class keeps rotation and IAM boundaries reviewable |
| AWS Secrets Manager, not GitHub Actions secrets | Actions secrets do not rotate with the fleet, are a second copy of cluster and OAuth material, and cannot be the in-cluster source Keycloak needs. The cloud operator's store already backs ESO on this cluster |
| GitHub OIDC into IAM, no static AWS keys in Actions | Same issuer already used from Actions in `hypershell-gitops`. Removing standing secrets from GitHub is wasted if the replacement is a long-lived AWS key stored as an Actions secret |
| Fork deny is an origin-repo check on OpenShift jobs, not a second E2E caller | GitHub's default `pull_request` subject does not encode fork vs origin, so IAM `sub` is not a fork filter. One Tests / E2E job keeps Kind on forks. Cluster login and deploy skip or refuse when `head.repo.full_name` is not the origin. Withholding `id-token` via a duplicate caller is later ok-to-test work |
| CI IAM reads only `hysh-aws-01/ci/cluster-login` | OAuth is ESO-only. Fetching the App secret onto the runner re-creates the Actions-secret blast radius this spec removes |
| Runner-only vs in-cluster split | The cluster login must never land in a public-internet PR namespace. The OAuth App secret must land in Keycloak. ESO for the second, OIDC fetch for the first |
| Cluster-login JSON is `server` plus `token` | One pinned shape so Terraform and the workflow cannot disagree. `oc login --server --token` is the OpenShift-native path; a kubeconfig blob is a second encoding of the same facts |
| `ClusterExternalSecret` into `hypershell-ci-pr-*-keycloak` | Keycloak namespaces are created at runtime. A cluster-scoped ESO object projects the OAuth Secret into each new companion namespace without a Git commit per pull request |
| Per-PR `hypershell-e2e` secret stays in Keycloak | A standing AWS or Actions copy cannot match a realm that is created per pull request. CI already reads it after bring-up |
| `github.token` stays GitHub-issued | It is short-lived, job-scoped, and not a standing fleet secret |
| Fail closed when OAuth material is missing | An environment nobody can log into is worse than a failed job; this is the same rule the PR-environment spec already has |
