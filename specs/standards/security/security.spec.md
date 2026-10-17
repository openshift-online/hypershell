# Security Standards

**When to load:** Working on authentication, authorization, or handling sensitive data.

## Critical Security Rules

### Secret Handling

**1. No Secrets in Logs or Routine Responses**

```go
// FORBIDDEN
log.Printf("Kubeconfig: %s", kubeconfigSecret)
log.Printf("Connection string: %s", connSecret)

// REQUIRED
log.Printf("Secret reference: %s (len=%d)", secretName, len(secretValue))
```

Routine resource responses and read endpoints SHALL NOT contain secret values.
A credential-creation or credential-replacement endpoint MAY return a newly generated
secret to the authenticated caller. It MAY return that secret only once. The response
MUST use `Cache-Control: no-store`. Logs and telemetry MUST redact the secret. The
application MUST NOT store it in a database, event stream, or query cache. Later read
responses MUST omit it.

**2. Secret References, Not Inline Secrets**

All sensitive values (kubeconfig, database credentials) SHALL be stored as Kubernetes Secret references, not inline in the database. The model stores the secret name/path, not the secret value.

**3. Secret Store Access Is Single-Directional per Component**

Access to the configured secret store SHALL be split by component and direction. The
API server SHALL be write-only to the store: it writes submitted credential material
to the store (through its `pkg/secretstore` client) and persists only the reference (a
`SecretSource` record), and it SHALL NOT read stored material back. The control plane
SHALL be read-only from the store: it reads referenced durable material (through its
read-only `pkg/secretstore` client) and uses it to realize the agent's credentials as
OpenShell **providers** on the gateway/workspace, and it SHALL NOT write material to
the store. HyperShell SHALL NOT render agent provider credentials as raw Kubernetes
`Secret`s and SHALL NOT resolve `SecretSource` material through an `ExternalSecret`
object or External Secrets Operator. This scoping applies only to
`SecretSource`/secret-store resolution and does not govern the unrelated ESO-based
subsystems (for example ephemeral-ci-secrets, ephemeral-test-credentials,
gateway-database, and the global-architecture platform secrets), which retain their own
secret mechanisms.

Raw stored material SHALL NOT be returned by any API read endpoint, SHALL NOT be
carried on any gRPC message (the `SecretSource` gRPC message has no value field), SHALL
NOT be persisted in the database, and SHALL NOT appear in logs, events, or the query
cache.

The control plane SHALL NOT mint, refresh, or rotate the usable credential and SHALL
NOT place or rotate a raw credential `Secret`. Keeping the usable credential current is
OWNED BY OpenShell, through the provider `refresh_strategy`
(`app-installation-token`, `google-service-account-jwt`, `static-token`): given the
durable material, OpenShell mints and rotates the short-lived credential itself. A
reference whose material has not yet been supplied is a benign waiting state, not a
failure: the control plane SHALL hold with a `SecretPending` condition that gates
PROVIDER CREATION, SHALL NOT configure a provider with empty material, and SHALL
converge when the material first appears (see
[`platform/agent-secret-late-binding.spec.md`](../../platform/agent-secret-late-binding.spec.md)).

**Three separate secret planes, never conflated.** The agent-secret plane (the
per-cluster store holding `secret/data/hypershell/secret-sources/*` that holds
`SecretSource` material) SHALL be kept SEPARATE from the gateway-provider Vault plane
([`platform/openshell-gateway-credentials.spec.md`](../../platform/openshell-gateway-credentials.spec.md))
and from the application-secret plane (Secrets Manager plus External Secrets Operator).
The three planes SHALL NOT share paths, policies, or identities and SHALL NOT be
conflated.

**Distributed per-cluster reads; no global runtime store.** `SecretSource` material SHALL
be read cluster-locally: the control plane reads referenced material from the LOCAL store
of the cluster the owning agent is assigned to and uses it to configure that agent's
providers. There SHALL be no hub staging store, no material transfer through the hub, and
no global runtime reader that every control plane consults.

**Scoped per-cluster writer.** Uploading material into a cluster's agent-secret store
SHALL use a per-cluster, least-privilege WRITER identity authorized only to write under
`secret/data/hypershell/secret-sources/*` on that cluster. The writer SHALL NOT be
granted read access to provider material and SHALL NOT be granted write access to the
gateway-provider Vault plane.

#### Scenario: Store access respects component direction

