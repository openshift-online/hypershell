# OpenShell Gateway Console Specification

**Date:** 2026-08-27
**Status:** Draft
**Parent:** `openshell-gateway.spec.md`
**Related:** `openshell-gateway-keycloak.spec.md` (per-gateway client, OIDC Role Bridge); `openshell-gateway-routing.spec.md` (Gateway API and OpenShift Route ingress, hostnames, NetworkPolicy)
**Upstream:** [OpenShell Dashboard](https://github.com/Gkrumbach07/openshell-dashboard); [oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/)

---

## Purpose

The Gateway Console is the OpenShell dashboard for one gateway. The control plane deploys a console with each gateway that has a route. The console runs in the gateway namespace (`openshell-<id-hex-8>`). The console connects to the gateway through the in-cluster Service. A user opens the console in a browser at a per-gateway hostname.

An oauth2-proxy sidecar authenticates the browser against a dedicated Keycloak client. The access token carries the gateway audience (`aud = {name}-{id}`) and the user roles (`hypershell.roles`). The dashboard sends this token to the gateway. The gateway validates the token with the rules that it applies to the CLI. The OIDC Role Bridge controls access. A user operates the console only where a `gateway:owner` or `gateway:viewer` RoleBinding exists.

This spec covers Option 1: one oauth2-proxy for each gateway, real tokens with the correct audience, no token exchange, and no shared credential across gateways.

> **Terminology.** The Gateway Console is the per-gateway dashboard in this spec. It is not the management web-console (`components/web-console/`), which is the fleet-wide UI. The two are separate deployments.

---

## Token flow

1. The browser opens `console-<ns>.<base-domain>`. oauth2-proxy finds no session.
2. oauth2-proxy starts an authorization-code flow with PKCE for the client `{name}-{id}-console`.
3. The user signs in to the shared realm.
4. Keycloak issues an access token with these claims:
   - `aud`: `{name}-{id}`
   - `hypershell.roles`: `openshell-admin` or `openshell-user`
   - `sub`: the user
5. oauth2-proxy stores the session in an encrypted cookie. oauth2-proxy sends the request to the dashboard with the header `X-Forwarded-Access-Token`.
6. The dashboard sends the token to the gateway over gRPC.
7. The gateway validates the issuer and `aud = {name}-{id}`. The gateway maps `hypershell.roles` to admin or user access. The gateway denies a user who has no role for this gateway.

The console client is a second Keycloak client. It is not the CLI client (`{name}-{id}`). Its mappers target the gateway client. The gateway accepts console tokens like CLI tokens. The CLI client stays unchanged.

A console needs external ingress. Browser traffic reaches the console through an HTTPRoute in `gateway-api` mode or through an OpenShift Route in `route` mode. A routed gateway has `client_ca_path` removed on its external data path because the ingress proxy cannot present a client certificate (see `openshell-gateway-routing.spec.md`). The console dashboard does not use external ingress to reach the gateway. It connects to the in-cluster `openshell-gateway.<ns>.svc.cluster.local:8080` admin API through gRPC with mutual TLS. It presents the `openshell-client` certificate and verifies the gateway server certificate against the OpenShell CA.

---

## Requirements

### Requirement: Console Enablement Tied to Routing

The reconciler SHALL deploy the console when all of these conditions are true:

- The gateway has an enabled route. A present route object defaults to enabled when it omits `enabled`; an explicit `route.enabled = false` disables it.
- The selected ingress mode is `gateway-api` or `route`.
- Keycloak is configured.

The console has no separate configuration field. The console SHALL use the same effective `GATEWAY_INGRESS_MODE` as the gateway. The console SHALL follow the route lifecycle.

#### Scenario: Gateway API mode creates a console

- GIVEN a gateway with an enabled route
- AND the selected ingress mode is `gateway-api`
- AND Keycloak is configured
- WHEN the reconciler reconciles the gateway
- THEN it must create all console resources: the console client, the console Secret, the Deployment, the Service, the HTTPRoute, and the NetworkPolicies
- AND the console must answer at `https://console-<ns>.<base-domain>`

#### Scenario: OpenShift Route mode creates a console

- GIVEN a gateway with an enabled route
- AND the selected ingress mode is `route`
- AND Keycloak is configured
- WHEN the reconciler reconciles the gateway
- THEN it must create all console resources: the console client, the console Secret, the Deployment, the Service, the OpenShift Route, and the NetworkPolicies
- AND it must not create a console HTTPRoute
- AND the console must answer at `https://console-<ns>.<base-domain>`

#### Scenario: Non-routed gateway gets no console

- GIVEN a gateway with no route
- WHEN the reconciler reconciles the gateway
- THEN it must not create console resources
- AND it must not create a console client

#### Scenario: Prerequisite is absent

- GIVEN a routed gateway with no selected ingress mode or with no Keycloak configuration
- WHEN the reconciler reconciles the gateway
- THEN it must write a warning
- AND it must skip the console
- AND it must not fail the gateway reconciliation

---

### Requirement: Confidential Console Keycloak Client

The reconciler must create one confidential OIDC client for each console. The reconciler creates this client in the same reconciliation pass that provisions the gateway CLI client, and it reuses the same Keycloak Admin REST API access. This client is separate from the CLI client.

#### Client properties

| Property | Value | Note |
|---|---|---|
| `clientId` | `{name}-{id}-console` | Unique in the realm; different from the CLI client `{name}-{id}` |
| `publicClient` | `false` | Confidential; oauth2-proxy needs a client secret |
| `standardFlowEnabled` | `true` | Authorization-code flow for the browser |
| `directAccessGrantsEnabled` | `false` | Browser flow only |
| `serviceAccountsEnabled` | `false` | Not a service account |
| `fullScopeAllowed` | `false` | Keeps per-gateway isolation |
| `redirectUris` | `["https://console-<ns>.<base-domain>/oauth2/callback"]` | oauth2-proxy callback |
| `webOrigins` | `["https://console-<ns>.<base-domain>"]` | Same origin |
| `attributes.pkce.code.challenge.method` | `S256` | PKCE on the confidential client |
| `defaultClientScopes` | `["openid", "profile", "email", "roles", "gateway-roles", "web-origins", "acr"]` | `gateway-roles` runs the client-role mapper |

> `fullScopeAllowed` must be `false`. A `true` value, together with the built-in audience-resolve mapper, adds every gateway audience and role to the token. This breaks per-gateway isolation.

#### Protocol mappers

The console client must have three protocol mappers. Each mapper targets the gateway client (`{name}-{id}`), not the console client:

1. **Audience** -- `oidc-audience-mapper`, `included.client.audience = {name}-{id}`, `access.token.claim = true`, `id.token.claim = false`. Sets `aud = {name}-{id}`.
2. **Client roles** -- `oidc-usermodel-client-role-mapper`, `claim.name = hypershell.roles`, `multivalued = true`, `access.token.claim = true`, `usermodel.clientRoleMapping.clientId = {name}-{id}`. Adds the user gateway-client roles to `hypershell.roles`.
3. **Sub** -- `oidc-sub-mapper`, `access.token.claim = true`. Adds `sub`.

The client-role mapper reads the user roles for the named client. It adds the gateway roles to the token, although the console client issues the token. The OIDC Role Bridge already assigns `openshell-admin` and `openshell-user` on the gateway client (see `openshell-gateway-keycloak.spec.md`). The reconciler adds no roles.

#### Scope mappings

Because `fullScopeAllowed` is `false`, Keycloak filters every role mapper's output to the console client's scope. A client role that is not in scope is silently dropped from the token, even when the mapper targets it and the user holds it. The reconciler MUST therefore grant the console client scope for the gateway client's roles by adding client scope-mappings for `openshell-admin` and `openshell-user` (the gateway client's roles) to the console client. Without this grant the access token omits `hypershell.roles` and the gateway denies every request with `role 'openshell-user' required`, even though `aud` (which is not scope-filtered) is present. The grant is scoped to this one gateway client's roles, so per-gateway isolation is preserved. The reconciler reconciles the scope-mappings on every pass so a console provisioned before this grant existed is healed.

