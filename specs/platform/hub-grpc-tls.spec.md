# Hub gRPC External TLS Exposure

**Date:** 2026-09-24
**Status:** Draft
**Ticket:** HYPERSHELL-333
**Related:** `control-plane.spec.md` (mandatory cluster identity, client-side transport selection), `managed-cluster-registration.spec.md` (registration, watch-stream caller binding), `oidc-integration.spec.md` (gRPC JWT bypass list), `security/rbac-enforcement.spec.md` (`managed-cluster-registrar`), `global-architecture.spec.md` (Cloud Hub / ManagedCluster topology)

---

## Purpose

Every control plane is a registered spoke of exactly one hub (see
`control-plane.spec.md`, "Requirement: Mandatory Cluster Identity"). Spokes run on
remote clusters -- other clouds and other Kubernetes distributions than their hub --
and must reach the hub API server's gRPC endpoint over the public internet to open
`WatchGateways` and the other watch streams. Today that endpoint is reachable only
in-cluster, through the `hypershell-api-server` Service on port `9000`, the listener
is plaintext, and the watch RPCs are exempt from JWT validation.

This spec defines what a hub SHALL expose so that any control plane, remote or
co-located with the hub, dials one TLS endpoint at one public hostname, and what the
hub SHALL enforce before that endpoint is exposed.

The api-server binary's TLS support already exists in the rh-trex-ai framework
(`--grpc-enable-tls`, `--grpc-tls-cert-file`, `--grpc-tls-key-file`; see
`pkg/config/grpc.go`). The framework runs **one** gRPC listener
(`--grpc-server-bindaddress`); enabling TLS makes that listener TLS-only. There is no
plaintext gRPC port on a TLS-enabled hub, so the hub's own co-located control plane
dials the same external hostname a remote spoke does. Environments that leave TLS off
(Kind, `make openshift-up` development namespaces) keep dialing the in-cluster
Service in plaintext; the control plane selects the transport from the address it is
given (`control-plane.spec.md`, "Requirement: gRPC Transport Security").

---

## Requirements

### Requirement: Hub API Server gRPC TLS

Each production hub api-server deployment SHALL enable gRPC TLS by adding
`--grpc-enable-tls=true`, `--grpc-tls-cert-file=/grpc-tls/tls.crt`, and
`--grpc-tls-key-file=/grpc-tls/tls.key` to its command args. The gRPC listener on
port `9000` then serves TLS only; the hub SHALL NOT run a second plaintext gRPC
listener. The certificate SHALL be provisioned by a cert-manager `Certificate`
referencing the `letsencrypt-dns01` `ClusterIssuer`, for hostname
`grpc.hyp{N}.infra.hypershell.app` (where `hyp{N}` is this hub's cluster name), and
mounted as a volume at `/grpc-tls` in the api-server pod.

This hostname is distinct from the existing `api.hyp{N}.infra.hypershell.app` REST
hostname: the two live on different Service ports (`8000` vs `9000`) and use
different Route termination modes (edge vs. passthrough), so they require separate
`Certificate` and `Route` objects even though both terminate at the same api-server
pod.

The api-server loads the certificate files once at startup and does not reload them.
The hub SHALL therefore roll the api-server pods after every cert-manager renewal so
that new connections present the renewed certificate before the previous one
expires. The mechanism (a checksum annotation on the Deployment, a Secret-watching
reloader, or an equivalent) is a gitops concern; the observable contract is the
scenario below.

#### Scenario: gRPC listener presents a valid certificate

- GIVEN a hub api-server deployment with `--grpc-enable-tls=true` and the
  cert-manager `Certificate` for `grpc.hyp0.infra.hypershell.app` issued and mounted
- WHEN a client completes a TLS handshake against port `9000`
- THEN the presented certificate SHALL be valid for `grpc.hyp0.infra.hypershell.app`
  and chain to a publicly trusted root (via `letsencrypt-dns01`)

#### Scenario: Plaintext dial is refused on a TLS-enabled hub

- GIVEN a hub api-server deployment with `--grpc-enable-tls=true`
- WHEN a client dials port `9000` with plaintext credentials
- THEN the connection SHALL fail the TLS handshake
- AND no gRPC request SHALL be served on that connection

#### Scenario: Certificate renewal is served without operator intervention

- GIVEN the mounted certificate is renewed by cert-manager via `letsencrypt-dns01`
- WHEN the renewal is written to the mounted Secret
- THEN the api-server pods SHALL be rolled without a manual step
- AND connections opened after the roll SHALL present the renewed certificate
- AND connected control planes SHALL reconnect through their normal watch-stream
  backoff (`control-plane.spec.md`, "TLS handshake failure retries, does not exit")

### Requirement: Passthrough Route Per Hub

Each hub SHALL expose an OpenShift `Route` on host `grpc.hyp{N}.infra.hypershell.app`
targeting the `hypershell-api-server` Service's `grpc` port (`targetPort: grpc`,
container port `9000`), with `spec.tls.termination: passthrough`. Passthrough
preserves end-to-end TLS: the router forwards the raw TLS bytestream to the
api-server pod, which terminates TLS itself (per the requirement above). The router
never sees plaintext gRPC traffic and does not need to speak HTTP/2 to a
re-encrypted backend.

