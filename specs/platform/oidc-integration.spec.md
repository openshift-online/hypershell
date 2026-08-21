# Platform OIDC Integration

**Date:** 2026-08-14
**Status:** Draft
**Related:** `openshell-gateway-oidc.spec.md` - per-gateway OIDC authentication; `../web-console/architecture.spec.md` - WEB-AUTH-01 through WEB-AUTH-03; `local-development.spec.md` - Kind cluster environment

---

## Purpose

Define how OIDC authentication integrates across the HyperShell platform: API server JWT validation, web console BFF session management, and deployment-specific wiring. Gateway-level OIDC is fully specified in `openshell-gateway-oidc.spec.md`; this specification covers the remaining components.

Today, the API server has no production-ready JWT configuration spec, the web console BFF has no OIDC implementation (WEB-AUTH-01 through WEB-AUTH-03 are defined but unbuilt), and local development cannot exercise end-to-end authentication. This specification closes those gaps.

---

## Architecture

```text
Browser
  │  1. GET /auth/login → 302 to IdP authorize endpoint (PKCE)
  │  2. User authenticates at identity provider
  │  3. IdP redirects to /auth/callback with authorization code
  │  4. BFF exchanges code for tokens, sets encrypted session cookie
  │
  │  Subsequent requests carry session cookie (HttpOnly, Secure, SameSite)
  ▼
Web Console BFF (Fastify)
  │  Decrypts session cookie → extracts access token
  │  Sets Authorization: Bearer <access_token> on proxied requests
  │  Refreshes token transparently when access token expires
  ▼
HyperShell REST API
  │  Validates JWT: issuer, audience, JWKS signature, expiry
  │  gRPC watch methods bypass JWT (trusted in-cluster services)
  ▼
PostgreSQL / Control Plane / Gateway
```

The platform supports two authentication modes:

| Mode | API Server | BFF | Use Case |
|------|-----------|-----|----------|
| **No-auth** | JWT disabled | Stateless proxy, no session | Development without auth overhead |
| **OIDC** | JWT enabled, issuer/audience/JWKS validation | Auth code + PKCE, encrypted cookie session | Production, staging, OIDC-enabled local dev |

Mode selection is deployment configuration. Application code consumes the same interfaces in both modes.

---

## API Server JWT Validation

The API server uses the upstream rh-trex-ai framework for JWT validation. When enabled, the framework validates Bearer tokens on incoming HTTP and gRPC requests against a JWKS endpoint and requires the configured issuer and API resource audience.

### Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--enable-jwt` | `true` (framework default) | Enable JWT validation |
| `--jwk-cert-url` | Red Hat SSO JWKS | JWKS endpoint URL(s) for JWT signature validation |
| `--grpc-jwk-cert-url` | (inherits `--jwk-cert-url`) | Override JWKS URL for gRPC validation |
| `--jwt-issuer` | empty | Exact expected `iss` claim; required by HyperShell OIDC deployments |
| `--jwt-audience` | empty | Required `aud` value; HyperShell uses `hypershell-frontend` as the management API resource audience |
| `--enable-authz` | `true` (framework default) | Enable authorization middleware |
| `--auth-bypass-paths` | `/healthcheck`, `/metrics`, `/openapi` | HTTP paths exempt from JWT validation |
| `--auth-bypass-methods` | Health, Reflection | gRPC methods exempt from JWT validation |

`hypershell-frontend` identifies the management API as the token resource; it is
not a restriction to the web-console OAuth client. First-party callers such as
hsctl and the control plane receive that audience from dedicated Keycloak
mappers and remain distinguishable through `azp`. Per-gateway clients receive
only their gateway audience, so their tokens SHALL be rejected by management
API endpoints.

### Environment System

The API server selects behavior via `API_ENV`. The existing `development` environment hardcodes `EnableJWT = false` in its `OverrideConfig()`, which runs after CLI flag parsing and cannot be overridden by flags.

| Environment | `API_ENV` | JWT | Authz | Use Case |
|-------------|-----------|-----|-------|----------|
| Development | `development` | Disabled (hardcoded) | Disabled | Local dev without auth |
| Development OIDC | `development_oidc` | Enabled | Disabled (mock) | Local dev with OIDC |
| Integration Testing | `integration_testing` | Mock | Mock | Automated tests |
| Production | `production` | Enabled | Enabled | Production deployments |