#### Scenario: Console token has the gateway audience and roles

- GIVEN a routed gateway `my-gateway` (id `2FhMpQzXBz`) with gateway client `my-gateway-2FhMpQzXBz`
- AND user-a has `gateway:owner` on the gateway, which gives `openshell-admin`
- WHEN user-a signs in through oauth2-proxy
- THEN the access token must contain `aud: "my-gateway-2FhMpQzXBz"` and `hypershell.roles: ["openshell-admin"]`
- AND the token must not contain any other gateway audience or role
- AND the gateway must give admin access

#### Scenario: User without a RoleBinding is denied

- GIVEN user-c is a realm user with no RoleBinding on the gateway
- WHEN user-c signs in through oauth2-proxy
- THEN the token must not contain `openshell-admin` or `openshell-user` for this gateway
- AND the gateway must deny the requests, because it runs with `allow_unauthenticated_users = false`

---

### Requirement: Console Credential Secret

The reconciler must store two values in one Secret named `openshell-console-oauth2` in the gateway namespace.

| Key | Source | Use |
|---|---|---|
| `client-secret` | Keycloak (`GET /admin/realms/{realm}/clients/{uuid}/client-secret`) | Confidential client secret for oauth2-proxy |
| `cookie-secret` | The control plane generates 32 random bytes, base64 | Encrypts the oauth2-proxy session cookie |

