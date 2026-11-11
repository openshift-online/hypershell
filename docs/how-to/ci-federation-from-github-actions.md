# How-To: Keyless CI federation from GitHub Actions into a gateway

Run CI (GitHub Actions) against a HyperShell gateway with **no stored secret**. A CI
job mints its native OIDC token, exchanges it at the cluster Keycloak, and receives a
short-lived gateway-scoped OpenShell token. This guide is the practical companion to
the contract in
[`specs/platform/ci-trust-ext-api.spec.md`](../../specs/platform/ci-trust-ext-api.spec.md)
(PLAT-CITR); when this guide and the spec disagree, the spec wins.

## What you need first

Two things are platform-wide prerequisites that a `ci-federation:admin` sets up once,
not per repository (see PLAT-CITR-01 and PLAT-CITR-14):

- A `CIProvider` for your forge (for example `github-actions`), registered with its
  `idp_alias`, realized as a validation-only Keycloak identity provider.
- CI federation enabled on **every** control-plane instance whose cluster can win
  placement for your gateway. Because gateway placement is late-bound and the
  assigned `cluster_id` is immutable, a selector-placed gateway can land on any
  matching cluster, and the `CITrust` is realized only by that cluster's control
  plane. If a matching cluster is not CI-enabled, the trust silently does not realize
  there (PLAT-CITR-13). Either enable CI on all matching clusters, or pin the gateway
  to a CI-enabled cluster.

If you own the gateway, you declare the rest yourself - no human needs Keycloak
access.

## Step 1 - Declare a CITrust-backed gateway

Declare a `Gateway` and a `CITrust` for it. Two ways:

- **Declaratively in a repo's `.hypershell/`** (the control-plane `RepositoryReconciler`
  imports them as the repository owner, PLAT-CITR-10). See
  [`.hypershell/base/platform/gateway.yaml`](../../.hypershell/base/platform/gateway.yaml)
  and [`.hypershell/base/platform/ci-trust.yaml`](../../.hypershell/base/platform/ci-trust.yaml).
- **Directly** with `hsctl apply` (authorized by per-gateway tenancy, SEC-RBAC-22).

The `CITrust` names the `CIProvider` and the `Gateway` by name and sets the subject
predicate that is the real trust boundary (PLAT-CITR-04):

```yaml
apiVersion: hypershell.redhat.io/v1
kind: CITrust
metadata:
  name: github-actions-hypershell
spec:
  ci_provider_id: github-actions          # CIProvider, by name
  gateway_id: agents-gateway              # Gateway, by name
  subject:
    repository: openshift-online/hypershell
    ref_protected: true                    # only protected refs (main) get a role
  role: openshell-user
```

A non-protected ref correctly receives the exchange but **no role** - scope working
as designed, not a bug.

## Step 2 - Find your gateway's audience

The token-exchange `audience` is the gateway's Keycloak client id, which is the
gateway's **immutable API-server resource id** (not its name; see
[`openshell-gateway-keycloak.spec.md`](../../specs/platform/openshell-gateway-keycloak.spec.md)).
Look it up:

```shell
hsctl get gateway <gateway-id>   # oidc.client_id == oidc.audience == the gateway id
```

If you ever delete and recreate the gateway under the same name, it gets a **new**
id, so you must recreate the `CITrust` against it (its `gateway_id` is immutable) and
update the audience you pass below (PLAT-CITR-13.02).

## Step 3 - Mint the OIDC token and exchange it (GitHub Actions)

This is the verified recipe used by
[`.github/workflows/ci-federation-test.yml`](../../.github/workflows/ci-federation-test.yml).
The job needs `id-token: write`.

