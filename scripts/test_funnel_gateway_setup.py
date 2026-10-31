import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'.hypershell/execute-sdlc'))
import gateway_setup as setup
import funnel_dispatch as funnel


def gateway(**changes):
    return dict(dict(id='gateway-1', name='fullsend-test', phase='Running',
                     route_address='grpcs://gw.example.test:443',
                     oidc=json.dumps(dict(issuer='https://keycloak.example.test/realms/test',
                                          client_id='human-gateway-client',audience='gateway-audience'))),**changes)


class GatewaySetupTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name)
        self.args=SimpleNamespace(gateway_state=self.root/'.hypershell/.funnel-gateway.json',
                                  gateway_login_timeout=120,api_url=None,gateway=None)
        env=patch.dict(os.environ,{},clear=True)
        env.start()
        self.addCleanup(env.stop)
        self.manager=setup.GatewayManager(self.args)
        self.manager.hsctl='hsctl'

    def test_api_check_detects_expired_session_despite_cached_identity(self):
        with patch.object(setup, 'capture', side_effect=['API URL: https://api.hyp0.infra.hypershell.app', setup.GatewaySetupError('session expired')]) as capture, patch('builtins.input', return_value='y'), patch.object(self.manager, 'api_login') as login:
            self.manager.ensure_api()
        login.assert_called_once()
        self.assertEqual(capture.call_args_list[-1].args[0], ['hsctl','list','gateways','--output','json'])

    def test_adapter_does_not_report_cancelled_or_failed_recovery_as_success(self):
        import dispatch_adapter
        manager=Mock()
        manager.recover.return_value=('wait',None)
        manager.last_recovery_error='session expired'
        with patch.object(sys,'argv',['adapter','gateway','agent/scope/test','--log-dir',str(self.root/'logs')]), patch.object(dispatch_adapter,'GatewayManager',return_value=manager), patch.object(sys,'stderr'), patch.object(dispatch_adapter,'RecoveryDiagnostics',side_effect=lambda stream,path: stream):
            with self.assertRaisesRegex(setup.GatewaySetupError,'session expired'):
                dispatch_adapter.main()

    def test_saved_state_reuses_gateway_and_contains_no_credentials(self):
        self.manager.save(gateway())
        os.environ['OPENSHELL_GATEWAY']='stale-global-gateway'
        again=setup.GatewayManager(self.args)
        self.assertEqual(os.environ['OPENSHELL_GATEWAY'],'fullsend-test')
        self.assertEqual(again.state['id'],'gateway-1')
        self.assertEqual(set(again.state),{'version','api_url','id','name'})
        self.assertEqual(self.args.gateway_state.stat().st_mode & 0o777,0o600)

    def test_explicit_gateway_wins_over_saved_selection(self):
        self.manager.save(gateway())
        os.environ.pop('OPENSHELL_GATEWAY')
        self.args.gateway='explicit'
        again=setup.GatewayManager(self.args)
        self.assertEqual(again.name,'explicit')
        self.assertNotIn('OPENSHELL_GATEWAY',os.environ)

    def test_invalid_state_does_not_crash_or_get_overwritten(self):
        self.args.gateway_state.parent.mkdir()
        self.args.gateway_state.write_text('broken-json')
        again=setup.GatewayManager(self.args)
        self.assertIsNone(again.state)
        self.assertIn('Cannot read',again.state_error)
        self.assertEqual(self.args.gateway_state.read_text(),'broken-json')

    def test_correct_human_oidc_client_used_and_service_account_rejected(self):
        argv=setup.connection_args(gateway())
        self.assertEqual(argv[argv.index('--oidc-client-id')+1],'human-gateway-client')
        self.assertEqual(argv[-1],'https://gw.example.test:443')
        oidc=json.loads(gateway()['oidc'])
        oidc['client_id']='hs-sa-service-client'
        with self.assertRaises(setup.GatewaySetupError):
            setup.connection_args(gateway(oidc=json.dumps(oidc)))

    def test_platform_selection_and_checksum_failure(self):
        for system,machine,expected in [('Linux','x86_64','linux-amd64'),('Linux','aarch64','linux-arm64'),('Darwin','arm64','darwin-arm64')]:
            self.assertEqual(setup.release_asset(system,machine),'hsctl-'+expected)
        binary=self.root/'hsctl-linux-amd64'
        binary.write_bytes(b'cli-binary')
        manifest=self.root/'SHA256SUMS'
        manifest.write_text(hashlib.sha256(binary.read_bytes()).hexdigest()+'  '+binary.name+'\n')
        setup.verify_checksum(binary,manifest)
        binary.write_bytes(b'corrupt')
        with self.assertRaises(setup.GatewaySetupError):
            setup.verify_checksum(binary,manifest)

    @patch.object(setup,'json_call')
    @patch('builtins.input',return_value='n')
    def test_missing_cli_install_can_be_declined_without_downloading(self, ask, api):
        self.manager.hsctl=None
        api.return_value={'tag_name':'v1','assets':[{'name':'hsctl-linux-amd64'},{'name':'SHA256SUMS'}]}
        with patch.object(setup.Path,'home',return_value=self.root), patch.object(setup,'release_asset',return_value='hsctl-linux-amd64'), patch.object(setup,'capture') as download:
            with self.assertRaisesRegex(setup.GatewaySetupError,'declined'):
                self.manager.ensure_hsctl()
            download.assert_not_called()
            self.assertFalse((self.root/'.local/bin/hsctl').exists())

    @patch.object(setup,'json_call')
    @patch('builtins.input',return_value='y')
    def test_missing_cli_download_is_verified_before_install(self, ask, api):
        self.manager.hsctl=None
        api.return_value={'tag_name':'v1','assets':[{'name':'hsctl-linux-amd64'},{'name':'SHA256SUMS'}]}
        def download(argv):
            root=Path(argv[argv.index('--dir')+1])
            binary=root/'hsctl-linux-amd64'
            binary.write_bytes(b'fake-cli')
            (root/'SHA256SUMS').write_text(hashlib.sha256(binary.read_bytes()).hexdigest()+'  '+binary.name+'\n')
            return ''
        with patch.object(setup.Path,'home',return_value=self.root), patch.object(setup,'release_asset',return_value='hsctl-linux-amd64'), patch.object(setup,'capture',side_effect=download):
            self.manager.ensure_hsctl()
        installed=self.root/'.local/bin/hsctl'
        self.assertEqual(installed.read_bytes(),b'fake-cli')
        self.assertEqual(installed.stat().st_mode & 0o777,0o755)
        self.assertEqual(self.manager.hsctl,str(installed))

    @patch.object(setup,'json_call')
    @patch('builtins.input',side_effect=['1','created-gateway','y'])
    def test_create_requires_confirmation_and_persists_before_connection(self, ask, api):
        created=gateway(name='created-gateway')
        api.side_effect=[{'items':[{'id':'cluster-1','name':'test-cluster'}]},created]
        self.assertEqual(self.manager.create(),created)
        self.assertEqual(json.loads(self.args.gateway_state.read_text())['id'],'gateway-1')
        self.assertEqual(api.call_args.args[0],['hsctl','create','gateway','--name','created-gateway','--cluster-id','cluster-1'])
        self.assertNotIn('--dedicated-route',api.call_args.args[0])

    @patch.object(setup,'json_call')
    @patch('builtins.input',side_effect=['1','pending-gateway','y'])
    def test_lost_create_response_recovers_by_name_without_second_create(self, ask, api):
        api.side_effect=[{'items':[{'id':'cluster-1','name':'cluster'}]},setup.GatewaySetupError('timeout')]
        with self.assertRaisesRegex(setup.GatewaySetupError,'outcome is unknown'):
            self.manager.create()
        self.assertEqual(self.manager.state['id'],'pending-create')
        api.side_effect=[{'items':[gateway(name='pending-gateway')]},gateway(name='pending-gateway'),[]]
        argv=self.manager.registration()
        self.assertEqual(self.manager.state['id'],'gateway-1')
        self.assertEqual(argv[0:3],['openshell','gateway','add'])
        self.assertEqual(sum('create' in call.args[0] for call in api.call_args_list),1)

    @patch.object(setup.GatewayManager,'interactive',return_value=True)
    @patch('builtins.input',return_value='r')
    @patch.object(setup.GatewayManager,'ensure_api')
    @patch.object(setup,'json_call',side_effect=setup.GatewaySetupError('API returned 404'))
    def test_missing_gateway_preserves_state_and_never_auto_creates(self, api, ensure, ask, interactive):
        self.manager.save(gateway())
        self.assertEqual(self.manager.recover()[0],'wait')
        self.assertEqual(self.manager.state['id'],'gateway-1')
        self.assertFalse(any('create' in call.args[0] for call in api.call_args_list))

    @patch.object(setup.GatewayManager,'interactive',return_value=True)
    @patch('builtins.input',return_value='l')
    @patch.object(setup.GatewayManager,'ensure_api')
    @patch.object(setup.GatewayManager,'existing',return_value=gateway())
    def test_old_login_shortcut_refreshes_api_metadata_instead_of_cached_login(self, existing, ensure, ask, interactive):
        action, selected=self.manager.recover()
        self.assertEqual(action,'connect')
        self.assertEqual(selected,gateway())
        ensure.assert_called_once()
        existing.assert_called_once()
        self.assertEqual(self.manager.state['id'],'gateway-1')

    @patch.object(setup.GatewayManager,'interactive',return_value=False)
    @patch('builtins.input')
    def test_headless_never_prompts_or_creates(self, ask, interactive):
        self.assertEqual(self.manager.recover(),('wait',None))
        ask.assert_not_called()

    @patch.object(setup.GatewayManager,'interactive',return_value=True)
    @patch('builtins.input')
    def test_running_workers_prevent_gateway_switch(self, ask, interactive):
        self.assertEqual(self.manager.recover(allow_replace=False),('wait',None))
        ask.assert_not_called()

    @patch.object(setup.GatewayManager,'interactive',return_value=True)
    @patch.object(setup.sys.stdin,'isatty',return_value=True)
    @patch.object(setup.select,'select',return_value=([],[],[]))
    @patch('builtins.input')
    def test_unanswered_recovery_prompt_returns_to_retries(self, ask, wait, tty, interactive):
        self.assertEqual(self.manager.recover(),('wait',None))
        ask.assert_not_called()

    @patch.object(funnel.subprocess,'Popen')
    @patch.object(funnel,'command',side_effect=funnel.DispatchError('expired token'))
    def test_auth_failure_offers_creation_then_connects_from_api(self, probe, spawn):
        manager=Mock()
        manager.recover.return_value=('connect',gateway())
        manager.registration.return_value=setup.connection_args(gateway())
        auth=funnel.GatewayAuth(30,manager)
        self.assertFalse(auth.ready(100))
        self.assertFalse(auth.ready(101))
        manager.recover.assert_called_once()
        spawn.assert_called_once_with(setup.connection_args(gateway()))

    def test_rejected_redirect_does_not_leave_browser_login_waiting_forever(self):
        manager=Mock()
        manager.recover.return_value=('wait',None)
        auth=funnel.GatewayAuth(30,manager,login_timeout=120)
        login=Mock()
        login.poll.return_value=None
        auth.login=login
        auth.login_started=100
        self.assertFalse(auth.ready(219))
        manager.recover.assert_not_called()
        self.assertFalse(auth.ready(220))
        login.terminate.assert_called_once()
        manager.recover.assert_called_once()

    @patch.object(setup,'json_call')
    def test_reconnect_uses_stable_alias_and_does_not_remove_existing_registration(self, api):
        self.manager.save(gateway())
        api.side_effect=[gateway(),[]]
        add=self.manager.registration()
        alias=add[add.index('--name')+1]
        self.assertTrue(alias.startswith('funnel-gateway-1-'))
        self.assertEqual(self.manager.state['local_name'],alias)
        api.side_effect=[gateway(),[{'name':alias,'endpoint':add[-1]}]]
        self.assertEqual(self.manager.registration(),['openshell','gateway','login',alias])
        self.assertFalse(any('remove' in call.args[0] for call in api.call_args_list))
