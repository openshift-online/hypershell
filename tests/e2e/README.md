# Gateway authentication

The existing `E2E_GATEWAY_AUTH=identity` default preserves password grants for
Kind/manual OpenShift and legacy exchange for PR environments. Long-mode user
and platform RBAC checks continue to use those identities.

For unattended short checks, set `E2E_GATEWAY_AUTH=service_account` alongside
`E2E_MODE=short`. The management identity can authenticate with
`E2E_OIDC_GRANT=client_credentials`, `E2E_OIDC_SA_CLIENT_ID`, and
`E2E_OIDC_SA_CLIENT_SECRET`. It needs `gateway:creator`, not `platform:admin`,
impersonation, token exchange, or Keycloak administrator access.

The suite creates a gateway under that identity, then calls the existing
`/gateways/{id}/service_accounts` API as its owner. The controller creates a
separate gateway service-account subject with exactly that gateway's
`openshell-admin` and `openshell-user` roles. This is automation operating under
its own authority; it does not impersonate a human. This mode explicitly tests
the public gateway service-account API rather than same-subject token exchange.

Credentials expire after two hours. The suite keeps the returned secret in a
0600 file in a run-private directory, uses it only at the expected HTTPS token
endpoint, and renews five-minute tokens before CLI operations. Gateway tokens
must have exactly the selected audience and expected subject and roles. The
gateway independently validates signatures and authorization. The suite checks
revocation stops token issuance, deletes the service account, and removes local
credentials even with `E2E_SKIP_CLEANUP=1`. Already issued tokens may remain valid
until expiry; revocation assertions concern new issuance.

The new mode is limited to short runs. Existing long/performance tests and
browser/CLI SSO configuration are unchanged. Run `make ci-test` for the shared
driver, mode selection, and credential-lifecycle regression tests.

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
