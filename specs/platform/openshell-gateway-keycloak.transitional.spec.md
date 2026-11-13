# OpenShell Gateway Keycloak Transitional Specification

**ID prefix:** OSG-KCLT
**Status:** Transitional
**Amends:** `openshell-gateway-keycloak.spec.md` (OSG-KCLK-02, Client ID Format)

> **Transitional.** This file keeps behavior that exists only for gateways
> provisioned before Keycloak client IDs became the gateway ID (PR #891,
> 2026-11). It is not part of the desired end state.
>
> **Remove when:** no live gateway has a stored client ID other than its own
> gateway ID.
>
> **Check:** on every long-lived hub API database, this query returns `0`:
>
> ```sql
> SELECT count(*) FROM gateways
> WHERE deleted_at IS NULL AND oidc->>'client_id' <> id;
> ```
>
> **On removal:**
>
> 1. Delete this file and its `OSG-KCLT` row in `specs/ids.md`.
> 2. Add its IDs to the Retired IDs table in `specs/id-rename-map.md`.
> 3. Delete the `-{gateway-id}` suffix branch in `isGatewayOwnedKeycloakClientID`
>    (`components/control-plane/internal/reconciler/reconciler.go`).
> 4. Run `make check`. It fails on every `Verifies:` marker that still names an
>    `OSG-KCLT` ID. Delete the earlier-format test cases there and the ID from
>    each marker.

---

## Purpose

Before PR #891, a gateway's Keycloak client ID was `{name}-{id}`: the gateway
name at provisioning time, a hyphen, and the gateway ID. Those clients cannot be
renamed in place, because the client ID is also the token audience. It is in the
gateway configuration, in issued tokens, and in users' CLI profiles. So existing
gateways keep their earlier client ID until they are deleted.

## Requirements

### Legacy compatibility: OSG-KCLT-01 -- Earlier {name}-{id} client IDs

In addition to the gateway-owned identity that OSG-KCLK-02 defines, a stored
client ID SHALL be accepted as gateway-owned when it is a non-empty prefix
followed by `-{gateway-id}`, and it meets every other OSG-KCLK-02 rule:
`client_id` and `audience` are both present and agree, and neither contains a
control character. The prefix is otherwise unrestricted, because gateway names
are not validated. It MAY contain spaces, non-ASCII characters, Unicode line or
paragraph separators, and formatting controls. Every path SHALL use such an
accepted identifier exactly as stored.

#### Scenario: OSG-KCLT-01.01 -- Gateway with an earlier client ID is reconciled with it

- GIVEN a gateway with ID `2FhMpQzXBz` has the stored client ID `my-gateway-2FhMpQzXBz`
- AND the gateway has since been renamed to `new-name`
- WHEN the control plane reconciles, deletes, assigns roles for, or configures CI federation for the gateway
- THEN each path SHALL use the client `my-gateway-2FhMpQzXBz`

#### Scenario: OSG-KCLT-01.02 -- Earlier client ID with unusual characters

- GIVEN a gateway's stored client ID is `Original gateway_日本語-2FhMpQzXBz`, or contains a Unicode line separator, paragraph separator, or bidi override before `-2FhMpQzXBz`
- WHEN the control plane resolves the gateway's client
- THEN it SHALL accept the identifier as gateway-owned
- AND every log line or error that includes it SHALL quote it
