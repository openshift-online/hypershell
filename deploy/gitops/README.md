# Platform and application GitOps packages

These packages support the two Argo CD instances introduced by
[hypershell-gitops PR #285](https://github.com/openshift-online/hypershell-gitops/pull/285).
This is a manifest refactor, not a live deployment or ownership migration.

| Entry point | Intended owner | Resources |
| --- | --- | --- |
| `deploy/gitops/applications` | Application Argo CD | API server and web console Deployments, their ServiceAccounts/Services, and API Route |
| `deploy/gitops/platform` | Platform Argo CD | Namespace, privileged controller and its identity/RBAC, SCC bindings, monitoring, shared certificates and network policies |
| `deploy/hub` | Existing combined deployment owner | Both packages, preserving the existing deployment |

Monitoring stays platform-owned because it includes cluster-wide permissions
and node-exporter hostPath mounts. Shared manifests are referenced, not copied.

## Consume from hypershell-gitops

Use separate environment overlays referencing these sources at the **same full
commit SHA**:

```text
github.com/openshift-online/hypershell/deploy/gitops/platform?ref=<sha>
github.com/openshift-online/hypershell/deploy/gitops/applications?ref=<same-sha>
```

Keep environment patches, compatible image digests, credentials, and Argo
Applications in `hypershell-gitops`. Split controller/platform patches from
API/console patches; do not reuse the combined overlay unchanged for both.
Operators/CRDs, databases, Keycloak, shared ingress, and credential delivery
remain external prerequisites. These packages do not install them.

## Compatibility and safety

- Existing `deploy/base`, `deploy/hub`, Kind, OpenShift, and IBM directory-based
  consumers still render the complete deployment.
- Raw-file consumers need updated paths. API server and console manifests moved
  from `deploy/base/` to `deploy/base/applications/`. Controller, controller RBAC,
  namespace, and network-policy files moved to `deploy/base/platform-resources/`.
  The hub Route and SCC files moved to their respective `deploy/gitops/` packages.
  Executable repository references are updated; older comments/specs may still
  name the original paths.
- Do not reconcile `deploy/hub` alongside either split package for the same
  instance. Before adoption, compare final environment renders, quiesce old
  owners, and orphan-delete their Applications without deleting live resources.
- Platform prerequisites must be ready before the initial application sync.
  Sync waves do not coordinate independent Argo instances.
- **This split is not runtime isolation.** Resources still share the existing
  namespace by default. Broad Pod/Secret access can expose the controller's
  privileged identity or credentials. Agree on namespace separation or suitable
  admission/access controls before granting application Argo access.

## Validation

With Kustomize v5 and Python/PyYAML installed:

```sh
python3 scripts/test_gitops_manifests.py
```

Four render-only checks cover approved application resources, disjoint ownership,
equality of the split composition and hub, and existing entry-point builds.
No Kind cluster or live Kubernetes API is used. CI runs the same checks.
