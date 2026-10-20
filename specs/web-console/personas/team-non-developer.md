# Persona: Team non-developer

This persona represents a team lead or contributor who uses shared agent environments but doesn't manage the underlying platform. This is a role-based persona, not a demographic profile.

## Snapshot

| Attribute | Working assumption |
| --- | --- |
| Representative roles | Research lead, operations lead, support lead, content team lead, or project coordinator |
| Working style | Shared workflows, handoffs, and repeatable team access |
| Primary interface | HyperShell web console and OpenShell console |
| Likely access | Viewer or owner of one or more shared gateways |
| Technical preference | Team and access context shown; infrastructure details hidden by default |

## What they're trying to do

- Find the gateway used by their team
- Open the OpenShell console and start team work
- Request or create a shared gateway
- Add team members with the right level of access
- Understand who owns a gateway and who can fix access problems
- Keep a shared workflow available when team membership changes

## Specific examples

| Example | Successful outcome |
| --- | --- |
| Set up a shared research environment | The team has one clearly named gateway and members can access it without sharing credentials |
| Onboard a new team member | An owner grants viewer access and the new member can open the console |
| Hand ownership to another lead | The new owner can manage access before the previous owner leaves the project |
| Respond to a team outage | The user sees that the gateway is degraded and knows whether to retry or contact platform support |

## What they know

- Their team's work, membership, and approval process
- Who should view the gateway and who should manage it
- The difference between personal work and shared team work
- How to use browser applications and identity-provider sign-in

## What they may not know

- How HyperShell maps users and groups to gateway roles
- The difference between gateway ownership and platform administration
- Cluster placement, gateway endpoints, namespaces, or releases
- Service-account roles, CI credentials, or secret rotation
- Which failures they can fix and which require platform support

## What they care about

- Clear gateway ownership
- Straightforward onboarding and offboarding
- No shared passwords or copied credentials
- Predictable access for the whole team
- Continuity when owners or team members change
- A visible path to support when the shared environment is unavailable

## Likely friction

- Per-user grants can become tedious for a large team
- Owner and viewer may not cover every team responsibility
- It may be unclear whether the gateway belongs to its creator or the team
- Technical troubleshooting details can obscure the action the team should take

## What HyperShell should prioritize

- Show gateway owner and team access clearly
- Make **Open console** the primary action for viewers
- Support identity-provider groups if teams are large or change often
- Make role changes and owner handoff safe and understandable
- Show team-facing status without requiring infrastructure knowledge
- Keep destructive actions away from routine team workflows

## Questions to validate

- Is a team a first-class HyperShell object or just a set of user grants?
- Do teams already exist as identity-provider groups we can use?
- Can a gateway have multiple owners, and can the final owner remove themselves?
- Does the team need a manager role between viewer and owner?

## Related flows

See [User flows](../user_flows.md), especially viewing gateways, provisioning a team gateway, sharing a team gateway, and opening the OpenShell console.