#### Scenario: Route forwards without terminating TLS

- GIVEN the passthrough Route for `grpc.hyp0.infra.hypershell.app` exists
- WHEN an external client opens a TLS connection to that host on port `443`
- THEN the router SHALL forward the raw TLS stream to the api-server pod's port `9000`
- AND the TLS handshake SHALL be terminated by the api-server, not the router

#### Scenario: Remote spoke opens a watch stream through the Route

- GIVEN a registered spoke control plane configured with
  `HYPERSHELL_GRPC_SERVER_ADDR=grpc.hyp0.infra.hypershell.app:443`
- WHEN it dials and opens `WatchGateways` for its own `cluster_id`
- THEN the connection SHALL succeed and remain open
- AND gateway events assigned to that `cluster_id` SHALL be delivered

#### Scenario: Co-located control plane dials the same Route

- GIVEN the hub's own control plane runs in the same cluster as the api-server
- AND it is configured with `HYPERSHELL_GRPC_SERVER_ADDR=grpc.hyp0.infra.hypershell.app:443`
- WHEN it dials and opens `WatchGateways` for its own `cluster_id`
- THEN the connection SHALL traverse the cluster's router (hairpin) and succeed
- AND the watch stream SHALL behave identically to a remote spoke's

### Requirement: DNS for the gRPC Hostname

`grpc.hyp{N}.infra.hypershell.app` SHALL resolve (via a CNAME record) to the
hub cluster's router load balancer, following the same DNS pattern already used for
`api.hyp{N}.infra.hypershell.app` and `keycloak.hyp{N}.infra.hypershell.app`. The
name SHALL resolve identically from inside the hub cluster, so the co-located control
plane's hairpin dial reaches the same router.

### Requirement: Authenticated Watch Streams Before Exposure

A hub SHALL NOT expose its gRPC port outside the cluster while any
`/hypershell.v1.*/Watch*` method is listed in `--auth-bypass-methods`. On every hub,
and in every environment that runs the control plane with OIDC credentials, the
api-server SHALL require a valid JWT on the four watch RPCs and SHALL bind
`WatchGateways` to the caller's registered cluster as defined in
`managed-cluster-registration.spec.md` ("Requirement: Watch Stream Caller Binding").
Only `/grpc.health.v1.Health/` and `/grpc.reflection.v1alpha.ServerReflection/`
remain exempt. This supersedes the "gRPC Bypass" list in `oidc-integration.spec.md`.

#### Scenario: Unauthenticated watch through the Route is refused

- GIVEN the passthrough Route for `grpc.hyp0.infra.hypershell.app` exists
- WHEN a client with no bearer token opens `WatchGateways` through it
- THEN the api-server SHALL return `UNAUTHENTICATED`
- AND no gateway event SHALL be delivered

#### Scenario: Spoke cannot watch another cluster's gateways

- GIVEN spoke `A` registered as `cluster_id: X` and spoke `B` registered as `cluster_id: Y`
- WHEN spoke `A` opens `WatchGateways` with `cluster_id: Y`
- THEN the api-server SHALL return `PERMISSION_DENIED`

---

## Non-Goals

