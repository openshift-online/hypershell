# ADR-0024: Watch Delete Events design decisions

Rationale relocated from [`specs/platform/watch-delete-events.spec.md`](../../specs/platform/watch-delete-events.spec.md) (CTRL-WDEL). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Unscoped DAO query rather than stashing before delete | Simpler: no changes to the `Delete()` service method or HTTP handler; the watch handler is the only consumer that needs the deleted state. |
| Apply to Gateways only (initially) | Gateways are the only Kind where delete cleanup requires resource fields. Other Kinds can adopt the pattern when needed. |
| Nil-payload delete re-reads rather than degrading to a warning | A nil payload is reachable in normal operation (unscoped subscribers receive nil-payload deletes), so treating it as a best-effort warning silently orphans the gateway's database and Keycloak clients, which have no namespace-GC recovery path. The control plane re-reads the deleting record with the unscoped query and reconciles from it; when the record cannot be loaded it returns a retryable error and records the no-silent-orphan signal, consistent with GTW-DEL-04 and GTW-DEL-07. |

## Additional rationale

- Why the full resource snapshot is needed in delete events: gRPC watch streams historically carried only the `resource_id` on a delete event, with a nil resource payload. That forced the control plane to maintain in-memory caches (for example the gateway-to-namespace map) to perform cleanup. On a control-plane restart those caches are lost and cleanup is silently skipped, which is the gap this spec closes by delivering the pre-delete resource snapshot on the event itself.
- A nil-payload delete that is marked done by logging a warning and returning success would leave the gateway's out-of-namespace resources (its database and Keycloak clients) behind with no recovery path and no operator-visible signal; this is why CTRL-WDEL-04 requires an unscoped re-read and forbids silent success.
