# Generic Agent Base Specification

**Status:** Draft
**Applies to:** `hypershell-gitops` (the central GitOps repo) sandbox-consuming agent manifests and scripts
**Related:** `platform/openshell-inference-routing.spec.md` - sandbox execution and provider model access; `platform/openshell-gateway-service-accounts.spec.md` - gateway authentication for non-interactive callers; `platform/global-architecture.spec.md` - GitOps delivery; `standards/security/security.spec.md` - container security baseline

## Purpose

HyperShell runs non-interactive workloads (for example the `amber-review` code reviewer) as Kubernetes CronJobs that consume OpenShell sandboxes: they authenticate to a gateway, create a sandbox, upload and execute a workload inside it, collect results, and clean up. The orchestration layer these workloads share - gateway authentication and token refresh, sandbox lifecycle management, provider verification, policy enforcement, stale-resource cleanup, signal handling, and the openshell CLI init container - is entirely general-purpose and independent of any one workload.

Today that orchestration exists only inside `amber-review`'s CronJob as several hundred lines of bash. Teams building other sandbox-consuming agents (CI bots, security scanners, test runners, data pipelines) have no supported starting point and must reverse-engineer and copy `amber-review`. Copies drift, least-privilege defaults are easy to get wrong from scratch, and fixes do not propagate across consumers.

The desired state is a reusable **Generic Agent base** in the `hypershell-gitops` repo: a Kustomize base plus a sourced shell library that encode the proven orchestration and least-privilege defaults, over which any agent is a thin overlay contributing only its own scripts, policy, provider bundle, environment, and schedule. `amber-review` becomes one such overlay. This specification defines the desired behavior of that base as a set of behavior contracts; concrete manifests, scripts, and per-cluster runbook steps are implementation.

This specification covers **specification only**. It does not change the openshell CLI or the gateway itself; the base is purely a consumption pattern on the Kubernetes/GitOps side. Workspace-side provider registration remains a manual or API step outside this base.

## Requirements

### Requirement: GENAGENT-01 -- Reusable Agent Base and Overlay Model

The `hypershell-gitops` repo SHALL contain a reusable Generic Agent base that downstream agents consume as a Kustomize base, analogous to how per-tenant control-plane instances consume a shared base. The base SHALL be self-contained: applying it directly SHALL produce valid, schema-correct Kubernetes manifests, and building an overlay on top of it SHALL merge the overlay's scripts, policy, provider bundle, environment, and schedule without the overlay having to redeclare the orchestration.

A downstream agent SHALL be able to stand up a working sandbox-consuming CronJob by supplying only agent-specific inputs: one or more workload scripts, a policy file, a provider bundle, environment values (for example the target gateway endpoint and workspace), and a schedule. The agent SHALL NOT need to copy or re-implement the gateway-authentication, sandbox-lifecycle, cleanup, or security-context logic supplied by the base.

The base SHALL define the canonical CronJob volume layout used by agents, providing distinct mounts for: agent scripts (read-only), policy files (read-only), the openshell CLI binary (populated by an init container and shared with the main container), agent working state, and scratch space.

#### Scenario: Base builds standalone

- GIVEN the Generic Agent base in `hypershell-gitops`
- WHEN the base is rendered with a Kustomize build
- THEN it SHALL produce valid Kubernetes manifests including the CronJob skeleton, the init container, and the canonical volume layout
- AND the manifests SHALL pass Kubernetes schema validation

#### Scenario: Overlay contributes only agent-specific inputs

- GIVEN a downstream agent overlay that references the Generic Agent base and adds its own scripts, policy, provider bundle, environment values, and schedule
- WHEN the overlay is rendered
- THEN the rendered CronJob SHALL include the base orchestration merged with the overlay's inputs
- AND the overlay SHALL NOT redeclare the gateway-authentication, sandbox-lifecycle, cleanup, or security-context logic

### Requirement: GENAGENT-02 -- openshell CLI Init Container with Pinned, Verified Version

The base SHALL provision the openshell CLI into the workload through an init container that downloads a specific pinned CLI version and verifies its integrity against a per-architecture SHA256 checksum before installing it onto the shared tools volume. The main container SHALL consume the CLI from that shared volume.

The pinned version and its checksums SHALL be supplied as configuration (for example environment values), not hard-coded into the workload logic, so that upgrading the CLI is a configuration change. The init container SHALL NOT install a floating or "latest" reference: an unspecified or unverifiable version SHALL cause the init container to fail rather than proceed.

#### Scenario: Checksum mismatch fails the init container

- GIVEN the init container is configured with a pinned CLI version and expected SHA256 checksum
- WHEN the downloaded binary's checksum does not match the expected value
- THEN the init container SHALL fail
- AND the main container SHALL NOT start

#### Scenario: CLI version is configurable

- GIVEN an agent overlay
- WHEN the overlay sets the pinned CLI version and checksums via configuration
- THEN the workload SHALL install that version without any change to the base scripts

### Requirement: GENAGENT-03 -- Gateway Authentication Lifecycle