#### `development_oidc` Environment

A new environment file `e_development_oidc.go` SHALL be added alongside the existing `e_development.go`. It SHALL behave identically to the `development` environment except:

- `OverrideConfig` SHALL NOT set `c.Auth.EnableJWT = false`  -- the CLI flag `--enable-jwt=true` SHALL take effect
- `Flags()` SHALL set `"enable-jwt": "true"` and `"jwk-cert-url"` pointing to the configured JWKS endpoint
- `Flags()` SHALL set `"enable-authz": "false"`  -- authorization remains disabled; JWT validates identity only
- `Flags()` SHALL set `"enable-mock": "true"`  -- mock authz client, since authz is disabled

The environment SHALL be activated via `API_ENV=development_oidc`. The `environments.go` registration SHALL include the new environment in the environment map.

### gRPC Bypass

When JWT is enabled, trusted in-cluster services (e.g., the control plane) SHALL be exempt from JWT validation on their gRPC watch streams. The `--auth-bypass-methods` flag SHALL include:

- `/grpc.health.v1.Health/`
- `/grpc.reflection.v1alpha.ServerReflection/`
- `/hypershell.v1.FleetService/WatchFleets`
- `/hypershell.v1.GatewayService/WatchGateways`
- `/hypershell.v1.GatewayReleaseService/WatchGatewayReleases`
- `/hypershell.v1.ManagedClusterService/WatchManagedClusters`
- `/hypershell.v1.ManagedDatabaseService/WatchManagedDatabases`
- `/hypershell.v1.GatewayNetworkService/WatchGatewayNetworks`

Health and OpenAPI HTTP paths SHALL also bypass JWT: `/healthcheck`, `/metrics`, `/api/hypershell/v1/openapi`, `/openapi`.

---

## Web Console BFF OIDC

The BFF SHALL implement WEB-AUTH-01 through WEB-AUTH-03 from `specs/web-console/architecture.spec.md`. The implementation uses `openid-client` v6.x (already in the selected stack) and encrypted cookies via `@fastify/secure-session`.

### Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `OIDC_ISSUER` | Yes (to enable OIDC) | (unset  -- no-auth mode) | OIDC issuer URL for discovery |
| `OIDC_CLIENT_ID` | Yes | - | OAuth 2.0 client identifier |
| `OIDC_REDIRECT_URI` | No | `{request origin}/auth/callback` | OAuth 2.0 redirect URI |
| `OIDC_POST_LOGOUT_REDIRECT_URI` | No | `{request origin}/` | Post-logout redirect URI |
| `SESSION_SECRET` | Yes | - | 32-byte hex-encoded key for cookie encryption |
| `SESSION_TTL_SECONDS` | No | `28800` (8 hours) | Session cookie max age |

When `OIDC_ISSUER` is unset, the BFF SHALL operate in no-auth mode (WEB-AUTH-00)  -- identical to today's behavior. When `OIDC_ISSUER` is set, `SESSION_SECRET` and `OIDC_CLIENT_ID` SHALL be required; missing values SHALL fail BFF startup with a clear error message.

### OIDC Endpoints

The BFF SHALL implement the OAuth 2.0 authorization code flow with PKCE:

1. **`GET /auth/login`**  -- Generate PKCE code verifier + challenge, state, and nonce. Store them in a short-lived encrypted cookie. Redirect to the IdP authorization endpoint with `response_type=code`, `code_challenge`, `state`, `nonce`, `scope=openid email profile`, and `client_id`.

2. **`GET /auth/callback`**  -- Validate `state` against the stored value. Exchange the authorization code for tokens using the PKCE code verifier. Validate the ID token (`issuer`, `audience`, `nonce`, `exp`). Store the access token, refresh token, and ID token claims in an encrypted session cookie. Clear the temporary PKCE cookie. Redirect to `/` (or a stored return-to path).

3. **`GET /auth/logout`**  -- Clear the session cookie. Redirect to the IdP `end_session_endpoint` with `id_token_hint` and `post_logout_redirect_uri` to perform RP-initiated logout. The IdP session SHALL be terminated.

