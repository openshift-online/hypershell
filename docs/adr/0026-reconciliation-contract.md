# ADR-0026: Control Plane Reconciliation Contract design decisions

Rationale relocated from [`specs/standards/control-plane/reconciliation-contract.spec.md`](../../specs/standards/control-plane/reconciliation-contract.spec.md) (STD-RCON). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| One normative contract referenced by every reconciliation spec | A reader assembled the guarantees from six files and a new reconciler author had no single bar to meet. Collecting them once, with each refining spec pointing back, gives one source of truth and a clear review checkpoint. |
| Describe the real model | The system uses no Kubernetes finalizer, per-write resource-version token, or leader election. A contract must match what is built or it misleads the author it exists to guide. |
| Soft-delete plus tombstone, not a finalizer | The resource's source of truth is PostgreSQL, not a Kubernetes object, so there is no object to carry a finalizer or `deletionTimestamp`. Durability comes from the soft-deleted row, the delete event snapshot, and the queue's bounded-backoff retry with an anti-resurrection tombstone (STD-RCON-07, GTW-DEL). |
| `generation` / `observed_generation` latch, not a resource-version token | Convergence is a monotonic range check the API server enforces on write-back, which both detects drift and rejects stale or reordered updates without a separate optimistic-concurrency token (STD-RCON-06, PLAT-DATA-06). |
| Single replica; leader election out of scope | The in-process queue already serializes per key within one process, and the deployment runs one replica. Cross-replica exclusion is a separate HA decision, kept an explicit non-goal so no reconciler is written to assume a leader lease (STD-RCON-11, GTW-CONC). |
| Record known exceptions | Recording each deviation, such as the Repository-import exception, keeps a divergence a recorded decision rather than a surprise found later in review (STD-RCON-12). |