The reconciler must create the Secret before the Deployment. The reconciler must use update-or-create. The control plane must not write either value to a log.

#### Scenario: Cookie secret is stable

- GIVEN an `openshell-console-oauth2` Secret with a `cookie-secret`
- WHEN the reconciler reconciles the gateway again
- THEN it must keep the current `cookie-secret`
- AND it must update `client-secret` only when the Keycloak secret changed
- AND active browser sessions must stay valid

---

### Requirement: Console Deployment

The reconciler must create one Deployment named `openshell-console` in the gateway namespace. The Deployment has two containers: the dashboard and the oauth2-proxy sidecar. The dashboard listens on loopback. The Service exposes only oauth2-proxy.

Every console resource must carry the standard gateway labels, with `app.kubernetes.io/component = console` and `app.kubernetes.io/instance = openshell-console`.

#### Dashboard container

- Image: the control-plane default (see ImageDefaults); a per-gateway value can override it.
- Listen port: `8000`. The dashboard binds all interfaces, so the kubelet can run the probe. The Service does not expose port `8000`. The NetworkPolicies do not allow ingress to port `8000`, so only the in-pod oauth2-proxy reaches the dashboard.
- Environment (the dashboard BFF reads these; see the upstream contract):
  - `PORT = 8000`
  - `OPENSHELL_GATEWAY_URL = grpcs://openshell-gateway.<ns>.svc.cluster.local:8080` (the `grpcs://` scheme selects the dashboard's TLS gRPC client; without it the client speaks h2c cleartext into the gateway's TLS listener and the connection is reset while reading the server preface)
  - `GATEWAY_CA_CERT = /etc/openshell-tls/gateway/ca.crt` (verifies the gateway server certificate)
  - `GATEWAY_CLIENT_CERT = /etc/openshell-tls/gateway-client/tls.crt` (client certificate presented for mutual TLS to the gateway admin API)
  - `GATEWAY_CLIENT_KEY = /etc/openshell-tls/gateway-client/tls.key` (client private key for mutual TLS)
  - `AUTH_DISABLED = false`
  - `AUTH_TOKEN_HEADER = X-Forwarded-Access-Token`
  - `AUTH_USER_HEADER = X-Forwarded-User`
  - `ADMIN_ROLE = openshell-admin`
  - `LOGOUT_URL = /oauth2/sign_out`
- Volume mounts:
  - the `openshell-server-tls` Secret key `ca.crt` at `/etc/openshell-tls/gateway` (read only), to verify the gateway server certificate.
  - the `openshell-client-tls` Secret keys `tls.crt` and `tls.key` at `/etc/openshell-tls/gateway-client` (read only), presented as the client certificate for mutual TLS to the gateway admin API.

#### oauth2-proxy sidecar container

