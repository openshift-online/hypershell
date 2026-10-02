# Persona: Individual developer

This persona represents a developer using agent sandboxes for their own development, testing, or automation. They want technical control without becoming the operator of the HyperShell platform.

## Snapshot

| Attribute | Working assumption |
| --- | --- |
| Representative roles | Software engineer, ML engineer, data engineer, or developer advocate |
| Working style | Personal experimentation, coding, testing, and small automation |
| Primary interface | HyperShell web console, OpenShell CLI, and terminal |
| Likely access | Creator and owner of personal gateways; viewer of some shared gateways |
| Technical preference | Technical details and copyable commands visible |

## What they're trying to do

- Provision a gateway for development work
- Install the OpenShell CLI and connect to the correct gateway
- Configure a model provider from their local environment
- Create isolated sandboxes with known CPU and memory settings
- Inspect status, placement, endpoint, version, and failure details
- Create a service account for a script or CI job
- Troubleshoot setup without waiting for platform support

## Specific examples

| Example | Successful outcome |
| --- | --- |
| Test an agent against an unfamiliar repository | The developer creates an isolated sandbox and runs the agent without exposing their workstation |
| Compare agent or provider configurations | The developer creates repeatable sandboxes and can tell which gateway and provider each test uses |
| Run a nightly validation job | The developer creates a gateway-scoped service account and stores its secret in the CI secret manager |
| Diagnose a failed connection | The developer can inspect endpoint, status, version, and actionable error guidance |

## What they know

- Command-line tools, environment variables, and shell workflows
- Source control, CI jobs, and secret managers
- Basic OAuth or OIDC concepts
- Cloud provider credentials and local development setup
- Resource requests and limits at a practical level

## What they may not know

- HyperShell's control-plane and managed-cluster architecture
- How placement selects a concrete cluster
- Which fields HyperShell owns and which OpenShell owns
- The exact mapping between HyperShell roles and OpenShell roles
- Why a gateway can be running but not ready for connection

## What they care about

- Correct, copyable commands
- Fast provisioning and clear progress
- Useful technical details without searching logs first
- Stable endpoints and predictable authentication
- Secret-safe service-account setup
- Recovery guidance that preserves entered values

## Likely friction

- Incomplete commands or hidden connection values stop the workflow immediately
- Generic errors don't provide enough context to self-diagnose
- Provider, gateway, and sandbox setup can feel like one long chain of prerequisites
- Losing a one-time client secret requires disruptive replacement work

## What HyperShell should prioritize

- Make **Create gateway** and **Copy CLI command** prominent
- Present connection steps in the order they must run
- Show endpoint, placement, release, namespace, and gateway ID
- Keep commands safe for shell execution and provide visible copy feedback
- Explain missing readiness values instead of generating partial commands
- Make service-account expiration and replacement visible before automation breaks

## Questions to validate

- How often do developers use the web console after initial setup?
- Which operating systems and shells need first-class command guidance?
- Do individual developers need multiple gateways for different projects or security boundaries?
- Which diagnostics let them recover without exposing platform-only data?

## Related flows

See [User flows](../user_flows.md), especially provisioning a personal gateway, connecting with the CLI, and creating or managing service accounts.