The base's shared library SHALL provide gateway authentication for a given isolated configuration root: registering the target gateway and logging in non-interactively using the agent's configured credentials. The library SHALL also provide token renewal that refreshes the login on a configurable interval and on demand, so that long-running agents keep a valid session for the duration of their work.

Authentication and renewal SHALL operate against a caller-supplied configuration root so that multiple gateway sessions can coexist. Authentication or renewal failure SHALL be surfaced to the caller as an error, never silently swallowed.

#### Scenario: Login established before sandbox use

- GIVEN an agent with configured gateway endpoint and credentials
- WHEN the agent configures the gateway for a configuration root
- THEN the library SHALL register the gateway and establish a logged-in session in that configuration root before any sandbox is created

#### Scenario: Token refreshed on interval

- GIVEN a running agent with an active gateway session and a configured refresh interval
- WHEN the refresh interval elapses
- THEN the library SHALL renew the login so the session remains valid
- AND a renewal failure SHALL be reported as an error rather than ignored

### Requirement: GENAGENT-04 -- Sandbox Lifecycle Orchestration

The base's shared library SHALL provide the full sandbox lifecycle as reusable operations: create a sandbox retained for exec use, wait for it to become ready via a readiness probe, execute commands inside it, and delete it. Creation SHALL accept the agent-specific inputs needed per sandbox, including the sandbox source, one or more providers, a policy file, files to upload into the sandbox, and a label used to identify sandboxes the agent owns.

Waiting for readiness and executing commands SHALL each honor a caller-supplied timeout. A sandbox that does not become ready within its timeout SHALL be reported as an error. Every sandbox the agent creates SHALL be registered for cleanup so it is deleted when the agent exits, whether the agent succeeds, fails, or is interrupted.

#### Scenario: Create, wait, exec, delete

- GIVEN an authenticated agent
- WHEN the agent creates a retained sandbox with a provider, policy, uploads, and an ownership label
- THEN the library SHALL wait for the sandbox to report ready within the configured timeout
- AND the agent SHALL be able to execute commands inside the sandbox
- AND the sandbox SHALL be deleted when the agent's work completes

#### Scenario: Readiness timeout is an error

- GIVEN a sandbox that has not reported ready
- WHEN the configured readiness timeout elapses
- THEN the library SHALL report an error to the caller rather than proceeding to exec

### Requirement: GENAGENT-05 -- Detached Execution for Long-Running Workloads

The base's shared library SHALL provide a detached execution mode for workloads that outlive a single exec timeout: the workload is started inside the sandbox so it survives the exec call returning, and its completion is observed by polling a status indicator from outside the sandbox. The detached mode SHALL report the workload's terminal status (success or failure) to the caller once the workload finishes or a caller-supplied overall timeout elapses.

#### Scenario: Long workload survives exec timeout

- GIVEN a workload expected to run longer than a single exec timeout
- WHEN the agent starts it in detached mode inside the sandbox
- THEN the exec call SHALL return without terminating the workload
- AND the library SHALL poll for the workload's completion status and report its terminal outcome to the caller

### Requirement: GENAGENT-06 -- Isolated Per-Sandbox Configuration Roots

The base's shared library SHALL provide isolated per-sandbox configuration roots (for example distinct `HOME`, `XDG_CONFIG_HOME`, and `XDG_STATE_HOME` under the working-state volume) so that agents running multiple sandboxes in parallel do not share or overwrite each other's gateway session state.

#### Scenario: Parallel sandboxes do not share session state

- GIVEN an agent that operates two sandboxes concurrently
- WHEN each sandbox is driven from its own configuration root
- THEN each SHALL maintain its own gateway session state
- AND neither SHALL overwrite the other's tokens or configuration

### Requirement: GENAGENT-07 -- Provider Verification Pre-flight

The base's shared library SHALL provide a pre-flight check that verifies every provider an agent depends on exists in the target workspace before any sandbox is created. If a required provider is missing, the agent SHALL fail the pre-flight with a clear error identifying the missing provider rather than creating sandboxes that cannot function.

#### Scenario: Missing provider fails pre-flight

- GIVEN an agent that requires a named provider
- WHEN that provider does not exist in the workspace
- THEN the pre-flight SHALL fail with an error naming the missing provider
- AND no sandbox SHALL be created

### Requirement: GENAGENT-08 -- Stale Sandbox Cleanup

The base's shared library SHALL provide cleanup of stale sandboxes left over from previous runs: it SHALL list sandboxes matching the agent's ownership label selector and delete them before the agent begins new work, so a crashed or interrupted prior run does not accumulate orphaned sandboxes.

#### Scenario: Leftover sandboxes removed before new work

- GIVEN sandboxes labeled as owned by the agent remain from a previous interrupted run
- WHEN the agent starts and runs stale-sandbox cleanup for its ownership label
- THEN those leftover sandboxes SHALL be deleted before the agent creates new sandboxes

### Requirement: GENAGENT-09 -- Signal Handling and Cleanup Traps

The base SHALL ensure agent resources are released on termination. The shared library SHALL install trap-based cleanup that, on normal exit and on SIGINT/SIGTERM, deletes the sandboxes the agent created and terminates child processes it started (such as the token-refresh loop). Cleanup SHALL run even when the agent's main work fails, and SHALL be resilient to partial state (for example a sandbox that was never fully created).