4. **`GET /auth/session`**  -- Return a minimal JSON session resource for the browser:
   ```json
   {
     "authenticated": true,
     "user": {
       "sub": "idp-user-id",
       "preferred_username": "admin",
       "email": "admin@example.com",
       "name": "Admin User"
     },
     "roles": ["hypershell-admins", "hypershell-users"],
     "expires_at": 1723401600
   }
   ```
   When unauthenticated, return `{ "authenticated": false }`. This endpoint SHALL NOT expose tokens.

### Session Cookies

Session state is split across two independently encrypted cookies so that no single cookie approaches the ~4KB per-cookie browser limit once refresh and ID tokens are stored server-side. Both cookies share the same encryption key, flags, and lifetime, and both are set atomically on login, refresh, and logout. `@fastify/secure-session` supports this natively through named sessions (an array of session options, each with its own `sessionName` and `cookieName`).

- **`session` (hot-path identity cookie)**  -- carries the access token, the access-token expiry timestamp, and display claims (sub, preferred_username, email, name, roles). Read on every proxied `/api/*` request.
- **`session_tok` (refresh cookie)**  -- carries the refresh token and the ID token. Read only when refreshing the access token or performing RP-initiated logout.

Common attributes for both cookies:

- Names: `session` and `session_tok` (the `__Host-` prefix requires the `Set-Cookie` response to use HTTPS; the BFF receives HTTP behind the TLS-terminating gateway, so the prefix cannot be used)
- Flags: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`
- Encryption: `@fastify/secure-session` named sessions with sodium-based secretbox (NaCl)
- Login SHALL rotate both cookies (new encrypted values after authentication)
- Each cookie SHALL remain under the 4KB browser limit after encryption and Base64 encoding. The BFF SHALL store only the claims needed to render the console plus the access token it must forward; it SHALL NOT copy the full ID-token or userinfo payload into the identity cookie.

### Token Refresh

The BFF SHALL keep the proxied access token valid without user interaction for as long as the refresh token remains valid.

- **Proactive refresh.** Before proxying an `/api/*` request, if the access token is missing or within a fixed skew window (30 seconds) of its expiry, the BFF SHALL exchange the stored refresh token for a new token set before forwarding.
- **Reactive refresh.** If the upstream API rejects a proxied request with 401 despite a token the BFF believed valid (clock skew, server-side revocation, key rotation), the BFF SHALL attempt exactly one refresh-and-retry before treating the request as a re-authentication event.
- **Rotation.** The BFF SHALL persist the rotated refresh token returned by the IdP into `session_tok` and SHALL NOT reuse a refresh token after a successful exchange.
- **Single-flight.** Concurrent requests carrying the same session SHALL share one in-flight refresh within a BFF instance, so a burst of parallel `/api/*` calls performs a single token exchange and avoids refresh-token reuse detection at the IdP.
- **Failure.** When refresh fails (expired or revoked refresh token, IdP unavailable), the BFF SHALL clear both session cookies and signal re-authentication per the Re-authentication requirement below. It SHALL NOT surface a raw provider error to the browser.

### Logout

The BFF clears both session cookies and redirects to the IdP `end_session_endpoint`. Because the ID token is stored in `session_tok`, the BFF SHALL include it as the `id_token_hint` parameter so the IdP terminates the session without a confirmation prompt, along with `post_logout_redirect_uri`.

### Re-authentication (no dead ends)

An invalid, expired, or unrefreshable session SHALL NOT surface as a generic application error. The user SHALL always be routed to the identity provider to re-authenticate.

- **Browser navigations.** A GET for an application route without a usable session SHALL 302 to `/auth/login`, which 302s to the IdP. `/auth/login` SHALL accept an optional `return_to` value, validate that it is a same-origin absolute path (no scheme, host, or protocol-relative `//` form), store it in the login session, and redirect to it after a successful callback.
- **API (fetch) requests.** Because a cross-origin 302 cannot drive a `fetch`, the BFF SHALL return the 401 re-authentication signal described in API Proxy Changes. The browser API transport SHALL detect this signal and perform a full-page navigation to `login_url`, preserving the current route as `return_to`. No generic error, retry banner, or permission-denied message SHALL be shown for an authentication failure.
- **Distinct from authorization.** A 403 from the API (genuine RBAC denial) SHALL remain a "denied" state and SHALL NOT trigger re-authentication. Only authentication failures (missing, expired, or revoked session) route to the IdP.
- **Observability.** The transport SHALL NOT write raw console output or ad-hoc telemetry for an authentication failure; it MAY publish a typed session-expiry fact through the domain probe fan-out before redirecting.

### CSRF Protection

The BFF SHALL validate the `Origin` header on all state-changing requests (POST, PATCH, PUT, DELETE). Requests without a valid same-origin `Origin` header SHALL be rejected with 403. CORS SHALL default to same-origin only.

### API Proxy Changes

When OIDC is enabled, the `/api/*` proxy SHALL:
- Extract the access token from the identity cookie
- Ensure the token is valid before forwarding, refreshing proactively per the Token Refresh requirement
- Set `Authorization: Bearer <access_token>` on upstream requests
- Remove any client-supplied `Authorization` header (the browser SHALL NOT send tokens)
- When no valid session exists and refresh is not possible, respond with 401 carrying a machine-readable re-authentication signal (a JSON body `{ "error": "reauth_required", "login_url": "/auth/login?return_to=<path>" }` and a `WWW-Authenticate: Bearer error="invalid_token"` header) rather than a generic error

When OIDC is not enabled (no-auth mode), the proxy SHALL behave as today  -- no auth header, no session check.

---

## Identity Provider Configuration

The platform uses Keycloak as the identity provider. In production, a downstream Keycloak brokers authentication to Red Hat SSO. In local development, a standalone Keycloak instance mirrors the same topology.

### `hypershell-frontend` Client

The `hypershell-frontend` client SHALL be used by the web console BFF for the authorization code flow. It is a public client secured by PKCE  -- no client secret.

| Setting | Value | Rationale |
|---------|-------|-----------|
| `publicClient` | `true` | BFF uses PKCE, not a client secret |
| `standardFlowEnabled` | `true` | Authorization code flow |
| `directAccessGrantsEnabled` | `true` | Retained for CLI password-grant testing |
| `redirectUris` | Deployment-specific console origins | Restrict redirect targets; wildcard is an open redirect vulnerability |
| `webOrigins` | Deployment-specific console origins | Restrict CORS |
| `defaultClientScopes` | `openid`, `email`, `profile` | Standard OIDC scopes |

Protocol mappers (retained as-is):
- **Audience mapper**  -- includes `hypershell-frontend` in the `aud` claim of the access token
- **Sub mapper**  -- includes `sub` in the access token
- **Realm roles mapper**  -- maps realm roles to the `groups` claim in all token types

The BFF validates the `aud` claim matches `OIDC_CLIENT_ID` (i.e., `hypershell-frontend`).

### `hypershell-provisioner` Client

The `hypershell-provisioner` client is a confidential service account used for automated Keycloak administration (e.g., per-gateway client provisioning). It is not used by the BFF or browser. See `openshell-gateway-credentials.spec.md` for its role in gateway OIDC provisioning.

---

## Deployment Configuration

### Production / Staging

| Component | Configuration |
|-----------|--------------|
| API server | `API_ENV=production`, `--enable-jwt=true`, `--jwk-cert-url=<production JWKS>`, `--jwt-issuer=<production issuer>`, `--jwt-audience=hypershell-frontend`, `--enable-authz=true` |
| BFF | `OIDC_ISSUER=<production issuer>`, `OIDC_CLIENT_ID=hypershell-frontend`, `SESSION_SECRET=<managed secret>` |
| Gateway | OIDC configured per `openshell-gateway-oidc.spec.md` |
| Keycloak | Downstream Keycloak brokering to Red Hat SSO |
| `redirectUris` | Production console origin(s) |

### Local Kind Development

OIDC is always enabled in the Kind cluster. `make kind-up` configures all components for OIDC authentication automatically.

#### Behavior

`make kind-up` SHALL:

1. Deploy the API server with `--enable-jwt=true`, `--jwk-cert-url`, `--jwt-issuer=https://keycloak.hypershell.localhost/realms/hypershell`, `--jwt-audience=hypershell-frontend`, `--auth-bypass-paths`, and `--auth-bypass-methods` flags via Kustomize JSON patch (direct flag override avoids dependency on `API_ENV=development_oidc` which may not exist in baseline images)
2. Deploy the web console BFF with:
   - `OIDC_ISSUER=https://keycloak.hypershell.localhost/realms/hypershell`
   - `OIDC_CLIENT_ID=hypershell-frontend`
   - `SESSION_SECRET` generated via `openssl rand -hex 32` during `kind-up` and stored in a Kubernetes Secret
3. Create the gateway resource with OIDC configuration per `local-development.spec.md`
4. Print OIDC connection information in the banner:
   ```
   Keycloak:     https://keycloak.hypershell.localhost (admin/admin)
   Login:        https://console.hypershell.localhost/auth/login
   Test users:   admin/admin (admins + users), developer/developer (users only)
   ```

#### Keycloak Client Configuration (Kind)

The `hypershell-frontend` client in the Kind Keycloak realm SHALL use restricted redirect URIs:

| Setting | Value |
|---------|-------|
| `redirectUris` | `["https://console.hypershell.localhost/*", "https://console.hypershell.localhost:*/*"]` |
| `webOrigins` | `["https://console.hypershell.localhost", "https://console.hypershell.localhost:*"]` |

