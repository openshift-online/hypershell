# Persona: Team developer

This persona represents a developer or technical lead responsible for shared agent workflows, team access, and automation. They bridge day-to-day development needs and platform expectations.

## Snapshot

| Attribute | Working assumption |
| --- | --- |
| Representative roles | Technical lead, staff engineer, ML platform engineer, DevOps engineer, or CI owner |
| Working style | Shared development environments, onboarding, and production-like automation |
| Primary interface | HyperShell web console, OpenShell CLI, CI platform, and secret manager |
| Likely access | Owner of shared gateways and manager of gateway-scoped service accounts |
| Technical preference | Technical and team-management details visible |

## What they're trying to do

- Provision a gateway with the placement needed by the team
- Give team members viewer or owner access
- Create a repeatable setup developers can follow
- Create and rotate service accounts used by CI or shared automation
- Keep gateway configuration and access understandable during handoffs
- Diagnose failures before escalating to a platform administrator

## Specific examples

| Example | Successful outcome |
| --- | --- |
| Onboard a development squad | The lead grants access and shares one reliable CLI setup path without sharing secrets |
| Add agent validation to CI | A gateway-scoped service account authenticates without a browser login and uses least privilege |
| Rotate automation credentials | The replacement account is tested in CI before the old account is revoked |
| Move ownership during a reorganization | Another owner can manage access and automation before the handoff completes |

## What they know

- CLI workflows, CI/CD, secret storage, and credential rotation
- Team ownership and code-review practices
- OAuth or OIDC flows and least-privilege access
- Cloud providers, networking, and possibly Kubernetes
- The operational impact of changing shared automation

## What they may not know

- Which managed cluster HyperShell will select for a placement intent
- Which gateway failures require platform-admin access
- Whether team access maps to users, groups, or a future team object
- Which service-account metadata remains available after the secret is shown once
- The platform's retention, audit, and notification policies

## What they care about

- Shared setup that behaves the same for every developer
- Least-privilege roles for people and automation
- Safe credential replacement without CI downtime
- Clear ownership, audit context, and offboarding
- Reliable gateways and actionable failure information
- Avoiding one person's account becoming a permanent team dependency

## Likely friction

- Per-user access management doesn't scale well for larger teams
- Team ownership is unclear if the gateway always belongs to its creator
- Service-account expiration can break automation without advance warning
- Platform-admin and gateway-owner responsibilities may overlap in confusing ways

## What HyperShell should prioritize

- Make gateway access and owner handoff visible
- Support group-based access when the identity system can provide it
- Show service-account creator, role, status, expiration, and subject
- Guide replacement in the order: create, update CI, verify, then revoke
- Expose enough gateway detail for first-line troubleshooting
- Keep platform-only actions clearly separate from gateway-owner actions

## Questions to validate

- Are team developers usually gateway owners, or does a separate lead own shared gateways?
- How many people and automation identities typically share one gateway?
- Which CI systems and secret managers need setup examples?
- Who should receive credential-expiration and gateway-health notifications?

## Related flows

See [User flows](../user_flows.md), especially provisioning and sharing a team gateway, connecting with the CLI, and managing service-account credentials.
