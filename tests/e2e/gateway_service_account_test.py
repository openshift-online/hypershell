"""Credential lifecycle and isolation checks for the opt-in machine e2e path."""
import base64
import copy
import io
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch
from urllib.error import HTTPError

import gateway_service_account as subject


class GatewayServiceAccountTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name) / "credential.json"
        self.environment = patch.dict(os.environ, {
            "API_HOST": "https://api.example", "GW_ID": "gateway-a", "GW_KC_CLIENT_ID": "client-a",
            "E2E_OIDC_ISSUER": "https://sso.example/realms/test", "E2E_GATEWAY_API_TOKEN": "management-token",
            "E2E_INFRA_DRIVER": "openshift", "GW_CONFIG_DIR": self.directory.name,
            "E2E_OIDC_SA_CLIENT_SECRET": "management-secret",
        })
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.account = {
            "id": "account-1", "client_id": "machine-a", "gateway_id": "gateway-a",
            "subject": "machine-subject", "role": "openshell-admin", "status": "ready",
            "credential": {
                "issuer": os.environ["E2E_OIDC_ISSUER"],
                "token_endpoint": os.environ["E2E_OIDC_ISSUER"] + "/protocol/openid-connect/token",
                "audience": "client-a", "grant_type": "client_credentials",
                "client_id": "machine-a", "client_secret": "gateway-secret",
            },
        }
        self.claims = {
            "sub": "machine-subject", "iss": os.environ["E2E_OIDC_ISSUER"], "aud": "client-a",
            "exp": int(time.time()) + 300,
            "hypershell": {"roles": ["openshell-admin", "openshell-user"]},
        }

    def token(self, claims=None):
        payload = base64.urlsafe_b64encode(json.dumps(claims or self.claims).encode()).decode().rstrip("=")
        return {"access_token": "header." + payload + ".signature"}

    def test_create_uses_owner_api_and_scoped_client_credentials(self):
        with patch.object(subject, "request", side_effect=[(201, self.account), (200, self.token())]) as request:
            subject.create(self.path)
        create, mint = request.call_args_list
        self.assertEqual(create.args[:2], ("https://api.example/api/hypershell/v1/gateways/gateway-a/service_accounts", "POST"))
        self.assertEqual(create.kwargs["token"], "management-token")
        self.assertEqual(create.args[2]["role"], "openshell-admin")
        self.assertEqual(mint.args[2], {"grant_type": "client_credentials", "client_id": "machine-a", "client_secret": "gateway-secret"})
        self.assertNotIn("token", mint.kwargs)
        self.assertEqual(stat.S_IMODE(self.path.stat().st_mode), 0o600)
        self.assertNotIn("management-secret", self.path.read_text())
        self.assertNotIn("management-token", self.path.read_text())

    def test_untrusted_credential_destination_never_receives_secret(self):
        for key, value in [("token_endpoint", "https://attacker.example/token"), ("issuer", "https://attacker.example"),
                           ("audience", "client-b"), ("client_id", "machine-b"), ("grant_type", "password")]:
            with self.subTest(key=key):
                account = copy.deepcopy(self.account)
                account["credential"][key] = value
                with patch.object(subject, "request") as request, self.assertRaises(ValueError):
                    subject.mint(account)
                request.assert_not_called()

    def test_rejects_cross_gateway_and_overprivileged_tokens(self):
        for change in [{"aud": "client-b"}, {"aud": ["client-a", "client-b"]}, {"aud": "hypershell-frontend"},
                       {"sub": "another-machine"}, {"iss": "https://another-issuer"},
                       {"hypershell": {"roles": ["openshell-admin"]}},
                       {"hypershell": {"roles": ["openshell-admin", "openshell-user", "extra-role"]}},
                       {"exp": int(time.time()) - 1}, {"exp": int(time.time()) + 3600}]:
            with self.subTest(change=change):
                claims = {**self.claims, **change}
                with patch.object(subject, "request", return_value=(200, self.token(claims))), self.assertRaises(ValueError):
                    subject.mint(self.account)

    def test_failed_token_setup_retains_id_for_cleanup(self):
        with patch.object(subject, "request", return_value=(201, self.account)), \
                patch.object(subject, "mint", side_effect=ValueError("invalid token")), self.assertRaises(ValueError):
            subject.create(self.path)
        self.assertEqual(json.loads(self.path.read_text())["id"], "account-1")

    def test_renewal_updates_cli_token_without_exposing_client_secret(self):
        self.account["token"] = {"expires_at": time.time() - 1, "access_token": "old"}
        subject.write_private(self.path, self.account)
        with patch.object(subject, "request", return_value=(200, self.token())) as request, \
                patch.object(subject.os, "execvpe", return_value=0) as process:
            self.assertEqual(subject.run(self.path, ["openshell", "status"]), 0)
        request.assert_called_once()
        token_path = self.path.parent / "oidc_token.json"
        self.assertEqual(stat.S_IMODE(token_path.stat().st_mode), 0o600)
        self.assertNotIn("gateway-secret", token_path.read_text())
        self.assertNotIn("E2E_OIDC_SA_CLIENT_SECRET", process.call_args.args[2])
        self.assertNotIn("E2E_GATEWAY_API_TOKEN", process.call_args.args[2])
        self.assertEqual(process.call_args.args[1], ["openshell", "status"])

    def test_valid_token_reused_without_grant(self):
        self.account["token"] = {"expires_at": time.time() + 300, "access_token": "current"}
        subject.write_private(self.path, self.account)
        with patch.object(subject, "request") as request, patch.object(subject.os, "execvpe", return_value=7):
            self.assertEqual(subject.run(self.path, ["openshell", "status"]), 7)
        request.assert_not_called()

    def test_shell_wrapper_executes_cli_with_private_token_and_no_management_secret(self):
        self.account["token"] = {"expires_at": int(time.time()) + 300, "access_token": "current"}
        subject.write_private(self.path, self.account)
        cli = self.path.parent / "fake cli"
        cli.write_text('''#!/usr/bin/env python3
import json, os, sys
assert 'E2E_GATEWAY_API_TOKEN' not in os.environ
assert 'E2E_OIDC_SA_CLIENT_SECRET' not in os.environ
with open(os.path.join(os.environ['GW_CONFIG_DIR'], 'oidc_token.json')) as stream:
    assert json.load(stream)['access_token'] == 'current'
print(json.dumps(sys.argv[1:]))
''')
        cli.chmod(0o700)
        environment = dict(os.environ, SCRIPT_DIR=str(Path(__file__).parent),
                           TEST_SA_DIR=str(self.path.parent), OPENSHELL_BIN=str(cli))
        result = subprocess.run(['bash', '-c', '''set -euo pipefail
source "${SCRIPT_DIR}/gateway_service_account.sh"
GATEWAY_SA_DIR="$TEST_SA_DIR"
install_gateway_service_account_cli
exec "$OPENSHELL_BIN" -g "gateway with spaces" status
'''], env=environment, capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(result.stdout), ['-g', 'gateway with spaces', 'status'])

    def rejection(self, status=401, error="invalid_client"):
        result = HTTPError("https://sso.example", status, "sensitive body", {},
                           io.BytesIO(json.dumps({"error": error}).encode()))
        self.addCleanup(result.close)
        return result

    def test_revocation_checks_issuance_then_deletes(self):
        subject.write_private(self.path, self.account)
        with patch.object(subject, "request", side_effect=[(200, {}), self.rejection(), (204, None)]) as request:
            subject.remove(self.path, verify_revocation=True)
        self.assertTrue(request.call_args_list[0].args[0].endswith("/account-1/revoke"))
        self.assertEqual(request.call_args_list[-1].args[1], "DELETE")
        self.assertFalse(self.path.exists())

    def test_revocation_does_not_pass_on_server_or_unrelated_oauth_errors(self):
        for rejection in [self.rejection(500), self.rejection(400, "invalid_scope")]:
            with self.subTest(code=rejection.code):
                subject.write_private(self.path, self.account)
                with patch.object(subject, "request", side_effect=[(200, {}), rejection]), \
                        self.assertRaises((HTTPError, ValueError)):
                    subject.remove(self.path, verify_revocation=True)
                self.assertTrue(self.path.exists())

    def test_delete_waits_for_async_cleanup(self):
        subject.write_private(self.path, self.account)
        with patch.object(subject, "request", side_effect=[(202, {}), (204, None)]) as request, \
                patch.object(subject.time, "sleep"):
            subject.remove(self.path)
        self.assertEqual(request.call_count, 2)

    def test_no_redirects_and_no_plain_http(self):
        self.assertIsNone(subject.NoRedirects().redirect_request(None, None, 302, "", {}, "https://attacker"))
        with self.assertRaises(ValueError):
            subject.request("http://sso.example", "POST", {"secret": "not-sent"})


if __name__ == "__main__":
    unittest.main()