- Image: the control-plane default (see ImageDefaults); a value can override it.
- Listen address: `0.0.0.0:4180` (the Service target).
- Configuration (through `OAUTH2_PROXY_*` variables):
  - `provider = oidc`
  - `oidc-issuer-url = {server-url}/realms/{realm}`
  - `client-id = {name}-{id}-console`
  - `client-secret` from Secret key `client-secret`
  - `cookie-secret` from Secret key `cookie-secret`
  - `code-challenge-method = S256`
  - `redirect-url = https://console-<ns>.<base-domain>/oauth2/callback`
  - `upstream = http://127.0.0.1:8000`
  - `http-address = 0.0.0.0:4180`
  - `reverse-proxy = true`
  - `pass-access-token = true` (adds `X-Forwarded-Access-Token`)
  - `pass-user-headers = true` (adds `X-Forwarded-User` and related headers)
  - `skip-provider-button = true`
  - `cookie-secure = true`
  - `email-domain = *` (any realm user; the gateway controls access)
  - `scope = "openid profile email roles gateway-roles"`

#### SecurityContext

Each container must set `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `seccompProfile.type: RuntimeDefault`, and `capabilities.drop: [ALL]`. Because the root filesystem is read only, each container must mount a writable `emptyDir` at `/tmp`. The Deployment must follow the gateway rules for the OpenShift UID and `fsGroup` (see `openshell-gateway.spec.md`). Each container must request modest resources.

#### Health probes

oauth2-proxy must expose readiness and liveness probes on `/ready` and `/ping` (port `4180`). The dashboard must expose a readiness probe on port `8000`. A TCP probe is sufficient when the dashboard has no health path.

---

### Requirement: Console Service and Mode-Selected HTTP Exposure

The reconciler SHALL create a `ClusterIP` Service named `openshell-console` on port `4180`. It SHALL create only the console exposure resource for the selected ingress mode. It SHALL remove an exposure resource from the inactive mode when one exists.

In `gateway-api` mode, the reconciler SHALL create this HTTPRoute. The HTTPRoute attaches to the shared Gateway.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: openshell-console
  namespace: <ns>
spec:
  parentRefs:
  - name: <GATEWAY_API_GATEWAY_NAME>
    namespace: <GATEWAY_API_GATEWAY_NAMESPACE>
    sectionName: <GATEWAY_API_HTTP_LISTENER_NAME>
  hostnames:
  - console-<ns>.<base-domain>
  rules:
  - backendRefs:
    - name: openshell-console
      port: 4180
```

The hostname `console-<ns>.<base-domain>` is a subdomain of `<base-domain>`. The shared Gateway wildcard certificate covers it, so the console needs no separate certificate. This hostname differs from the gateway gRPC hostname (`gw-<ns>.<base-domain>`), so the two attach to different listeners.

In `route` mode, the reconciler SHALL create this OpenShift Route.

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: openshell-console
  namespace: <ns>