The ephemeral port variant (`console.hypershell.localhost:*`) covers Kind setups without sudo port forwarding.

#### CLI Output

- The `kind-up` banner SHALL include the OIDC section shown above
- `make kind-status` SHALL report OIDC as active and show the Keycloak URL and test credentials

---

## Requirements

### Requirement: API Server JWT Validation

The API server SHALL support JWT validation against a configurable JWKS endpoint. The `development_oidc` environment SHALL enable JWT while retaining development-mode conveniences.

#### Scenario: JWT Validation Enabled
- GIVEN the API server is started with `API_ENV=development_oidc`
- WHEN a request arrives without a valid Bearer token
- THEN the API server SHALL reject the request with 401 Unauthorized
- AND the response SHALL include a `WWW-Authenticate` header

#### Scenario: Valid Token Accepted
- GIVEN the API server is started with JWT enabled
- AND a valid JWT is obtained from the configured IdP
- WHEN a request is made with `Authorization: Bearer <token>`
- THEN the API server SHALL validate the JWT signature against the JWKS endpoint
- AND the token `iss` SHALL exactly match the configured issuer
- AND the token `aud` SHALL contain `hypershell-frontend`
- AND the request SHALL be processed normally

#### Scenario: Token Issued for Another Resource Is Rejected
- GIVEN the API server is started with issuer and audience validation enabled
- AND a correctly signed token has a missing or different `iss`
- OR its `aud` does not contain `hypershell-frontend`
- WHEN the token is presented to an HTTP or gRPC management endpoint
- THEN the API server SHALL reject the request as unauthenticated

