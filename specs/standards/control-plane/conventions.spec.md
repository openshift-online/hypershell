# Control Plane Development Context

**When to load:** Working on the control plane, reconciliation logic, or gRPC watch streams.

## Quick Reference

- **Language:** Go 1.25+
- **Pattern:** gRPC watch-stream reconciler (watches the API server via gRPC streams, not controller-runtime)
- **Primary Files:** `components/control-plane/internal/reconciler/reconciler.go`, `components/control-plane/internal/watcher/watcher.go`
- **Behavior contract:** [`reconciliation-contract.spec.md`](./reconciliation-contract.spec.md)

## Critical Rules

### Resource Cleanup

The control plane is responsible for cleaning up Kubernetes and external resources when a managed resource enters durable deletion. Lifecycle events schedule cleanup, but the retained deleting resource and its finalizer are authoritative.

### SecurityContext on All Pod Specs

All pod specs must include a restrictive SecurityContext:

```go
SecurityContext: &corev1.SecurityContext{
    AllowPrivilegeEscalation: boolPtr(false),
    ReadOnlyRootFilesystem:   boolPtr(false),
    SeccompProfile: &corev1.SeccompProfile{
        Type: corev1.SeccompProfileTypeRuntimeDefault,
    },
    Capabilities: &corev1.Capabilities{
        Drop: []corev1.Capability{"ALL"},
    },
},
```

### Reconciliation Error Handling

```go
if errors.IsNotFound(err) {
    log.Printf("Resource %s/%s deleted, skipping", namespace, name)
    return nil
}

if err != nil {
    return fmt.Errorf("failed to get object: %w", err)
}
```

**Key patterns:**
- `IsNotFound` -> log and skip (resource gone, no retry)
- Transient errors -> return error (triggers reconnect/retry)
- Terminal errors -> update resource status to "Failed", do not retry

### No panic() in Production

Return `fmt.Errorf` with context instead. A panic crashes the entire control plane.

### Context Propagation

Use the context from the gRPC stream, not `context.TODO()`.

## Pre-Commit Checklist

- [ ] SecurityContext on all pod specs
- [ ] Resource limits/requests on containers
- [ ] Status updated on error paths
- [ ] No `panic()` in non-test code
- [ ] Proper context propagation
- [ ] `gofmt -w .` applied
- [ ] `go vet ./...` passes