#### Scenario: Interrupt triggers cleanup

- GIVEN a running agent that has created sandboxes and started a token-refresh loop
- WHEN the CronJob pod receives SIGTERM
- THEN the trap SHALL delete the agent's sandboxes and terminate its child processes before the process exits

#### Scenario: Cleanup runs on failure

- GIVEN an agent whose main workload fails partway through
- WHEN the agent process exits with a non-zero status
- THEN the trap SHALL still delete the sandboxes the agent created

### Requirement: GENAGENT-10 -- Least-Privilege Security Baseline

The base SHALL encode a restricted security baseline that every agent inherits, consistent with `standards/security/security.spec.md`. Agent containers SHALL run with `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, all Linux capabilities dropped, and the `RuntimeDefault` seccomp profile. Service-account token automounting SHALL be disabled unless an overlay explicitly requires it. The script and policy volumes SHALL be mounted read-only. These defaults SHALL apply without the overlay restating them, and an overlay SHALL be able to add scope only additively (for example an agent-specific provider), never by weakening the baseline.

#### Scenario: Inherited restricted context

- GIVEN an agent overlay that sets no security context of its own
- WHEN its CronJob is rendered from the base
- THEN the agent container SHALL run as non-root with privilege escalation disabled, all capabilities dropped, and the RuntimeDefault seccomp profile
- AND the script and policy volumes SHALL be mounted read-only

### Requirement: GENAGENT-11 -- Policy and Provider Templates

The base SHALL ship annotated, minimal-privilege templates that agents copy and specialize: a sandbox policy template describing the filesystem, network, and process constraints and their meaning, and a provider bundle template describing the credential-bundle schema for registering API access (endpoints, binaries, and authentication style). The base SHALL support distinct policies for distinct sandbox roles, so an agent that runs sandboxes with different privilege needs can supply a different policy per role. The templates SHALL default to least privilege, granting only what the annotations describe.

#### Scenario: Per-role policies

- GIVEN an agent that runs two sandbox roles with different privilege needs
- WHEN it supplies a distinct policy file per role
- THEN each sandbox SHALL be created with the policy for its role

#### Scenario: Templates are documented and minimal

- GIVEN the policy and provider templates shipped by the base
- WHEN an agent author reads them
- THEN each constraint section SHALL be annotated with what it controls
- AND the defaults SHALL grant only least-privilege access

### Requirement: GENAGENT-12 -- Shared Library Function Surface

The base SHALL expose its orchestration as a single shell library that an agent sources from a well-known path, presenting a stable function surface covering gateway lifecycle (configure and renew), pre-flight (provider verification and stale-sandbox cleanup), sandbox lifecycle (create, wait, exec, detached exec, delete), and cleanup and isolation (cleanup registration and configuration-root creation). An agent's entrypoint SHALL be able to source the library and drive a complete run through these functions without invoking the openshell CLI directly for orchestration concerns the library already covers.

#### Scenario: Agent sources the library

- GIVEN an agent entrypoint script
- WHEN it sources the shared library from its well-known path
- THEN it SHALL be able to configure the gateway, verify providers, clean up stale sandboxes, create and drive sandboxes, and rely on registered cleanup - using the library's functions rather than re-implementing them

### Requirement: GENAGENT-13 -- Documentation

The base SHALL ship a concise README that enables a new agent author to build an agent without reverse-engineering an existing one. It SHALL cover the prerequisites (gateway endpoint, OIDC credentials, workspace, providers), the openshell CLI lifecycle the base automates (add, login, create, exec, delete), how to write a policy file and its constraint sections, how to define and register a provider, how to build a new agent as an overlay on the base, and the detached-execution pattern for long-running workloads.

#### Scenario: Author builds an agent from the README alone

- GIVEN the base README
- WHEN a new agent author follows it
- THEN they SHALL be able to author a working agent overlay - scripts, policy, provider, environment, and schedule - without reading another agent's implementation

### Requirement: GENAGENT-14 -- amber-review Becomes an Overlay on the Base

`amber-review` SHALL be expressed as an overlay on the Generic Agent base, delegating the generic orchestration (gateway authentication, sandbox lifecycle, provider verification, cleanup, security baseline, and the openshell CLI init container) to the base and contributing only its amber-specific parts: its PR-discovery and review scripts, its scoped GitHub provider bundle, its review-specific environment values, and an entrypoint that sources the shared library and calls its discovery and review phases. The migration SHALL preserve `amber-review`'s existing review behavior; it is a refactor of where the orchestration lives, not a change to what the reviewer does.

#### Scenario: amber-review delegates orchestration

- GIVEN the Generic Agent base exists
- WHEN `amber-review` is expressed as an overlay on it
- THEN the generic gateway-authentication, sandbox-lifecycle, cleanup, and security logic SHALL come from the base
- AND `amber-review` SHALL contribute only its discovery/review scripts, scoped GitHub provider, review environment, and entrypoint
- AND the reviewer's externally observable behavior SHALL be unchanged