#### Scenario: gRPC Watch Streams Bypass JWT
- GIVEN the API server is started with JWT enabled
- WHEN the control plane connects via gRPC watch streams
- THEN the gRPC methods SHALL be exempted from JWT validation
- AND the control plane SHALL function without an IdP token

#### Scenario: Development Environment Unchanged
- GIVEN the API server is started with `API_ENV=development` (default)
- THEN JWT validation SHALL remain disabled
- AND all existing no-auth behavior SHALL be preserved

#### Scenario: Health and OpenAPI Bypass JWT
- GIVEN the API server is started with JWT enabled
- WHEN a request is made to `/healthcheck` or `/api/hypershell/v1/openapi`
- THEN the request SHALL be processed without JWT validation

### Requirement: BFF OIDC Authorization Code Flow

The web console BFF SHALL implement OAuth 2.0 authorization code flow with PKCE when configured with an OIDC issuer. This fulfills WEB-AUTH-01.

#### Scenario: Unauthenticated Browser Access
- GIVEN the BFF is configured with `OIDC_ISSUER`
- WHEN an unauthenticated browser requests any application route
- THEN the BFF SHALL redirect to `/auth/login`
- AND `/auth/login` SHALL redirect to the IdP authorization endpoint with PKCE parameters

#### Scenario: Successful Authentication
- GIVEN the user has authenticated at the IdP
- WHEN the IdP redirects to `/auth/callback` with an authorization code
- THEN the BFF SHALL exchange the code for tokens
- AND validate the ID token
- AND set an encrypted session cookie
- AND redirect to the application

#### Scenario: Authenticated API Proxy
- GIVEN the browser has a valid session cookie
- WHEN the browser requests `/api/hypershell/v1/gateways`
- THEN the BFF SHALL decrypt the session cookie
- AND set `Authorization: Bearer <access_token>` on the upstream request
- AND proxy the response back to the browser