- **Mutual TLS.** This spec covers server-side TLS only (the control plane verifies
  the hub's certificate). Control-plane identity is established at the application
  layer via the OIDC `client_credentials` bearer token, not via a client certificate.
  Revisit if the bearer-token model proves insufficient.
- **Framework changes.** `--grpc-enable-tls` and friends already exist in rh-trex-ai.
  In particular this spec does NOT add a second, plaintext gRPC listener to the
  framework; see Design Decisions.
- **Service-account provisioner reachability.** The api-server reaches the
  control-plane's internal service-account provisioner at one in-cluster address
  (`HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR`, see
  `openshell-gateway-service-accounts.spec.md`). That path is unchanged and only
  reaches the hub's co-located control plane. OpenShellGatewayServiceAccount
  provisioning for gateways hosted on a remote spoke is out of scope here and needs
  its own spec.

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| TLS-only listener plus router hairpin for the co-located control plane, rather than a second plaintext listener | The framework has one gRPC listener and TLS makes it TLS-only. A second listener is a framework change for the sole benefit of one client that already has to be a registered spoke anyway. One transport path in production means the co-located control plane exercises exactly the code and infrastructure a remote spoke depends on, so certificate, DNS, and Route regressions surface on every hub, not only at the first remote spoke. Dev environments keep plaintext by leaving TLS off, not by a special client path. |
| Watch streams must authenticate before the port is exposed | With `Watch*` in the JWT bypass list, a public Route would hand every gateway record to any anonymous client, and cluster scoping in `WatchGateways` is cooperative (the server does not verify who owns a `cluster_id`). Mandatory registration gives every control plane an identity, which makes enforcement possible; exposure without it is not acceptable. |
| Passthrough Route termination, not edge or re-encrypt | The api-server terminates its own TLS (first requirement). Edge termination would require the router to speak plaintext HTTP/2 to the backend, the router-level cleartext-h2 path rejected below. Re-encrypt would add a second TLS hop and a second certificate for no isolation benefit, since router and api-server are both hub-operated. |
| Rejected: unsecured Route with `haproxy.router.openshift.io/h2c-enable` on the plaintext port | Tried as a stopgap ahead of this spec: an unsecured Route with the h2c-enable annotation lets a client complete an HTTP/2 cleartext handshake through the router, but the resulting long-lived gRPC streams disconnected unpredictably (`error reading server preface: EOF`, intermittent `RST_STREAM`) even though a direct in-cluster connection to the identical backend was reliable. Real TLS passthrough avoids the router parsing HTTP/2 at all. |
| Separate hostname (`grpc.hyp{N}`) rather than reusing `api.hyp{N}` | `api.hyp{N}` is edge-terminated (HTTP redirect to HTTPS) on the REST port `8000`; a single Route can have only one termination mode and one target port. Reusing the hostname would require SNI-based multiplexing between two termination modes for no real benefit over a second DNS record. |
| Hub-managed certificate via `letsencrypt-dns01`, not a self-signed/internal CA | The control plane dials with the system trust store (`credentials.NewTLS()` with default verification, per `control-plane.spec.md`) rather than a pinned custom CA, so the certificate must chain to a publicly trusted root. This also matches the existing `api.hyp{N}` and `keycloak.hyp{N}` certificates on the same hub. |
| Roll the api-server after renewal rather than hot-reloading the certificate | The framework loads the certificate once at startup. Hot reload would be a framework change; a rollout after renewal is already how cert-manager-backed workloads on the hub are refreshed, and the control plane's retry rule makes the roll invisible to spokes. |

---

## Scope

- **hypershell repo:** control-plane transport selection and mandatory identity
  (`control-plane.spec.md`); api-server watch JWT enforcement and caller binding
  (`managed-cluster-registration.spec.md`); removal of `Watch*` from every
  `--auth-bypass-methods` value in `deploy/` and from the `development_oidc`
  environment defaults; Kind and OpenShift development overlays giving the control
  plane a cluster name and the `managed-cluster-registrar` role.
- **hypershell-gitops repo:** `grpc-tls` kustomize component (per-hub `Certificate`
  + passthrough `Route` + api-server rollout on renewal), and every control plane's
  `HYPERSHELL_GRPC_SERVER_ADDR` set to the hub's external hostname, including the
  hub's co-located control plane.
- **DNS:** `grpc.hyp{N}.infra.hypershell.app` CNAME records pointing at each hub's
  router load balancer.

This spec (in the `hypershell` repo) is the single source of truth for the desired
behavior; the gitops manifests implementing it live in the separate
`hypershell-gitops` repo and are out of scope for this document.
