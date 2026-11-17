# ADR-0016: Gateway Reconcile Concurrency design decisions

Rationale relocated from [`specs/platform/gateway-reconcile-concurrency.spec.md`](../../specs/platform/gateway-reconcile-concurrency.spec.md) (GTW-CONC). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
| --- | --- |
| Expose worker count as an environment variable, not a flag or code constant | Matches the existing control-plane configuration pattern (`GATEWAY_NAMESPACE_GC_*`); config changes must not require a code change |
| Keep a bounded positive default equal to today's behavior | Zero-surprise upgrade: an unset variable reconciles exactly as before |
| Invalid value warns and falls back rather than failing startup | Consistent with `getEnvBool`/`getEnvDuration`; a mistuned knob should not take the control plane down; a wrong worker count only mis-sizes a throttle |
| Pool stays a bounded throttle, never unbounded | Unbounded parallelism could overwhelm target-cluster scheduling capacity and worsen `Degraded` outcomes; the original request was explicitly "parallel **but throttled**" |
| Raising concurrency only, not reducing per-provision time | Per-provision cost is environment-driven (cluster image pull, scheduling), orthogonal to how many run at once |
| Multi-replica advisory lock deferred | The in-process queue already serializes per gateway within one process; cross-replica exclusion is a separate HA decision and the deployment runs a single replica |

## Additional rationale

- Before this change the gateway reconcile queue drained through a fixed pool of worker goroutines whose size was a hardcoded constant (`gatewayReconcileWorkers = 4` in `components/control-plane/internal/watcher/requeue.go`), overridable only by a test-only option.
- When more gateways than the worker count are created in a short window, the surplus waits in the queue until a worker frees, so provisioning wait time grows with burst size - the behavior users reported for a burst of six gateways, which motivated making the pool size configurable.
