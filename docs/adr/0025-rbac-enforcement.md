# ADR-0025: RBAC Enforcement design decisions

Rationale relocated from [`specs/security/rbac-enforcement.spec.md`](../../specs/security/rbac-enforcement.spec.md) (SEC-RBAC). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Control plane writes back CI status status-only, not by import or role binding (SEC-RBAC-27) | The control plane never authors `CIProvider` or `CITrust` spec, but must persist what it realized (the adopted IdP alias, the exchange endpoint). Denying every CI write stranded federation: no `status.idp_alias` meant every adopting `CITrust` failed. A field-scoped status-only write-back mirrors the existing `Gateway`/`AgentRuntime` non-import status path and the Gateway Field Write Policy, so the control plane records observed state without gaining authority to author CI spec. |
| Keycloak as authority for platform roles | Centralizes role management. Eliminates need for admin-seeding CLI or DB migration. Role changes take effect on next JWT. |
| Four roles model | Minimal model that covers the use cases: platform administration (view/delete all), create gateways, own gateways, view gateways. No fleet-scoped RBAC needed. |
| gRPC decides each method explicitly, default-deny | The previous rule granted `gateway:creator` or `gateway:owner` every gRPC method, so any signed-in user could rewrite or delete any gateway once the hub's gRPC API became public. A per-method table matches the HTTP rules and fails closed for methods added later. |
| `platform:admin` is view + delete only | Platform admins handle operational tasks (viewing all gateways, cleaning up orphaned resources). Full modification requires ownership to prevent accidental changes. Separation of concerns: visibility ≠ modification authority. |
| `platform:admin` orthogonal to `gateway:creator` | A platform admin may or may not create gateways. Roles compose: `platform:admin` + `gateway:creator` allows both operational oversight and resource creation. |
| JWT roles synced to DB on every request | DB is the projection, Keycloak is the authority. Revocations in Keycloak take effect immediately. Existing per-gateway bindings are unaffected by platform role changes. |
| Service accounts treated identically to users | Control plane gets `gateway:creator` in Keycloak, provisions like any user. No special bypass logic needed. |
| `managed-cluster-registrar` is separate from gateway roles | A spoke service account only needs fleet membership rights, not gateway creation rights. Keeping the roles separate limits blast radius if a spoke credential is compromised. |
| `gateway:dedicated-route-creator` separate from `gateway:creator` | A dedicated per-gateway route and certificate is a rate-limited resource (one certificate order, load balancer, and wildcard DNS record per gateway). A distinct platform-wide entitlement lets operators grant it narrowly instead of to every creator; it is a marker role checked at the create boundary and grants nothing else (SEC-RBAC-20). |
| `managed-cluster-registrar` is JWT-direct, not DB-synced | The role is for specific service accounts, not users in general. There is no reason to maintain a DB binding for it. Checking from the live JWT claim in `isAuthorized` is sufficient, consistent with other JWT-direct roles, and avoids `JWTSyncedRoles` entanglement. |
| Role assigned by admin, not auto-granted | Provides a human control point for fleet membership. A new spoke cannot join the fleet without an explicit Keycloak admin action. |
| Gateway owners can grant co-owners | No hierarchy restriction. Team leads assign `gateway:creator` to team members or invite them as owners/viewers per gateway. Simple mental model. |
| Auto-assign `gateway:owner` on creation | Creator automatically owns what they create. No separate grant step needed. |
| `gateway:creator` assigned exclusively via Keycloak | Every deployment sources the role only from Keycloak, directly or through the realm default role for a blanket grant (SEC-RBAC-13). The role cannot be self-assigned via the API. |
| Per-gateway bindings stored in DB | Gateway-scoped access requires per-resource granularity that JWT claims cannot provide (you'd need dynamic claim values per gateway ID). |
| 404 on unauthorized singleton GETs | Returning 403 confirms the resource exists. 404 prevents ID enumeration. |
| OpenShell role bridge | `gateway:owner` maps to `openshell-admin`, `gateway:viewer` maps to `openshell-user`. Ensures consistent access via CLI. |
| OpenShellGatewayServiceAccount role limit | A gateway binding limits the selected OpenShell role. Owners can select `openshell-user` or `openshell-admin`. Viewers can select only `openshell-user`. Each OpenShellGatewayServiceAccount remains bound to its creator. A binding change can downgrade or revoke it. |

## Additional rationale

- JWT roles are synced lazily on each request so Keycloak role changes take effect on the next API request without a separate sync process.
- Auto-provisioning writes a caller's profile row only when a claim actually changed so the caller's own row is not locked on every request (including the control plane's gRPC write-backs), keeping a slow downstream call in one request from stalling that caller's other concurrent requests.
- The gateway field write policy is enforced in the service funnel rather than per handler to close the whole-row-replace gap (a gRPC `UpdateGateway` could set more than a REST `PATCH`) and the import loophole (a GitOps manifest could declare platform-managed fields the direct user API forbids).
