# Gateway authentication

`E2E_GATEWAY_AUTH` selects how the Go suite (`tests/e2e`, run by `make e2e`)
authenticates to the gateways it creates.

| Value | Where it runs | Per-gateway token |
|-------|---------------|-------------------|
| `identity` (default) | long and short | The management identity's own token from `AcquireGatewayTokenWithRole`. With `E2E_OIDC_GRANT=client_credentials` this is a Keycloak token exchange, so the OIDC client needs exchange rights. Unchanged. |
| `service_account` | short only | A short-lived gateway service account's `client_credentials` token. No token exchange. |

Any other value fails `SetupSuite`, as does `service_account` outside
`E2E_MODE=short`. Long-mode user and platform RBAC checks keep using the
`identity` path.

## Service-account mode for unattended gates

```sh
E2E_MODE=short E2E_GATEWAY_AUTH=service_account \
E2E_OIDC_GRANT=client_credentials \
E2E_OIDC_SA_CLIENT_ID=... E2E_OIDC_SA_CLIENT_SECRET=... make e2e
```

The management client needs only the realm roles `gateway:creator` and
`hypershell-users`. It does not need token exchange (`standard.token.exchange.enabled`
can be `false`), `platform:admin`, impersonation, or Keycloak administrator
access. The suite uses none of those in this mode.

The suite creates each gateway under that identity, then calls the existing
`/gateways/{id}/service_accounts` API as the gateway's owner. The controller
creates a separate subject holding exactly that gateway's `openshell-admin` and
`openshell-user` roles. This is automation operating under its own authority; it
does not impersonate a human. The primary gateway (provisioned in suite setup,
asserted by P1.3 to P1.5) and the P2.1 sandbox gateway each get their own account.

Credential handling:

- Credentials expire after two hours. The secret lives only in memory or in a
  `0600` file in a `0700` run-private directory, and is never logged.
- The secret is sent only to the expected HTTPS token endpoint (the issuer in
  `E2E_OIDC_ISSUER` plus `/protocol/openid-connect/token`), without following
  redirects, and only after the credential is checked.
- Access tokens last about five minutes. They are renewed before they expire and
  before every CLI operation, by atomically replacing the CLI's `oidc_token.json`.
- A gateway token must have exactly the gateway's audience, the service account's
  subject and issuer, and exactly `openshell-admin` and `openshell-user`.
- Retiring an account (`TearDownSuite` for the primary gateway, P2.1 for the
  sandbox gateway) revokes it, checks that the token endpoint then rejects it with
  `invalid_client` or `unauthorized_client`, and deletes it. The primary gateway's
  account is retired in teardown rather than in P2.3, so it never races with P2.5's
  use of the primary gateway. Already issued tokens may stay valid until they
  expire; the check concerns new issuance.
- Teardown retires any account still live and removes the credential directory and
  the CLI token files on every path, including `E2E_SKIP_CLEANUP=1`. That variable
  keeps gateways for inspection, never credentials.

Short mode already skips every step that needs a second principal: P2.2
(developer and platform-admin RBAC), P2.4, P2.5, P2.6, P3.1, and the
device-authorization sub-check of P1.3 are long-only. The remaining steps act as
the management identity and the gateway service accounts.

If a gateway token request fails in `identity` mode on OpenShift, the error now
includes the last token endpoint response (HTTP status and OAuth error code), so
a refused token exchange is visible instead of only "never gained role".

Run `go -C tests/e2e test ./... -skip 'TestE2E$|TestMatrix$'` for the unit tests
of mode selection, the credential lifecycle, renewal, and claim checks. The
retired Bash suite's helpers (`gateway_service_account.sh` and `.py`) and their
tests remain for `make ci-test`; the Go suite does not use them.

# Running phases and steps in isolation

The shared primary gateway is provisioned once in suite setup through an idempotent
`ensurePrimaryGateway` helper, not inside the P1 steps, so any phase or any single
step can be reproduced on its own with `go test -run`. Every step is a named
subtest nested under its phase (`TestE2E/TestPhases/Phase-P<n>/P<n>.<m>-<slug>`), so
the same `-run` shape targets a whole phase or one step:

```sh
# A whole phase:
go -C tests/e2e test . -run 'TestE2E/TestPhases/Phase-P2' -v
# A single step (nested under its phase):
go -C tests/e2e test . -run 'TestE2E/TestPhases/.*/P2\.5' -v
go -C tests/e2e test . -run 'TestE2E/TestPhases/.*/P1\.2' -v
```

`make e2e` accepts an `E2E_RUN` filter that builds the right pattern for you. A
`Phase-P*` value runs the whole phase; any other value is treated as a step id:

```sh
make e2e E2E_RUN=Phase-P2      # the whole P2 phase
make e2e E2E_RUN='P2\.5'       # just the P2.5 step (escape the dot)
```

When the filter selects P1, or runs the full suite, the primary gateway is
provisioned eagerly at `Phase-P1` start. When it selects a later phase or an
individual P2/P3 step that reads the primary gateway (P1.1-P1.5, P2.5, P2.6, P2.9,
P2.11, P3.1), that step calls `ensurePrimaryGateway` and provisions it on demand.
A step that does not use the primary gateway (for example P0.1, P2.1, P2.4) skips
provisioning it entirely. The long-mode orphan reaper is seeded in `SetupSuite`;
an isolated `P2.3` run self-seeds it.

For fast local iteration, set `E2E_PRIMARY_GATEWAY_ID` to an existing running
gateway's id. Setup adopts that gateway instead of creating one and does not delete
it in teardown, so repeated single-step runs skip the provisioning wait.

## Failure diagnostics

Every failed Go E2E suite collects cluster diagnostics before teardown, whether
it runs locally, in GitHub Actions, or as a managed-cluster promotion gate. The
test log includes pod readiness and descriptions, platform and gateway workload
logs, events, Gateway API and OpenShift Route status, tracing state, and
OpenShell console state. Credential-shaped values and private keys are redacted
before emission.

Set `E2E_DIAGNOSTICS_DIR` to also write the redacted sections as `0600` files
under a per-run directory. GitHub Actions sets this variable and only uploads
the files; diagnostic collection itself remains part of the test. Collection is
best-effort and bounded to 45 seconds by default. Override that bound with
`E2E_DIAGNOSTICS_TIMEOUT` when a slow remote cluster needs more time.

The controller also has an opt-in test against a disposable Keycloak realm:

```sh
cd components/control-plane
# Set KEYCLOAK_INTEGRATION_URL, KEYCLOAK_INTEGRATION_REALM,
# KEYCLOAK_INTEGRATION_CLIENT_ID, and KEYCLOAK_INTEGRATION_CLIENT_SECRET
# for a provisioner in an isolated test instance, then:
go test ./internal/serviceaccountkeycloak -run TestIntegration -v
```

It provisions two gateways and scoped machine accounts, adds an unrelated
gateway-admin grant to each subject, and verifies audience and role isolation
plus revocation against real issued tokens. The test removes its resources.
