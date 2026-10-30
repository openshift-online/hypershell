# Cross-Cutting Platform Conventions

Conventions that apply across all HyperShell components.

## Image references must match across the stack

Every image name and tag used in manifests, env vars, kustomization overlays, kind load commands, and Makefile targets must resolve to the same artifact.

After changing an image name or tag, grep all overlays, Makefiles, and deployment manifests. Mismatches cause silent deployment failures.

## Reconcile, don't create-or-skip

Operator and backend code that creates K8s resources must use update-or-create (reconcile) patterns, not create-and-ignore-`AlreadyExists`.

**Pattern to avoid**:
```go
err := client.Create(ctx, obj)
if apierrors.IsAlreadyExists(err) {
    return nil // BAD: silently skips spec updates
}
```

**Correct pattern**:
```go
existing := &v1.Resource{}
err := client.Get(ctx, key, existing)
if apierrors.IsNotFound(err) {
    return client.Create(ctx, obj)
}
existing.Spec = obj.Spec
return client.Update(ctx, existing)
```

## Never silently swallow partial failures

Every error path must propagate or explicitly log the failure. If a step can fail independently, collect errors and return them together.

```go
var errs []error
for _, item := range items {
    if err := reconcile(item); err != nil {
        errs = append(errs, fmt.Errorf("reconcile %s: %w", item.Name, err))
    }
}
return errors.Join(errs...)
```

## Database migrations are raw SQL, never AutoMigrate

Every plugin migration creates and evolves its table with raw SQL through
`tx.Exec`, using idempotent statements (`CREATE TABLE IF NOT EXISTS`,
`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`). GORM `AutoMigrate` is forbidden: it
can fail closed on the production PostgreSQL driver and protocol (`lib/pq` with
`PreferSimpleProtocol`), which poisons the transaction and aborts every
subsequent query on the table (`SQLSTATE 25P02`). The rule is uniform across all
Kinds; the Kind conformance gate rejects `AutoMigrate`. See
`standards/sdlc/kind-conformance.spec.md` (SDLC-CONF-02).

**Pattern to avoid**:
```go
db.AutoMigrate(&Resource{}) // BAD: fails closed on lib/pq + PreferSimpleProtocol
```

**Correct pattern**:
```go
tx.Exec(`CREATE TABLE IF NOT EXISTS resources (...)`)
tx.Exec(`ALTER TABLE resources ADD COLUMN IF NOT EXISTS field text`)
```

## Restricted SecurityContext on all containers

All containers in manifests must set:
- `runAsNonRoot: true`
- `capabilities.drop: ["ALL"]`
- `readOnlyRootFilesystem: true` (unless a specific write path is required)

```yaml
securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
  readOnlyRootFilesystem: true
  seccompProfile:
    type: RuntimeDefault
```
