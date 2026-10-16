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
API server SHALL be write-only to the store: it writes a submitted secret value to
the store (through its `pkg/secretstore` client) and persists only the reference (a
`SecretSource` record), and it SHALL NOT read a stored secret value back. The control
plane SHALL be read-only from the store: it reads a referenced value (through its
read-only `pkg/secretstore` client) and places it directly into a Kubernetes `Secret`
on the target cluster, and it SHALL NOT write values to the store. For the
`SecretSource`/secret-store resolution path specifically, HyperShell SHALL NOT
resolve a stored `SecretSource` value through an `ExternalSecret` object or External
Secrets Operator; the control plane reads the store and places the Kubernetes
`Secret` directly. This scoping applies only to `SecretSource`/secret-store
resolution and does not govern the unrelated ESO-based subsystems (for example
ephemeral-ci-secrets, ephemeral-test-credentials, gateway-database, and the
global-architecture platform secrets), which retain their own secret mechanisms.

A raw stored secret value SHALL NOT be returned by any API read endpoint, SHALL NOT
be carried on any gRPC message (the `SecretSource` gRPC message has no value field),
SHALL NOT be persisted in the database, and SHALL NOT appear in logs, events, or the
query cache.

Control-plane secret placement SHALL be one-shot at provision time: the control plane
reads the store and writes the Kubernetes `Secret` once when it provisions the owning
resource, and SHALL NOT run an ESO-style periodic resync or rotation of the placed
value.

#### Scenario: Store access respects component direction

- GIVEN a user submits a secret value to the API server
- WHEN the API server stores it
- THEN the API server SHALL write the value to the store and persist only a `SecretSource` reference
- AND the API server SHALL NOT read the value back from the store
- AND when the control plane provisions a resource that references the `SecretSource`, it SHALL read the value through its read-only store client and place it directly into a Kubernetes `Secret`, once, without an `ExternalSecret`

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
- [ ] Secret store access is single-directional: API server write-only, control plane read-only; no `ExternalSecret`/ESO resolution; placement is one-shot

**Input:**
- [ ] All user input validated
- [ ] Resource names validated (K8s DNS label format)
- [ ] Log injection prevented

**Containers:**
- [ ] SecurityContext set on all pods
- [ ] AllowPrivilegeEscalation: false
- [ ] Capabilities dropped (ALL)
