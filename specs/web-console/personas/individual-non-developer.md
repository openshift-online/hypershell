# Persona: Individual non-developer

This persona represents a user working mostly on their own who wants the outcome of an agent sandbox without managing the infrastructure behind it. This is a role-based persona, not a demographic profile.

## Snapshot

| Attribute | Working assumption |
| --- | --- |
| Representative roles | Researcher, analyst, product manager, technical writer, or subject-matter expert |
| Working style | Individual or ad hoc work with limited sharing |
| Primary interface | HyperShell web console and OpenShell console |
| Likely access | Viewer or owner of one or a few gateways |
| Technical preference | Straightforward view with infrastructure details hidden by default |

## What they're trying to do

- Find a gateway they can use
- Open the OpenShell console and start working
- Create a personal gateway when self-service is allowed
- Understand whether a gateway is ready, unavailable, or needs attention
- Return to a recently used gateway without repeating setup
- Finish their work without learning cluster, endpoint, or identity terminology

## Specific examples

| Example | Successful outcome |
| --- | --- |
| Analyze a set of documents with an agent | The user opens a ready gateway, starts a sandbox, and focuses on the documents |
| Try an agent workflow safely | The user gets an isolated environment without choosing Kubernetes or network settings they don't understand |
| Return to yesterday's work | The landing page makes the recent gateway and Open console action easy to find |
| Recover from an unavailable gateway | The page explains the status in plain language and tells the user what to do next |

## What they know

- Their subject area and the result they need
- How to use browser applications and sign in with their Red Hat identity
- Basic AI-agent concepts such as prompts, files, and tasks
- Whether their work is personal or should be shared with a team

## What they may not know

- What a gateway does behind the scenes
- Kubernetes namespaces, clusters, releases, or placement
- OIDC, service accounts, client credentials, or gateway endpoints
- How cloud providers and network visibility affect provisioning
- Whether an error comes from HyperShell, OpenShell, the model provider, or the sandbox

## What they care about

- A short path from sign-in to useful work
- Plain-language status and recovery guidance
- Confidence that their work is isolated and access is controlled
- Safe defaults that don't require infrastructure decisions
- Avoiding accidental deletion or loss of work
- Knowing who to contact when they can't recover on their own

## Likely friction

- Technical fields can make the application feel like an infrastructure console
- A provisioning form may ask questions they aren't prepared to answer
- CLI-first setup can block them before they reach the useful part of the product
- Terms such as gateway, provider, sandbox, and cluster may blur together

## What HyperShell should prioritize

- Make **Open console** the primary action
- Use plain-language gateway states with a clear next step
- Hide raw endpoints, namespaces, releases, and IDs by default
- Provide safe provisioning defaults or an assisted request flow
- Show recent gateways and work that needs attention
- Keep technical details available through a secondary disclosure

## Questions to validate

- Do these users create gateways, or does another role create gateways for them?
- Can they create and manage sandboxes entirely through the OpenShell console?
- Which provisioning decisions can use organization defaults?
- What work do they expect HyperShell to preserve between sessions?

## Related flows

See [User flows](../user_flows.md), especially viewing gateways, provisioning a personal gateway, reviewing gateway status, and opening the OpenShell console.