- GIVEN a user submits credential material to the API server
- WHEN the API server stores it
- THEN the API server SHALL write the material to the store and persist only a `SecretSource` reference
- AND the API server SHALL NOT read the material back from the store
- AND when the control plane provisions a resource that references the `SecretSource`, it SHALL read the material through its read-only store client and realize an OpenShell provider from it, rendering no raw credential `Secret` and using no `ExternalSecret`

#### Scenario: Agent material is read cluster-locally and the writer is confined

- GIVEN an `AgentRuntime` placed on cluster `Y` with a `SecretSource` reference
- WHEN its material is uploaded by the per-cluster writer identity and later consumed
- THEN the material SHALL be written and read only within cluster `Y`'s local agent-secret store, never a hub or global store
- AND the writer identity SHALL be permitted to write only under `secret/data/hypershell/secret-sources/*` and SHALL NOT read provider material or write the gateway-provider Vault plane

#### Scenario: Rotation is owned by OpenShell, not the control plane

- GIVEN an agent credential realized as an OpenShell provider from stored durable material
- WHEN the short-lived usable credential expires
- THEN OpenShell SHALL mint and rotate it under the provider `refresh_strategy`
- AND the control plane SHALL NOT mint, refresh, or rotate the credential and SHALL NOT render or re-place a raw credential `Secret`

### Input Validation

**1. Validate All User Input**

```go
if !isValidK8sName(name) {
    return fmt.Errorf("invalid name format: must be a valid K8s DNS label")
}
```

**2. Sanitize for Log Injection**

```go
name = strings.ReplaceAll(name, "\n", "")
name = strings.ReplaceAll(name, "\r", "")
```

### Container Security

All containers must set:
- `AllowPrivilegeEscalation: false`
- `Capabilities.Drop: ["ALL"]`
- `runAsNonRoot: true`

**Note:** HyperShell runs no PostgreSQL containers. The API server, Keycloak, and
per-gateway databases live on externally provisioned servers outside the cluster, so
no database container security configuration is needed on the HyperShell side.
Development and CI stand-in PostgreSQL servers simulate those external servers and
are not HyperShell components. The control plane's admin connection to the gateway
database server always uses `sslmode=verify-full` with an operator-supplied CA
bundle; there is no TLS downgrade setting for it. Gateway workloads connect to their
own database with `sslmode=require` instead, because the upstream OpenShell Helm
chart has no mechanism to mount a CA bundle into the gateway pod for this connection
(see
[`openshell-gateway-database.spec.md`](../../platform/openshell-gateway-database.spec.md)).

### Gateway Access Isolation

Gateways are scoped by per-gateway RBAC RoleBindings (`gateway:owner`, `gateway:admin`, `gateway:viewer`). All gateway queries MUST filter by the caller's RoleBindings so users can only see and operate on gateways where they have a binding. See [`security/rbac-enforcement.spec.md`](../../security/rbac-enforcement.spec.md) for the scope-aware RBAC model and [`platform/openshell-gateway-keycloak.spec.md`](../../platform/openshell-gateway-keycloak.spec.md) for the Keycloak OIDC role bridge.

#### Scenario: Cross-User Gateway Access Prevention
- GIVEN user A querying gateways
- WHEN user A has no RoleBinding on a gateway
- THEN the query SHALL NOT return that gateway
- AND direct access by ID SHALL return 404 (not 403) to avoid revealing existence

## Security Checklist

**Secrets:**
- [ ] No secrets in logs or error messages
- [ ] Secrets stored as K8s Secret references
- [ ] Secret values absent from routine API responses
- [ ] One-time credential responses use `no-store`, redact telemetry, and never persist plaintext
- [ ] Secret store access is single-directional: API server write-only, control plane read-only; no `ExternalSecret`/ESO resolution; no raw credential `Secret` rendered for provider material
- [ ] Agent credentials realized as OpenShell providers; OpenShell owns rotation; control plane never mints/refreshes/rotates the usable credential; `SecretPending` gates provider creation
- [ ] Agent-secret, gateway-provider, and application-secret planes kept separate; agent-secret reads are cluster-local (no hub/global store); writer scoped to `secret/data/hypershell/secret-sources/*`

**Input:**
- [ ] All user input validated
- [ ] Resource names validated (K8s DNS label format)
- [ ] Log injection prevented

**Containers:**
- [ ] SecurityContext set on all pods
- [ ] AllowPrivilegeEscalation: false
- [ ] Capabilities dropped (ALL)
