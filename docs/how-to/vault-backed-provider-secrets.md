# How-To: Run new HyperShell deployments with Vault-backed OpenShell Provider secrets

New HyperShell service deployments store OpenShell **Provider** secrets in HashiCorp
Vault instead of persisting them in the gateway database under the upstream KEK
mechanism. The HyperShell service configures and operates the Vault integration;
newly provisioned gateways resolve their Provider secrets from Vault at runtime, and
those secret values are never persisted in the gateway database or exposed in API
responses, logs, manifests, or generated configuration. This guide is the practical
companion to the contracts in
[`specs/platform/openshell-gateway-credentials.spec.md`](../../specs/platform/openshell-gateway-credentials.spec.md)
(OSG-CRED) and the GitOps-owned
`specs/openshell-gateway-vault.spec.md` in `hypershell-gitops`; when this guide and a
spec disagree, the spec wins. The GitOps spec is authoritative for the Vault
configuration contract.

## What this is (and is not)

- **Service-operated, not user-selected.** Operators do not choose a secrets-storage
  strategy and gateways carry no per-gateway credential-driver override. The flat
  `vault` default applies to every newly provisioned gateway (PLAT-DATA-23).
- **New deployments and new gateways only.** Existing gateways keep their current
  upstream secret-storage behavior; nothing is migrated.
- **HashiCorp Vault only** for the initial implementation (OpenShell supports it).

## Operational prerequisites

These are stood up once per cluster by `hypershell-gitops` before any gateway
provisions (see the GitOps Vault spec and the `openshell-gateway-vault` base):

- A per-cluster Vault in namespace `openshell-vault`, reachable at the convention
  address `https://openshell-gateway-vault.openshell-vault.svc:8200` (one Vault per
  cluster, no per-gateway address override).
- KV v2 enabled; per-tenant mounts `openshell-gw/<tenant-namespace>` are created by
  the control plane at provision and deleted at teardown.
- Kubernetes auth mount `kubernetes`, with two roles:
  - `openshell-gateway` -- bound to namespaces labeled
    `hypershell.redhat.io/managed=true`; token scoped by a templated,
    capability-scoped policy to the gateway's own mount (`create`/`read`/`update`/`delete`
    on `.../provider-credentials/*`, `read`/`delete` on the matching `metadata/` path,
    **no `list`**, nothing outside its own mount).
  - `openshell-gateway-provisioner` -- bound to the control-plane namespace labeled
    `hypershell.redhat.io/control-plane=true`; scoped to `sys/mounts/openshell-gw/*`
    only (mount lifecycle, no `data/` access, no policy or role writes).
- A Vault serving CA published as Secret `openshell-gateway-vault-tls` (key `ca.crt`)
  in `openshell-vault`; the control plane mirrors it into each tenant namespace as
  ConfigMap `openshell-gateway-vault-ca`.

## What a correctly configured gateway looks like

A vault-backed gateway's generated `gateway.toml` selects the vault driver and carries
only Vault **references** -- never inline credential material:

```toml
credential_drivers = ["vault"]

[openshell.credential_drivers.vault]
address     = "https://openshell-gateway-vault.openshell-vault.svc:8200"
auth_method = "kubernetes"
ca_bundle   = "/etc/openshell-tls/vault-ca/ca.crt"
role        = "openshell-gateway"
```

There is **no** `[openshell.gateway.credential_storage]` stanza and **no** `in_tree`
transport (those are the KEK path that persisted Provider secrets in the database),
and **no** `openshell-gateway-credential-kek` Secret in the tenant namespace. Provider
secret values live only in Vault under
`openshell-gw/<namespace>/data/openshell/provider-credentials/<hash>`.

## Verify

```shell
# 1. The gateway reaches Running (fail-closed probes passed, serving CA mirrored).
openshell -g "$GW" status

# 2. The generated config references Vault and carries no KEK path.
kubectl -n "$NS" get configmap openshell-gateway-config -o jsonpath='{.data.gateway\.toml}' \
  | grep -E 'credential_drivers.*vault|auth_method|ca_bundle'
kubectl -n "$NS" get configmap openshell-gateway-config -o jsonpath='{.data.gateway\.toml}' \
  | grep -E 'credential_storage|in_tree' && echo "UNEXPECTED: KEK path present" || echo "OK: no KEK path"

# 3. No KEK secret is persisted in the tenant namespace.
kubectl -n "$NS" get secret openshell-gateway-credential-kek 2>/dev/null \
  && echo "UNEXPECTED: KEK secret present" || echo "OK: no KEK secret"
```

The end-to-end suite asserts the same invariants automatically: the flat-vault happy
path and provider-secret non-disclosure in P1.2 (OSG-CRED-14.03 / E2E-CORE-13.04), the
real gateway-process resolve via the sandbox lifecycle (P2.1), and fail-closed when
Vault is unreachable in P2.7 (OSG-CRED-14.02). The per-tenant capability policy is
self-verified by the Vault fixture at bring-up (OSG-CRED-14.01).

## Authentication, failure, rotation, and revocation

- **Least-privilege identity.** The gateway and the control-plane provisioner use
  separate Kubernetes-auth roles; neither holds Vault admin. Tokens are projected,
  `audience: vault`, 1-hour TTL.
- **Fail-closed.** If Vault is unreachable, a newly provisioned gateway never reaches
  `Running`; `EnvironmentReady` reports a Vault-named reason (OSG-CRED-14.02).
- **Access revocation.** Removing the `openshell-gateway-provider` policy denies the
  gateway identity (self-verified by the fixture, OSG-CRED-14.01).
- **Token revocation.** The control plane revokes its short-lived provisioner and
  probe tokens via `auth/token/revoke-self` immediately after use (OSG-CRED-10.04).
- **Value rotation.** Rotating a Provider secret's *value* is OpenShell's own
  `provider` mechanism; see
  [`specs/platform/openshell-gateway-secret-rotation.spec.md`](../../specs/platform/openshell-gateway-secret-rotation.spec.md).

## See also

- Contract: [`specs/platform/openshell-gateway-credentials.spec.md`](../../specs/platform/openshell-gateway-credentials.spec.md) (OSG-CRED)
- E2E coverage: [`specs/platform/e2e-testing.spec.md`](../../specs/platform/e2e-testing.spec.md) (E2E-CORE-13.04)
- GitOps Vault operation: `hypershell-gitops` `specs/openshell-gateway-vault.spec.md` (authoritative for the Vault configuration contract)