```yaml
permissions:
  contents: read
  id-token: write
steps:
  - name: Mint GitHub Actions OIDC token
    id: oidc
    run: |
      set -euo pipefail
      tok="$(curl -sS -H "Authorization: bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" \
        "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=openshell-fleet" | jq -r '.value')"
      echo "::add-mask::$tok"
      echo "token=$tok" >> "$GITHUB_OUTPUT"

  - name: Exchange for a gateway-scoped token
    id: exchange
    env:
      OIDC_TOKEN: ${{ steps.oidc.outputs.token }}
      KEYCLOAK_URL: https://keycloak.hyp0.infra.hypershell.app
      REALM: hypershell
      REQUESTER: ci-federation            # public, keyless requester client
      IDP_ALIAS: github-actions           # CIProvider.status_idp_alias
      GATEWAY_AUDIENCE: <your-gateway-id> # from Step 2
    run: |
      set -euo pipefail
      resp="$(curl -sS -X POST \
        "${KEYCLOAK_URL}/realms/${REALM}/protocol/openid-connect/token" \
        -d 'grant_type=urn:ietf:params:oauth:grant-type:token-exchange' \
        -d "client_id=${REQUESTER}" \
        -d "subject_token=${OIDC_TOKEN}" \
        -d "subject_issuer=${IDP_ALIAS}" \
        -d 'subject_token_type=urn:ietf:params:oauth:token-type:jwt' \
        -d "audience=${GATEWAY_AUDIENCE}")"
      access="$(echo "$resp" | jq -r '.access_token // empty')"
      [ -n "$access" ] || { echo "exchange failed: $resp" >&2; exit 1; }
      echo "::add-mask::$access"
      echo "access=$access" >> "$GITHUB_OUTPUT"
```

The exchanged token carries the nested claim `hypershell.roles: ["openshell-user"]`
(not a flat `hypershell.roles` key). That nested role is the definitive proof the
`CITrust` realized - the token-exchange grant alone does not put it there.

## Step 4 - Create a sandbox over the federated token

Install the matching OpenShell CLI for your gateway version and inject the exchanged
token into its per-gateway token store, then drive the gateway:

```shell
OPENSHELL_VERSION=v<gateway-version> sh scripts/install-openshell.sh   # installs to ~/.local/bin
GW=agents-gateway
mkdir -p "$HOME/.config/openshell/gateways/$GW"
cat > "$HOME/.config/openshell/gateways/$GW/oidc_token.json" <<EOF
{"access_token": "${ACCESS_TOKEN}", "issuer": "https://keycloak.hyp0.infra.hypershell.app/realms/hypershell", "client_id": "<your-gateway-id>"}
EOF
# metadata.json (gateway endpoint + name) is written by the CLI on first `openshell add`;
# for a non-interactive CI run, point the CLI at the gateway endpoint it expects.

openshell -g "$GW" sandbox create --name ci-$GITHUB_RUN_ID
openshell -g "$GW" sandbox list
```

The gateway must be running a build that serves client sandbox CRUD (v0.1.8 or
later); older builds accept the federated token but return `UNIMPLEMENTED` on sandbox
RPCs.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Exchange returns `invalid_token` / wrong audience | `audience` is the gateway name, not its id; or wrong gateway | Use the gateway's resource id (Step 2). |
| Exchange succeeds but `hypershell.roles` is null | Ref is not protected, or claims do not match the `CITrust` subject | Run on a protected ref; check `subject.repository`/`ref_protected`. |
| Exchange succeeds, role present, but nothing realized in Keycloak | The gateway's assigned cluster has CI federation disabled | Enable CI on that cluster, or re-home the gateway (PLAT-CITR-13/14). |
| Sandbox RPCs return `UNIMPLEMENTED` | Gateway build too old | Roll the gateway to v0.1.8+ (control-plane `GATEWAY_IMAGE` default). |
| `User already exists` / `invalid_token` on a second protected ref | Keycloak IdP first-broker-login links federated identity per subject | Known multi-ref limitation; delete the brokered `gh:<repo>` user, or use main-only CI for now. |

## See also

- Contract: [`specs/platform/ci-trust-ext-api.spec.md`](../../specs/platform/ci-trust-ext-api.spec.md) (PLAT-CITR)
- Data model: [`specs/platform/data-model.spec.md`](../../specs/platform/data-model.spec.md) (CIProvider/CITrust, PLAT-DATA-24/25)
- Platform federation design: gitops `specs/ci-oidc-federation.spec.md`
- Test harness: [`.github/workflows/ci-federation-test.yml`](../../.github/workflows/ci-federation-test.yml)