#### Scenario: Silent Token Refresh
- GIVEN the access token has expired or is within the skew window
- AND the refresh token is still valid
- WHEN the browser makes an API request
- THEN the BFF SHALL exchange the refresh token for a new token set before forwarding
- AND persist the rotated refresh token
- AND the request SHALL succeed without any browser-visible interruption

#### Scenario: Full RP-Initiated Logout
- GIVEN the browser has an active session
- WHEN the user navigates to `/auth/logout`
- THEN the BFF SHALL clear the session cookie
- AND redirect to the IdP `end_session_endpoint` with `id_token_hint`
- AND the IdP session SHALL be terminated

#### Scenario: No-Auth Mode Preserved
- GIVEN the BFF is started without `OIDC_ISSUER`
- THEN the BFF SHALL operate in no-auth mode
- AND no session, cookie, or authentication logic SHALL be active
- AND the `/api/*` proxy SHALL forward requests without auth headers

### Requirement: BFF Session Security

The BFF session SHALL meet the security requirements in WEB-AUTH-02.

#### Scenario: Cookie Attributes
- GIVEN the BFF is configured with OIDC
- WHEN a session cookie is set
- THEN the cookie SHALL be `HttpOnly`, `Secure`, `SameSite=Lax`
- AND the cookie SHALL use the narrowest practical path (`/`)
- AND JavaScript SHALL NOT be able to read the session identifier

#### Scenario: Session Rotation on Login
- GIVEN a browser has an existing session cookie
- WHEN the user completes a new login flow
- THEN the old session SHALL be replaced with a new encrypted cookie
- AND the previous cookie value SHALL NOT be valid

#### Scenario: CSRF Protection
- GIVEN the BFF is configured with OIDC
- WHEN a POST/PATCH/PUT/DELETE request arrives without a valid same-origin `Origin` header
- THEN the request SHALL be rejected with 403
- AND the response SHALL NOT include any protected data

### Requirement: BFF Browser Session Contract

The BFF SHALL expose a session resource per WEB-AUTH-03.

#### Scenario: Authenticated Session Resource
- GIVEN the browser has a valid session
- WHEN the browser requests `GET /auth/session`
- THEN the response SHALL contain display identity, roles, and expiry
- AND the response SHALL NOT contain access tokens, refresh tokens, or provider secrets

#### Scenario: Unauthenticated Session Resource
- GIVEN the browser has no session
- WHEN the browser requests `GET /auth/session`
- THEN the response SHALL contain `{ "authenticated": false }`

### Requirement: Browser Identity and Sign-Out

The console SHALL display the authenticated user's identity in the masthead and provide a sign-out control that performs full RP-initiated logout. Identity SHALL come from the `/auth/session` resource; the browser SHALL NOT read tokens.

#### Scenario: Authenticated Identity in the Masthead
- GIVEN an authenticated session
- WHEN the console shell renders
- THEN the masthead SHALL show an identity menu labeled with the user's display name from `/auth/session`
- AND no token SHALL be exposed to the browser

#### Scenario: Sign Out Performs Full Logout
- GIVEN the user opens the identity menu
- WHEN the user selects sign out
- THEN the browser SHALL navigate to `/auth/logout`
- AND the BFF SHALL clear both session cookies and redirect to the IdP `end_session_endpoint` with `id_token_hint`
- AND the IdP session SHALL be terminated

#### Scenario: No Identity Menu When Unauthenticated
- GIVEN no session, or the BFF running in no-auth mode
- WHEN the console shell renders
- THEN no identity menu SHALL be shown

### Requirement: Silent Token Refresh and Rotation

The BFF SHALL maintain a valid access token without user interaction while the refresh token is valid, rotating refresh tokens and coalescing concurrent refreshes.

#### Scenario: Proactive Refresh Before Expiry
- GIVEN a session whose access token is within 30 seconds of expiry
- WHEN the browser makes an `/api/*` request
- THEN the BFF SHALL refresh the token before forwarding
- AND set the new access token on the upstream request

#### Scenario: Reactive Refresh on Upstream 401
- GIVEN the upstream API rejects a proxied request with 401
- AND the refresh token is valid
- WHEN the BFF receives the 401
- THEN the BFF SHALL refresh once and retry the request
- AND return the retried response on success

