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

**Note:** All PostgreSQL databases (API server and per-gateway) are provisioned by
the CloudNativePG (CNPG) operator, which manages its own PostgreSQL pods with
appropriate security contexts. The CNPG operator enforces non-root execution and
drops capabilities by default. No database container security configuration is
needed on the HyperShell side.

### Gateway Access Isolation

Gateways are scoped by per-gateway RBAC RoleBindings (`gateway:owner`, `gateway:viewer`). All gateway queries MUST filter by the caller's RoleBindings so users can only see and operate on gateways where they have a binding. See [`security/rbac-enforcement.spec.md`](../../security/rbac-enforcement.spec.md) for the scope-aware RBAC model and [`platform/openshell-gateway-keycloak.spec.md`](../../platform/openshell-gateway-keycloak.spec.md) for the Keycloak OIDC role bridge.

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

**Input:**
- [ ] All user input validated
- [ ] Resource names validated (K8s DNS label format)
- [ ] Log injection prevented

**Containers:**
- [ ] SecurityContext set on all pods
- [ ] AllowPrivilegeEscalation: false
- [ ] Capabilities dropped (ALL)
