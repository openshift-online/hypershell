#!/usr/bin/env bash
# Unit tests for deploy/base/keycloak/render-realm-config.py.
# Reproduces the OpenShift "Invalid parameter: redirect_uri" failure: a Keycloak
# pod recycle re-imports the realm, so console redirect URIs must be in the
# rendered JSON (not only patched via the admin API after boot).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RENDER="${SCRIPT_DIR}/render-realm-config.py"
REALM_YAML="${SCRIPT_DIR}/keycloak.yaml"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "${label}" "${want}" "${got}"
  fi
}

extract_realm() {
  python3 - "$REALM_YAML" <<'PY'
import sys
from pathlib import Path
text = Path(sys.argv[1]).read_text()
marker = "hypershell-realm.json: |"
idx = text.index(marker) + len(marker)
body = []
for line in text[idx:].splitlines():
    if line.startswith("    "):
        body.append(line[4:])
        continue
    if line.strip() == "":
        body.append("")
        continue
    break
print("\n".join(body).strip())
PY
}

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT
extract_realm >"${WORKDIR}/hypershell-realm.json"
python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "${WORKDIR}/hypershell-realm.json"

# --- Kind / no console host: localhost redirect URIs stay, GitHub IdP off ---
env -i PATH="${PATH}" python3 "${RENDER}" \
  "${WORKDIR}/hypershell-realm.json" "${WORKDIR}/kind.json"
kind_redirects="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); client=next(c for c in realm["clients"] if c["clientId"]=="hypershell-frontend"); print(json.dumps(client["redirectUris"]))' "${WORKDIR}/kind.json")"
assert_eq '["https://console.hypershell.localhost/*", "https://console.hypershell.localhost:*"]' \
  "${kind_redirects}" "Kind keeps localhost frontend redirect URIs"
kind_idp="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); print(json.dumps(next(i for i in realm["identityProviders"] if i["alias"]=="github")["enabled"]))' "${WORKDIR}/kind.json")"
assert_eq 'false' "${kind_idp}" "Kind leaves GitHub IdP disabled as JSON boolean"
kind_e2e="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); print(json.dumps(next(c for c in realm["clients"] if c["clientId"]=="hypershell-e2e")["enabled"]))' "${WORKDIR}/kind.json")"
assert_eq 'false' "${kind_e2e}" "Kind leaves hypershell-e2e disabled as JSON boolean"

# --- OpenShift console host: exact callback URIs, no localhost wildcards ---
env -i PATH="${PATH}" \
  HYPERSHELL_CONSOLE_HOST='web-console-hypershell-ci-pr-267.apps.rosa.example.com' \
  PR_ENV_GITHUB_IDP_ENABLED=true \
  PR_ENV_GITHUB_CLIENT_ID='Iv1.example' \
  PR_ENV_GITHUB_CLIENT_SECRET='s3cr3t' \
  PR_ENV_GITHUB_ORG='openshift-online' \
  HYPERSHELL_E2E_CLIENT_ENABLED=true \
  HYPERSHELL_E2E_CLIENT_SECRET='e2e-secret' \
  python3 "${RENDER}" "${WORKDIR}/hypershell-realm.json" "${WORKDIR}/pr.json"

pr_redirects="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); client=next(c for c in realm["clients"] if c["clientId"]=="hypershell-frontend"); print(json.dumps(client["redirectUris"]))' "${WORKDIR}/pr.json")"
assert_eq '["https://web-console-hypershell-ci-pr-267.apps.rosa.example.com/auth/callback", "https://web-console-hypershell-ci-pr-267.apps.rosa.example.com"]' \
  "${pr_redirects}" "PR env frontend redirect URIs match the console Route"
if printf '%s' "${pr_redirects}" | grep -q localhost; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: PR env frontend redirect URIs still include localhost (Keycloak would reject the BFF redirect_uri)'
else
  PASS=$((PASS + 1))
fi
pr_idp="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); idp=next(i for i in realm["identityProviders"] if i["alias"]=="github"); print(json.dumps({"enabled": idp["enabled"], "clientId": idp["config"]["clientId"]}))' "${WORKDIR}/pr.json")"
assert_eq '{"enabled": true, "clientId": "Iv1.example"}' "${pr_idp}" "PR env GitHub IdP enabled with OAuth client id"
pr_broker="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); idp=next(i for i in realm["identityProviders"] if i["alias"]=="github"); print(json.dumps({"storeToken": idp.get("storeToken"), "addReadTokenRoleOnCreate": idp.get("addReadTokenRoleOnCreate"), "githubJsonFormat": idp["config"].get("githubJsonFormat")}))' "${WORKDIR}/pr.json")"
assert_eq '{"storeToken": true, "addReadTokenRoleOnCreate": true, "githubJsonFormat": "true"}' \
  "${pr_broker}" "GitHub IdP stores a JSON GitHub token and grants broker read-token on first login"
pr_frontend_scopes="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); client=next(c for c in realm["clients"] if c["clientId"]=="hypershell-frontend"); print(",".join(client["defaultClientScopes"]))' "${WORKDIR}/pr.json")"
assert_eq 'openid,email,profile,roles' \
  "${pr_frontend_scopes}" "Frontend access tokens include client roles so broker.read-token can be presented"
pr_mappers="$(python3 -c 'import json,sys; realm=json.load(open(sys.argv[1])); print(",".join(sorted({m["identityProviderMapper"] for m in realm["identityProviderMappers"]})))' "${WORKDIR}/pr.json")"
assert_eq 'oidc-hardcoded-role-idp-mapper' \
  "${pr_mappers}" "GitHub IdP hardcoded-role mapper uses the Keycloak 26 provider id"

if grep -q 'value: "token-exchange,admin-fine-grained-authz:v1"' "${REALM_YAML}"; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
  echo 'FAIL: Keycloak Deployment must enable token-exchange and admin-fine-grained-authz:v1'
fi

printf 'render-realm-config tests: %d passed, %d failed\n' "$PASS" "$FAIL"
[[ "${FAIL}" -eq 0 ]]
