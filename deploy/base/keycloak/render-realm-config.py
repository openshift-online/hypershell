#!/usr/bin/env python3
"""Render the HyperShell Keycloak realm for the current environment.

Keycloak --import-realm does not substitute ${VAR:default} placeholders
(upstream keycloak#20199). This script is the init-container renderer: it
loads the ConfigMap JSON, applies env-gated GitHub IdP / e2e-client values
(omitting the privileged hypershell-e2e identity unless it is enabled),
and (on OpenShift) replaces hypershell-frontend redirect URIs with the
web-console Route origin so they survive Keycloak pod restarts. start-dev
uses an ephemeral H2 store, so any admin-API mutation of redirect URIs is
lost on recycle; baking them into the imported JSON is the durable path.
"""

from __future__ import annotations

import json
import os
import sys
from typing import Any


def render_realm(realm: dict[str, Any], environ: dict[str, str] | None = None) -> dict[str, Any]:
    env = os.environ if environ is None else environ
    idp_enabled = env.get("PR_ENV_GITHUB_IDP_ENABLED", "false").strip().lower() == "true"
    e2e_enabled = env.get("HYPERSHELL_E2E_CLIENT_ENABLED", "false").strip().lower() == "true"
    console_host = env.get("HYPERSHELL_CONSOLE_HOST", "").strip()

    attributes = realm.setdefault("attributes", {})
    attributes["github.org.gate"] = env.get("PR_ENV_GITHUB_ORG", "openshift-online")
    attributes["github.username.allowlist"] = env.get("PR_ENV_GITHUB_ALLOWLIST", "")

    for idp in realm.get("identityProviders") or []:
        if idp.get("alias") != "github":
            continue
        idp["enabled"] = idp_enabled
        config = idp.setdefault("config", {})
        config["clientId"] = env.get("PR_ENV_GITHUB_CLIENT_ID", "")
        config["clientSecret"] = env.get("PR_ENV_GITHUB_CLIENT_SECRET", "")

    clients: list[dict[str, Any]] = []
    for client in realm.get("clients") or []:
        client_id = client.get("clientId")
        if client_id == "hypershell-e2e":
            # Omit the privileged CI client from Kind, local OpenShift, and
            # hub imports. A disabled-but-present client still has a UUID, and
            # the control plane must not grant token-exchange from it.
            if not e2e_enabled:
                continue
            client["enabled"] = True
            client["secret"] = env.get("HYPERSHELL_E2E_CLIENT_SECRET", "")
        if client_id == "hypershell-frontend" and console_host:
            # Exact console origin only. Wildcard redirect URIs are forbidden
            # (oidc-integration.spec.md / openshift-development.spec.md).
            client["redirectUris"] = [
                f"https://{console_host}/auth/callback",
                f"https://{console_host}",
            ]
        clients.append(client)
    realm["clients"] = clients

    e2e_user = "service-account-hypershell-e2e"
    users: list[dict[str, Any]] = []
    for user in realm.get("users") or []:
        is_e2e_sa = (
            user.get("username") == e2e_user
            or user.get("serviceAccountClientId") == "hypershell-e2e"
        )
        if is_e2e_sa and not e2e_enabled:
            continue
        if is_e2e_sa:
            user["enabled"] = True
        users.append(user)
    realm["users"] = users

    mappings = realm.get("clientScopeMappings") or {}
    if e2e_enabled:
        realm["clientScopeMappings"] = mappings
    else:
        mappings.pop("hypershell-e2e", None)
        realm["clientScopeMappings"] = mappings

    return realm


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    src = args[0] if len(args) > 0 else "/config/hypershell-realm.json"
    dst = args[1] if len(args) > 1 else "/rendered/hypershell-realm.json"
    with open(src, encoding="utf-8") as handle:
        realm = json.load(handle)
    if not isinstance(realm, dict):
        raise SystemExit(f"realm JSON root must be an object, got {type(realm).__name__}")
    render_realm(realm)
    with open(dst, "w", encoding="utf-8") as handle:
        json.dump(realm, handle, indent=2)
        handle.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
