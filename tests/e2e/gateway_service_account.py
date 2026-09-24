#!/usr/bin/env python3
"""Exercise the public gateway service-account API without logging credentials.

State lives in a run-private directory. JWT inspection is a test assertion on a
response from the trusted token endpoint; the gateway still verifies signatures.
"""

import base64
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import ssl
import sys
import tempfile
import time
from urllib.error import HTTPError
from urllib.parse import quote, urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, Request, build_opener


class NoRedirects(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def request(url, method="GET", data=None, token=None, form=False):
    if urlsplit(url).scheme != "https":
        raise ValueError("service-account requests require HTTPS")
    context = ssl.create_default_context(cafile=os.environ.get("SSL_CERT_FILE") or None)
    if os.environ.get("E2E_INFRA_DRIVER") == "kind":
        # Match the existing Kind driver's self-signed development TLS policy.
        context = ssl._create_unverified_context()
    headers = {}
    if token:
        headers["Authorization"] = "Bearer " + token
    if data is not None:
        headers["Content-Type"] = "application/x-www-form-urlencoded" if form else "application/json"
        data = (urlencode(data) if form else json.dumps(data)).encode()
    opener = build_opener(NoRedirects(), HTTPSHandler(context=context))
    with opener.open(Request(url, data=data, headers=headers, method=method), timeout=120) as response:
        body = response.read()
        return response.status, json.loads(body) if body else None


def write_private(path, data):
    # mkstemp starts at 0600; replace atomically so concurrent CLI invocations
    # cannot read a partial token bundle, even during renewal.
    fd, temporary = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(data, stream)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def api_url():
    return (os.environ["API_HOST"].rstrip("/") + "/api/hypershell/v1/gateways/"
            + quote(os.environ["GW_ID"], safe="") + "/service_accounts")


def api_token():
    return os.environ["E2E_GATEWAY_API_TOKEN"]


def validate_credential(account):
    credential = account["credential"]
    issuer = os.environ["E2E_OIDC_ISSUER"].rstrip("/")
    expected = {
        "issuer": issuer,
        "token_endpoint": issuer + "/protocol/openid-connect/token",
        "audience": os.environ["GW_KC_CLIENT_ID"],
        "grant_type": "client_credentials",
        "client_id": account["client_id"],
    }
    if any(credential.get(key) != value for key, value in expected.items()):
        raise ValueError("unexpected gateway credential destination or client")
    if (account["gateway_id"] != os.environ["GW_ID"] or account["role"] != "openshell-admin"
            or account["status"] != "ready" or not account["subject"] or not credential["client_secret"]):
        raise ValueError("unexpected gateway service-account identity or role")
    return credential


def mint(account):
    credential = validate_credential(account)
    _, response = request(credential["token_endpoint"], "POST", {
        "grant_type": "client_credentials", "client_id": credential["client_id"],
        "client_secret": credential["client_secret"],
    }, form=True)
    token = response["access_token"]
    payload = token.split(".")[1]
    claims = json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))
    audience = claims.get("aud")
    if isinstance(audience, str):
        audience = [audience]
    roles = claims.get("hypershell", {}).get("roles", [])
    if (audience != [credential["audience"]] or claims.get("iss") != credential["issuer"]
            or claims.get("sub") != account["subject"]
            or not isinstance(roles, list) or set(roles) != {"openshell-admin", "openshell-user"}
            or not time.time() + 30 < claims["exp"] <= time.time() + 330):
        raise ValueError("gateway token violates subject, audience, role, or lifetime limits")
    return {"access_token": token, "issuer": credential["issuer"],
            "client_id": credential["client_id"], "expires_at": claims["exp"]}


def create(path):
    if path.exists():
        raise ValueError("gateway credential state already exists")
    _, account = request(api_url(), "POST", {
        "name": "release-e2e", "role": "openshell-admin", "credential_type": "client_secret",
        "expires_at": (datetime.now(timezone.utc) + timedelta(hours=2)).isoformat(),
    }, token=api_token())
    # Save the ID before token validation so teardown can delete a partially
    # successful setup. Never retry a credential-creating POST blindly.
    write_private(path, account)
    account["token"] = mint(account)
    write_private(path, account)


def remove(path, verify_revocation=False):
    if not path.exists():
        return
    account = json.loads(path.read_text())
    url = api_url() + "/" + quote(account["id"], safe="")
    if verify_revocation:
        request(url + "/revoke", "POST", token=api_token())
        deadline = time.monotonic() + 90
        while True:
            try:
                mint(account)
            except HTTPError as error:
                # Only an OAuth client rejection proves revocation. Transport,
                # server, parsing, and other errors must fail this assertion.
                if error.code not in (400, 401):
                    raise
                body = json.load(error)
                if body.get("error") not in ("invalid_client", "unauthorized_client"):
                    raise ValueError("unexpected token error after revocation") from None
                break
            if time.monotonic() >= deadline:
                raise ValueError("revoked gateway credential still issues tokens")
            time.sleep(2)
    deadline = time.monotonic() + 90
    while True:
        try:
            status, _ = request(url, "DELETE", token=api_token())
            if status == 204:
                break
        except HTTPError as error:
            if error.code != 404:
                raise
            break
        if time.monotonic() >= deadline:
            raise ValueError("gateway credential deletion did not complete")
        time.sleep(2)
    path.unlink()


def run(path, argv):
    account = json.loads(path.read_text())
    token = account["token"]
    if token["expires_at"] <= time.time() + 60:
        token = account["token"] = mint(account)
        write_private(path, account)
    write_private(Path(os.environ["GW_CONFIG_DIR"]) / "oidc_token.json", token)
    environment = dict(os.environ)
    for name in ("E2E_GATEWAY_API_TOKEN", "E2E_OIDC_SA_CLIENT_SECRET", "E2E_OIDC_PASSWORD",
                 "E2E_KC_ADMIN_PASSWORD", "OPENSHELL_OIDC_CLIENT_SECRET"):
        environment.pop(name, None)
    return os.execvpe(argv[0], argv, environment)


def main():
    action, filename, *args = sys.argv[1:]
    path = Path(filename)
    if action == "create":
        create(path)
    elif action in ("delete", "revoke"):
        remove(path, verify_revocation=action == "revoke")
    elif action == "run":
        return run(path, args)
    elif action in ("client-id", "token"):
        account = json.loads(path.read_text())
        print(account["client_id"] if action == "client-id" else account["token"]["access_token"])
    else:
        raise ValueError("unknown gateway credential operation")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except HTTPError as error:
        sys.exit("Gateway service-account request failed (HTTP %d)." % error.code)
    except Exception:
        # Responses and exceptions can contain credentials; never print them.
        sys.exit("Gateway service-account operation failed; check API/provisioner readiness and credential policy.")
