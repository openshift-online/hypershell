# Persona: Platform administrator

This persona represents an administrator responsible for the reliability and safe operation of HyperShell across users, gateways, and managed clusters. They support tenant workflows without owning the tenant's work.

## Snapshot

| Attribute | Working assumption |
| --- | --- |
| Representative roles | Site reliability engineer, platform engineer, cluster administrator, or service owner |
| Working style | Fleet monitoring, incident response, support, and lifecycle cleanup |
| Primary interface | HyperShell web console, operational dashboards, observability tools, and cluster tools |
| Likely access | Platform-wide gateway view and delete; separate gateway access only when explicitly granted |
| Technical preference | Operational context, ownership, placement, and diagnostics visible |

## What they're trying to do

- Understand overall platform health
- Find degraded, failed, or stuck gateways quickly
- Identify a gateway's owner, creator, placement, release, and current use
- Distinguish a tenant problem from a platform problem
- Contact the right owner with useful context
- Delete abandoned, unsafe, or unrecoverable gateways
- Track provisioning reliability, API reliability, capacity, and fleet distribution

## Specific examples

| Example | Successful outcome |
| --- | --- |
| Provisioning failures increase | The admin sees the success-rate change, narrows the issue to a provider or cluster, and starts investigation |
| A gateway remains degraded | The admin finds its owner, placement, release, and diagnostics without impersonating the user |
| A cluster incident affects tenants | The admin identifies affected gateways and communicates the impact to their owners |
| An abandoned gateway consumes resources | The admin confirms its identity and owner, records the reason, and deletes it safely |

## What they know

- Kubernetes or OpenShift operations
- Metrics, logs, traces, alerts, and incident response
- Identity, role, and access-control concepts
- Cluster capacity, networking, releases, and failure domains
- The difference between platform health and tenant workload behavior

## What they may not know

- The business purpose of a gateway or sandbox
- Whether a gateway is still important to its owner
- The user's provider configuration or local CLI environment
- What data or unfinished work a deletion might affect
- Whether a tenant-visible error matches the platform symptom they see

## What they care about

- Accurate, current health data
- Fast movement from a fleet symptom to the affected resource
- Ownership and contact context
- Low-noise alerts and useful trends
- Clear authorization boundaries and audit history
- Reducing incident duration and avoiding unnecessary tenant impact

## Likely friction

- A dashboard can show a problem without providing a path to the affected gateways
- Missing owner or placement context slows support
- Platform-wide data can become difficult to search and paginate
- Delete may be the only available admin action even when a recoverable action would be safer
- Admin visibility doesn't automatically grant OpenShell access for tenant-level diagnosis

## What HyperShell should prioritize

- Put **Needs attention** and platform health at the top of the landing page
- Link summary metrics to filtered gateway or cluster views where possible
- Show owner, creator, cluster, release, endpoint, gateway ID, and timestamps
- Keep operational and reliability dashboards focused on different questions
- Provide diagnostics and runbook links without exposing secrets or tenant content
- Require clear confirmation and audit context for destructive actions

## Questions to validate

- Do administrators need one system-wide view or separate views per hub cluster?
- Which gateway actions should exist beyond view and delete?
- What owner-contact information can the application safely expose?
- Which dashboard signals should link directly to affected gateways?
- What audit note or support reference is required before deletion?

## Related flows

See [User flows](../user_flows.md), especially viewing all gateways, monitoring platform health, investigating a failed gateway, and deleting any gateway.
