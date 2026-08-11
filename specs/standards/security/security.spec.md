# Security Standards

**When to load:** Working on authentication, authorization, or handling sensitive data.

## Critical Security Rules

### Secret Handling

**1. No Secrets in Logs or Responses**

```go
// FORBIDDEN
log.Printf("Kubeconfig: %s", kubeconfigSecret)
log.Printf("Connection string: %s", connSecret)

// REQUIRED
log.Printf("Secret reference: %s (len=%d)", secretName, len(secretValue))
```

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

**Exception:** Third-party database images (e.g. upstream `postgres`) that run as
root by default are exempt from `runAsNonRoot`. The gateway database manifest omits
this constraint so operators can choose any compatible image via `HYPERSHELL_DATABASE_IMAGE`.
When using images that support non-root (such as Red Hat Hardened Images), configure
`runAsNonRoot` at the pod or namespace level instead.

### Fleet Isolation

Resources are scoped to fleets via `fleet_id`. All queries MUST include fleet scoping to prevent cross-tenant data access.

#### Scenario: Cross-Fleet Access Prevention
- GIVEN a user querying gateways for Fleet A
- WHEN the query does not include `fleet_id` filtering
- THEN the system SHALL reject the request or apply implicit fleet scoping

### Gateway Owner Isolation

Gateways are scoped to their creating user via the `owner` field. All gateway queries MUST include owner scoping so users can only see and operate on gateways they created. See [`platform/openshell-gateway-keycloak.spec.md`](../../platform/openshell-gateway-keycloak.spec.md) for provisioning and visibility details.

#### Scenario: Cross-User Gateway Access Prevention
- GIVEN user A querying gateways
- WHEN user B owns a gateway
- THEN user A's query SHALL NOT return user B's gateway
- AND direct access by ID SHALL return 404 (not 403) to avoid revealing existence

## Security Checklist

**Secrets:**
- [ ] No secrets in logs or error messages
- [ ] Secrets stored as K8s Secret references
- [ ] Secret values never returned in API responses

**Input:**
- [ ] All user input validated
- [ ] Resource names validated (K8s DNS label format)
- [ ] Log injection prevented

**Containers:**
- [ ] SecurityContext set on all pods
- [ ] AllowPrivilegeEscalation: false
- [ ] Capabilities dropped (ALL)