#### Scenario: Refresh Token Rotation Persisted
- GIVEN a successful refresh that returns a new refresh token
- WHEN the BFF stores the token set
- THEN the rotated refresh token SHALL replace the previous one in `session_tok`
- AND the previous refresh token SHALL NOT be reused

#### Scenario: Concurrent Requests Coalesce
- GIVEN several `/api/*` requests carrying the same expired session arrive together at one BFF instance
- WHEN the BFF refreshes
- THEN a single refresh exchange SHALL occur
- AND all coalesced requests SHALL proceed with the refreshed token

### Requirement: Re-authentication Without Dead Ends

An invalid, expired, or unrefreshable session SHALL route the user to the identity provider to re-authenticate and SHALL NOT surface as a generic application error. This refines WEB-AUTH-03.

#### Scenario: Expired Session on Navigation
- GIVEN a browser whose session cannot be refreshed
- WHEN the browser requests an application route
- THEN the BFF SHALL redirect to `/auth/login`
- AND `/auth/login` SHALL redirect to the IdP with the current route preserved as `return_to`

#### Scenario: Expired Session on API Request
- GIVEN a browser whose session cannot be refreshed
- WHEN the browser makes an `/api/*` request
- THEN the BFF SHALL respond 401 with a re-authentication signal (`reauth_required` and a `login_url`)
- AND the browser SHALL perform a full-page navigation to the login URL
- AND no generic error or permission-denied message SHALL be shown

#### Scenario: Return To Original Route
- GIVEN a user was on `/gateways/{id}` when the session expired
- WHEN re-authentication completes
- THEN the browser SHALL land back on `/gateways/{id}`

#### Scenario: Open-Redirect Rejected
- GIVEN a `return_to` value that is not a same-origin absolute path (an absolute URL, a scheme, or a protocol-relative `//host`)
- WHEN `/auth/login` processes it
- THEN the BFF SHALL ignore it and use the default post-login path

#### Scenario: Authorization Denial Is Not Re-authentication
- GIVEN an authenticated user without access to a resource
- WHEN the API returns 403
- THEN the console SHALL show a denied state
- AND SHALL NOT redirect to the identity provider

### Requirement: Chunked Session Cookies Within Browser Limits

Session state SHALL be split so that no single cookie exceeds the browser per-cookie size limit, even with refresh and ID tokens stored server-side.

#### Scenario: Two Encrypted Cookies
- GIVEN a completed login
- WHEN the BFF sets the session
- THEN identity and access-token state SHALL be written to the `session` cookie
- AND the refresh and ID tokens SHALL be written to the `session_tok` cookie
- AND each cookie SHALL remain under the 4KB browser limit after encryption

#### Scenario: Tokens Never Reach the Browser
- GIVEN a valid session
- WHEN the browser reads `GET /auth/session` or inspects cookies via JavaScript
- THEN no access token, refresh token, or ID token SHALL be observable

### Requirement: Kind OIDC Always-On

OIDC in the local Kind cluster SHALL always be enabled. `make kind-up` configures all components for OIDC authentication automatically.

#### Scenario: OIDC Configured on kind-up
- WHEN a developer runs `make kind-up`
- THEN the API server SHALL use `API_ENV=development_oidc`
- AND the BFF SHALL be configured with OIDC env vars
- AND the gateway SHALL be created with OIDC configuration
- AND the banner SHALL display OIDC connection information

#### Scenario: OIDC Status Reporting
- WHEN a developer runs `make kind-status`
- THEN the output SHALL show OIDC as active
- AND display the IdP URL and test user credentials

### Requirement: Identity Provider Client Security

The `hypershell-frontend` client SHALL be configured with deployment-appropriate redirect URI restrictions.

