# HYPERSHELL-369 - Latest Plan (current source of truth)

Supersedes the earlier `HYPERSHELL-369-vault-plan.md` / `-open-questions.md` /
`-open-questions-answers.md` (deleted): those predate the hypershell-gitops PR and
their disaster-recovery/unseal approach is stale (see "What changed" below).

**Goal:** run new HyperShell gateways with their OpenShell **provider** secrets stored
in a HashiCorp Vault (behind opaque handles) instead of the gateway-DB KEK. One
dedicated Vault **per cluster** that hosts gateways, used solely for provider secrets.
The existing platform-secrets pipeline (ESO / Secrets Manager / any dev-mode Vault) is
left untouched.

## Two-repo split
- **hypershell-gitops (DONE - PR #662, validated live on hypc-ibm-00).** Stands up the
  per-cluster Vault: KV v2 at `secret`, Kubernetes auth mount `kubernetes`, shared role
  `openshell-gateway` + a namespace-templated ACL policy, a self-healing init Job,
  serving TLS, and the unsealer. Namespace `openshell-vault`.
- **hypershell (THIS work).** Wire the gateway + control plane to use that Vault: make
  `vault` the default credential driver, generate the right `gateway.toml`, propagate the
  Vault serving CA into each gateway namespace, fail closed when Vault is unreachable, and
  purge a gateway's KV path on deletion.

## What changed since the old docs (stale bits)
- **DR / persistence:** NOT KMS auto-unseal + PVC. Actual: **Shamir** seal (OSS Vault
  has no IBM Key Protect seal) + unseal key in **Secrets Manager** delivered by **ESO** +
  a small **unsealer** container; storage backend = **managed PostgreSQL (ICD)**, so DR is
  the managed DB's backups/PITR. This is entirely a gitops concern and does not affect the
  hypershell-side wiring.

## GitOps contract (what hypershell implements against - from PR #662)
| Item | Value |
|---|---|
| Address (in-cluster) | `https://openshell-gateway-vault.openshell-vault.svc:8200` |
| KV engine / mount | KV v2, mount `secret` |
| KV path layout | `secret/data/openshell/<tenant-namespace>/<provider-handle>` |
| Kubernetes auth mount | `kubernetes` |
| Vault role (shared) | `openshell-gateway` |
| Isolation | one templated policy resolves each caller's verified namespace at login |
| SA token | projected, `audience: vault`, `expirationSeconds: 3600`, at `/var/run/secrets/vault/token` |
| Serving CA source | Secret `openshell-gateway-vault-tls` (key `ca.crt`) in `openshell-vault` |

## hypershell spec changes (Step 1 - amend in place)
Primary: `specs/platform/openshell-gateway-credentials.spec.md`
1. Default: omitted `credential_driver` resolves to `vault` (was KEK). Add `kek-db` as an
   explicit third value; `kubernetes-secrets` unchanged.
2. Vault coordinates become control-plane **convention defaults** (address/mount/role/auth
   mount/kv version/SA-token path+audience), overridable; no longer required user input.
3. New requirement: **Vault serving-CA propagation** - control plane mirrors
   `openshell-gateway-vault-tls` (same cluster) into each gateway namespace as a ConfigMap
   and sets the chart's `credentialDrivers.vault.caConfigMapName`, mirroring the existing
   `reconcileTrustedCABundle` pattern.
4. Fail-closed reachability check at creation + `Degraded` at runtime + a dashboard signal.
5. Purge the gateway's KV path (`secret/.../<tenant-namespace>/*`) on deletion.
6. Secret-hygiene scenarios (no secret value in `gateway.toml`; SA token never logged),
   citing `specs/standards/security/security.spec.md`.
7. A dedicated "GitOps contract" section (the table above).

Secondary (minimal cross-ref scenarios): `gateway-provisioning-progress.spec.md` /
`openshell-gateway-health.spec.md` (reachability / `Degraded`),
`gateway-deletion-finalization.spec.md` (KV purge cleanup class), and one-line default
updates in `openshell-gateway.spec.md` / `data-model.spec.md`.

## Design decisions (resolved)
- **Vault coordinates = conventions, overridable** (makes "vault by default" work with no
  user input).
- **CA propagation is same-cluster** - the control plane provisions gateways on its own
  cluster (single clientset; no remote/spoke client exists in the control-plane code), so
  it reads the Vault CA locally and mirrors it into the tenant namespace.
- **Behavior placement** - describe reachability/Degraded and KV-purge fully in the
  credentials spec with scenarios; add only minimal cross-ref scenarios to the owner specs.

## Out of scope (does not close the Jira)
Vault HA, credential migration between drivers, rotating the provider secret *values*
(OpenShell's own `provider refresh`), and rollout to clusters beyond the first.
