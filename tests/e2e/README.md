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
does not impersonate a human. The primary gateway (P1.3 to P1.5) and the P2.1
sandbox gateway each get their own account.

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
- Retiring an account (P2.3 for the primary gateway, P2.1 for the sandbox
  gateway) revokes it, checks that the token endpoint then rejects it with
  `invalid_client` or `unauthorized_client`, and deletes it. Already issued
  tokens may stay valid until they expire; the check concerns new issuance.
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