#### Scenario: Redirect URI Restricted
- GIVEN a Keycloak realm is configured for HyperShell
- THEN `hypershell-frontend` redirect URIs SHALL be restricted to the console origin for that deployment
- AND wildcard redirect URIs SHALL NOT be used

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| New `development_oidc` environment instead of modifying `development` | Preserves the existing no-auth developer experience. The `development` environment is the common case; OIDC is opt-in for contributors working on auth flows. Avoids breaking `make run-no-auth` semantics. |
| Encrypted cookies instead of server-side session store | Keeps the BFF stateless and horizontally scalable with zero infrastructure beyond what already exists. No Redis, no session table, no cleanup jobs. Cookie size is manageable with Keycloak's token payload. Revocation limitations are acceptable  -- short access token TTL + refresh rotation means a revoked user is bounced within minutes. A server-side store MAY be introduced later if revocation requirements demand it. |
| `@fastify/secure-session` for cookie encryption | Sodium-based secretbox (NaCl) is the gold standard for symmetric encryption. The library is maintained by the Fastify team and integrates natively. `iron-session` is an alternative but adds an extra dependency outside the Fastify ecosystem. |
| RP-initiated logout (full IdP session termination) | Clearing the cookie alone leaves the IdP session alive  -- the user could re-authenticate without credentials until TTL expires. Full logout is the expected UX for an enterprise console. One extra redirect is negligible. |
| Control plane authenticates with its own service account | The control plane obtains JWTs via `client_credentials` grant using a dedicated `hypershell-control-plane` Keycloak client. This is the standard pattern for service-to-service auth with the rh-trex-ai framework (the JWT interceptor validates tokens on all gRPC methods). The service account is least-privilege ready for future RBAC enforcement. |
| Shared management API resource audience | Web-console, hsctl, and control-plane tokens carry `aud=hypershell-frontend` because they call the same management API; `azp` identifies the OAuth client. Gateway-specific tokens carry only their gateway audience and cannot be replayed against the management API. |
| OIDC always-on in Kind | OIDC is the only supported authentication method. Running without it masks integration issues and diverges from production. |
| `hypershell-frontend` client reused for BFF | The client already exists with the correct audience mapper and role claims. Creating a separate BFF client would duplicate configuration and require additional Keycloak provisioning. PKCE secures the public client adequately for a BFF. |
| Restrict `redirectUris` from wildcard | Wildcard redirect URIs are an OAuth security anti-pattern (open redirect). Restricting to the deployment's console origin prevents authorization code interception. |
| `directAccessGrantsEnabled` retained | Password grant is used by CLI tooling and curl-based testing in local dev. Disabling it would break the documented CLI authentication flow in `openshell-gateway-oidc.spec.md`. |
| Session secret as deployment configuration | Each deployment generates or provisions its own encryption key. In Kind, `openssl rand -hex 32` during `kind-up` stored in a Kubernetes Secret. In production, provisioned via the deployment's secret management system. |
| Chunked session cookies (`session` + `session_tok`) | Storing the access token, refresh token, and ID token together would push a single encrypted cookie past the ~4KB browser limit (a limit this project has hit before). Splitting a hot-path identity cookie from a refresh cookie keeps each well under the limit and keeps the per-request read small. Both use the same key and are set atomically via `@fastify/secure-session` named sessions. |
| In-process single-flight refresh instead of a shared lock | The BFF is stateless (encrypted cookies, no shared store), so a distributed lock would add infrastructure the design deliberately avoids. Coalescing concurrent refreshes within an instance handles the common case (an SPA firing parallel requests), and a failed refresh falls back to re-authentication, which is safe. Session affinity MAY be enabled if cross-replica refresh races become material. |
| Redirect to the IdP on any authentication failure instead of showing an error | A stale or revoked session is not an application error the user can act on; the only recovery is to re-authenticate. Routing straight to the IdP (302 for navigations, a 401 re-authentication signal that the transport turns into a full-page redirect for fetches) removes dead ends. 403 authorization denials remain distinct and do not redirect. |

---

## References

- `specs/web-console/architecture.spec.md`  -- WEB-AUTH-00 through WEB-AUTH-03, WEB-BFF-01
- `specs/platform/local-development.spec.md`  -- Kind cluster environment, Keycloak configuration
- `specs/platform/openshell-gateway-oidc.spec.md`  -- Gateway OIDC configuration and CLI auth flow
- `specs/platform/openshell-gateway-credentials.spec.md`  -- Per-gateway Keycloak client provisioning
- [openid-client v6.x](https://github.com/panva/openid-client)  -- OIDC Relying Party library
- [@fastify/secure-session](https://github.com/fastify/fastify-secure-session)  -- Sodium-based encrypted cookie sessions
- [OAuth 2.0 for Browser-Based Apps (BCP 212)](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-browser-based-apps)  -- BFF pattern recommendation
