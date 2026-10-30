import base64
from datetime import datetime, timedelta, timezone
import gzip
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT/'.hypershell/execute-sdlc'))
import jira_auth


class JiraAuthTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = patch.dict(os.environ, {'ACLI_CONFIG_DIR': str(self.root)}, clear=True)
        self.env.start()
        self.addCleanup(self.env.stop)
        self.profile = dict(site='redhat.atlassian.net', cloud_id='test-cloud', account_id='test-account', auth_type='oauth_global')
        (self.root/'jira_config.yaml').write_text(json.dumps(dict(current_profile='test-cloud:test-account', profiles=[self.profile])))

    def token(self, **changes):
        token = dict(access_token='access-secret', refresh_token='refresh-secret', token_type='Bearer',
                     expiry=(datetime.now(timezone.utc)+timedelta(hours=1)).isoformat())
        return dict(token, **changes)

    def test_auto_prefers_explicit_api_credentials_and_preserves_override(self):
        self.assertEqual(jira_auth.selected_auth('auto')['mode'], 'acli')
        os.environ.update(JIRA_USER_EMAIL='user@example.test', JIRA_TOKEN='api-secret')
        choice = jira_auth.selected_auth('auto')
        self.assertEqual(choice, {'mode':'api-token','site_url':'https://redhat.atlassian.net'})
        os.environ['JIRA_BASE_URL'] = 'https://custom.atlassian.net/'
        choice = jira_auth.selected_auth('auto')
        self.assertEqual(choice['site_url'], 'https://custom.atlassian.net')
        snapshot = jira_auth.snapshot(choice, 'scope', self.root/'lock')
        self.assertEqual(snapshot['authorization'], 'Basic '+base64.b64encode(b'user@example.test:api-secret').decode())
        self.assertNotIn('authorization', choice)

    def test_partial_api_credentials_use_acli_unless_api_mode_is_explicit(self):
        for key, value in (('JIRA_USER_EMAIL', 'user@example.test'), ('JIRA_TOKEN', 'api-secret')):
            with self.subTest(key=key), patch.dict(os.environ, {key: value}):
                self.assertEqual(jira_auth.selected_auth('auto')['mode'], 'acli')
                with self.assertRaises(jira_auth.JiraAuthError):
                    jira_auth.selected_auth('api-token')
                self.assertEqual(jira_auth.selected_auth('acli')['mode'], 'acli')

    def test_acli_tenant_mismatch_is_rejected(self):
        os.environ['JIRA_BASE_URL'] = 'https://different.atlassian.net'
        with self.assertRaisesRegex(jira_auth.JiraAuthError, 'differs'):
            jira_auth.selected_auth('acli')

    def test_acli_uses_selected_profile_not_first_account(self):
        other = dict(self.profile, cloud_id='other-cloud', account_id='other-account', site='other.atlassian.net')
        (self.root/'jira_config.yaml').write_text(json.dumps(dict(current_profile='test-cloud:test-account', profiles=[other,self.profile])))
        self.assertEqual(jira_auth.selected_auth('acli')['cloud_id'], 'test-cloud')

    @patch.object(jira_auth, 'private_command')
    def test_keyring_formats_export_only_access_token(self, run):
        value=json.dumps(self.token()).encode()
        for data in (value, gzip.compress(value), b'go-keyring-base64:'+base64.b64encode(gzip.compress(value))):
            run.return_value=data
            snapshot=jira_auth.read_keyring(self.profile)
            self.assertEqual(snapshot['authorization'], 'Bearer access-secret')
            self.assertNotIn('refresh-secret', json.dumps(snapshot))
            self.assertEqual(set(snapshot), {'authorization','expires_at'})

    @patch.object(jira_auth, 'private_command')
    def test_expired_and_unknown_formats_fail_without_leaking_tokens(self, run):
        for data in (b'unknown-secret-format', json.dumps(self.token(expiry='invalid')).encode(),
                     json.dumps(self.token(expiry=(datetime.now(timezone.utc)-timedelta(seconds=1)).isoformat())).encode()):
            run.return_value=data
            with self.assertRaises(jira_auth.JiraAuthError) as error:
                jira_auth.read_keyring(self.profile)
            self.assertNotIn('access-secret', str(error.exception))
            self.assertNotIn('unknown-secret-format', str(error.exception))

    @patch.object(jira_auth, 'private_command')
    def test_scoped_acli_request_refreshes_before_snapshot(self, run):
        run.side_effect=[b'0',gzip.compress(json.dumps(self.token()).encode())]
        choice=jira_auth.selected_auth('acli')
        snapshot=jira_auth.snapshot(choice,'agent/scope/test',self.root/'lock')
        self.assertEqual(run.call_args_list[0].args[0][:4], ['acli','jira','workitem','search'])
        self.assertIn('labels = "agent/scope/test" AND labels = "agent/allowed"',run.call_args_list[0].args[0])
        self.assertEqual(snapshot['api_base_url'],'https://api.atlassian.com/ex/jira/test-cloud')
        self.assertNotIn('refresh_token',snapshot)

    @patch.object(jira_auth, 'private_command')
    def test_account_switch_blocks_worker_before_refresh(self, run):
        choice=jira_auth.selected_auth('acli')
        changed=dict(self.profile, account_id='new-account')
        (self.root/'jira_config.yaml').write_text(json.dumps(dict(current_profile='test-cloud:new-account',profiles=[changed])))
        with self.assertRaisesRegex(jira_auth.JiraAuthError,'account changed'):
            jira_auth.snapshot(choice,'scope',self.root/'lock')
        run.assert_not_called()

    @patch.object(jira_auth.subprocess, 'run')
    def test_failed_credential_command_never_echoes_output(self, run):
        run.return_value=subprocess.CompletedProcess(['secret-tool'],1,stdout=b'access-secret',stderr=b'refresh-secret')
        with self.assertRaises(jira_auth.JiraAuthError) as error:
            jira_auth.private_command(['secret-tool','lookup'])
        self.assertNotIn('access-secret',str(error.exception))
        self.assertNotIn('refresh-secret',str(error.exception))