spec:
  host: console-<ns>.<base-domain>
  to:
    kind: Service
    name: openshell-console
  port:
    targetPort: http
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
```

The OpenShift router SHALL terminate TLS. It SHALL send HTTP to oauth2-proxy on Service port `http`. The Route SHALL use the OpenShift router certificate when no Route certificate is set.

#### Scenario: HTTP listener absent on the shared Gateway

- GIVEN a shared Gateway with no listener that matches `GATEWAY_API_HTTP_LISTENER_NAME`
- WHEN the reconciler creates the HTTPRoute
- THEN the HTTPRoute must report a not-accepted condition
- AND the reconciler must write a warning that names the missing listener
- AND it must not fail the gateway reconciliation

#### Scenario: OpenShift Route is admitted

- GIVEN the selected ingress mode is `route`
- AND the console Deployment is Ready
- WHEN an OpenShift router reports `Admitted=True` for the console Route
- THEN the console exposure must be Ready

#### Scenario: OpenShift Route is not admitted

- GIVEN the selected ingress mode is `route`
- WHEN no OpenShift router reports `Admitted=True` for the console Route
- THEN the reconciler must not publish `consoleAddress`
- AND it must write the Route admission reason when one exists
- AND it must not fail the gateway reconciliation

---

### Requirement: Console NetworkPolicies

The reconciler must create two NetworkPolicies. Existing gateway policies already select the gateway pod for ingress, so the namespace denies traffic by default from any other source. The ingress controller namespace is `GATEWAY_API_GATEWAY_NAMESPACE`, with the default value `openshift-ingress`. This namespace applies to the shared Gateway proxy and to the OpenShift router.

1. **`openshell-console-allow-router`** -- selects the console pod (`app.kubernetes.io/instance: openshell-console`); allows ingress on TCP `4180` from the selected ingress controller namespace.
2. **`openshell-gateway-allow-console`** -- selects the gateway pod (`app.kubernetes.io/instance: openshell-gateway`); allows ingress on TCP `8080` from the console pod in the same namespace.

#### Scenario: Console reaches the gateway

- GIVEN a routed gateway with the console deployed
- WHEN the dashboard connects to `openshell-gateway:8080`
- THEN `openshell-gateway-allow-console` must allow the connection
- AND without this policy the namespace default-deny posture would block it

---

### Requirement: Console Lifecycle and Cleanup

The reconciler must reconcile and remove the console together with the route and the gateway. The reconciler must use update-or-create for all console resources.

#### Scenario: Route removed from a gateway

- GIVEN a gateway that had a route and a console
- WHEN the route field is removed
- THEN the reconciler must delete all console resources: the Deployment, the Service, the HTTPRoute or OpenShift Route, the NetworkPolicies, and the `openshell-console-oauth2` Secret
- AND it must delete the console client `{name}-{id}-console`
- AND it must clear the `consoleAddress` field on the gateway

#### Scenario: Gateway deleted

- GIVEN a gateway with a console
- WHEN the reconciler gets a Gateway DELETED event
- THEN it must delete the console client
- AND the console namespaced resources go away with the other gateway resources
- AND Keycloak deletes the console mappers with the client

#### Scenario: Cleanup failure does not block

- GIVEN the reconciler removes a console
- WHEN it cannot delete the console client, because Keycloak is unavailable
- THEN it must log the error and the orphan `clientId`
- AND it must continue to remove the other resources

---

### Requirement: Console Provisioning Atomicity and Idempotency

The reconciler must treat the console Keycloak work (client, mappers, secret) as one atomic step. When a step fails, the reconciler must delete the console client and retry on the next cycle. Repeated reconciliation must not create duplicate resources.

#### Scenario: Mapper creation fails

- GIVEN the reconciler created the console client
- WHEN a mapper fails
- THEN it must delete the console client, which removes its mappers
- AND it must log the error and retry on the next cycle
- AND it must not deploy the console workload until the Keycloak work succeeds

---

### Requirement: Console Address Discovery

The reconciler must PATCH the console URL into a read-only `consoleAddress` field on the gateway. The management web-console and the CLI use this field to link to the console.

- Format: `https://console-<ns>.<base-domain>`.
- The reconciler sets it only once the console Deployment (dashboard + oauth2-proxy) is observed Ready, so the web-console's console button never appears before the console pod can serve.
- The reconciler sets it only when the exposure resource for the selected ingress mode is Ready. An HTTPRoute is Ready when one parent reports `Accepted=True` and `ResolvedRefs=True`. An OpenShift Route is Ready when one router reports `Admitted=True`.
- The reconciler clears it when the console pod is not Ready, when the console is removed, or when the base domain is unknown (for example, `GATEWAY_API_BASE_DOMAIN` is unset). A console that later goes unready has its address retracted, hiding the button.
- Readiness is observed both during provisioning (a prompt check once the gateway's route is ready) and continuously by the health reconciler, so the field self-heals as the console pod's readiness changes.

#### Scenario: Console button gated on console readiness

- GIVEN a routed gateway whose console resources are applied but whose console pod is not yet Ready
- THEN `consoleAddress` stays empty and the web-console does not offer the console button
- WHEN the console Deployment and its selected exposure resource become Ready
- THEN the reconciler sets `consoleAddress` to `https://console-<ns>.<base-domain>` and the button appears

---

## Configuration

### Gateway resource

| Field | Description |
|---|---|
| `consoleAddress` | Read-only. The control plane sets the console URL. Empty when no console runs. |

The gateway has no user field for the console. The console follows the route.

### Control-plane variables

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_INGRESS_MODE` | auto-detect | Selects `gateway-api`, `route`, or no managed ingress for both the gateway and its console |
| `GATEWAY_API_HTTP_LISTENER_NAME` | `https` | The `sectionName` of the shared Gateway HTTP listener for console HTTPRoutes; it has no effect in `route` mode |
| `HYPERSHELL_CONSOLE_IMAGE` | *(ImageDefaults default)* | The OpenShell dashboard image |
| `HYPERSHELL_OAUTH2_PROXY_IMAGE` | *(ImageDefaults default)* | The oauth2-proxy image |

The reconciler reuses `GATEWAY_API_BASE_DOMAIN` (see `openshell-gateway-routing.spec.md`) for the console hostname.

### ImageDefaults

The `ImageDefaults` interface (`internal/gateway/config.go`) must add `DefaultConsoleImage()` and `DefaultOAuth2ProxyImage()`. They use the same override order as the other images.

---

## Data model

The gateway kind must add a read-only `consoleAddress` field.

```sql
ALTER TABLE gateways ADD COLUMN console_address TEXT;
```

The REST API and the gRPC API must not let a user set or update `consoleAddress`.

---

## RBAC

The control-plane ServiceAccount already has create, update, patch, and delete access to `services`, `secrets`, `deployments`, and `networkpolicies`. Gateway API mode needs this rule in gateway namespaces:

```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["httproutes"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
```

OpenShift Route mode needs these rules. The `routes/custom-host` access permits the controller to set the explicit `spec.host` value.

```yaml
- apiGroups: ["route.openshift.io"]
  resources: ["routes"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["route.openshift.io"]
  resources: ["routes/custom-host"]
  verbs: ["create", "update"]
```

The console needs no new Keycloak permission. The `hypershell-keycloak-admin` account already manages clients, roles, and mappers.

---

## Prerequisites

1. **Ingress controller.** In `gateway-api` mode, the admin must add an HTTP listener to the shared Gateway (HTTPS/Terminate, port 443, wildcard `*.<base-domain>` certificate). The listener must accept HTTPRoutes from gateway namespaces. Its `sectionName` must match `GATEWAY_API_HTTP_LISTENER_NAME`. In `route` mode, an OpenShift ingress controller must admit Routes for `*.<base-domain>` and serve its router certificate.
2. **Dashboard image.** The upstream project ([Gkrumbach07/openshell-dashboard](https://github.com/Gkrumbach07/openshell-dashboard)) publishes the dashboard to `quay.io/gkrumbach07/openshell-dashboard` (per-commit `sha-<short>` tags plus `latest`). The control plane's `ImageDefaults` pin it by digest, so clusters pull it directly (imagePullPolicy `IfNotPresent`) with no build-from-source step; Kind pulls the same public image. Production should mirror the pinned digest into the platform registry and override `HYPERSHELL_CONSOLE_IMAGE`. The image contract (`OPENSHELL_GATEWAY_URL` as a `grpcs://` URL, mutual TLS through `GATEWAY_CA_CERT` plus `GATEWAY_CLIENT_CERT`/`GATEWAY_CLIENT_KEY`, and the `X-Forwarded-Access-Token` relay) is an upstream dependency.
3. **Keycloak realm.** The realm prerequisites in `openshell-gateway-keycloak.spec.md` apply. The console adds no realm-level object.

---

## Out of scope

- A central console with single sign-on across all gateways (one login, token exchange for each `aud`).
- The dashboard inside the management web-console.
- Changes to the OpenShell dashboard image.

---

## References

- [OpenShell Dashboard](https://github.com/Gkrumbach07/openshell-dashboard) -- the gateway URL and `X-Forwarded-Access-Token` contract
- [oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/) -- OIDC provider, PKCE, `pass-access-token`, `reverse-proxy`
- [oauth2-proxy #1714](https://github.com/oauth2-proxy/oauth2-proxy/issues/1714) -- a client secret is required with PKCE
- [Gateway API HTTPRoute](https://gateway-api.sigs.k8s.io/api-types/httproute/)
- [OpenShift Route](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/ingress_and_load_balancing/configuring-routes)
- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/)
- `openshell-gateway-keycloak.spec.md` -- the per-gateway client and the OIDC Role Bridge
- `openshell-gateway-routing.spec.md` -- the shared Gateway, hostnames, and NetworkPolicy pattern
