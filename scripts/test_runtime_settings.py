# Verifies: SDLC-LOC-12
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from unittest.mock import patch

sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'.hypershell/execute-sdlc'))
import runtime_settings as settings
import funnel_dispatch
import local_scope


class SettingsTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.path=Path(self.tmp.name)/'settings.json'
        self.env=patch.dict(os.environ,{'FULLSEND_CONFIG_FILE':str(self.path),'FULLSEND_RUNTIME_SETTINGS':''})
        self.env.start();self.addCleanup(self.env.stop)
        os.environ.pop('FULLSEND_RUNTIME_SETTINGS',None)

    def test_atomic_private_save_preserves_other_values_and_concurrent_edits(self):
        self.assertEqual(settings.read(),{})
        with ThreadPoolExecutor(max_workers=2) as executor:
            results=list(executor.map(lambda kv:settings.save(*kv),[('model','test-model'),('concurrency','7')]))
        self.assertEqual(results,['test-model',7])
        self.assertEqual(settings.read(),{'model':'test-model','concurrency':7})
        self.assertEqual(stat.S_IMODE(self.path.stat().st_mode),0o600)
        self.assertEqual(list(self.path.parent.glob('.fullsend-*')),[])

    def test_invalid_input_and_malformed_file_never_overwrite_data(self):
        settings.save('concurrency',3);before=self.path.read_bytes()
        for key,value in [('concurrency','0'),('concurrency','oops'),('GH_TOKEN','secret'),('scope','wrong'),('api_url','https://user:password@example.test'),('model','$(touch X)\n')]:
            with self.assertRaises(ValueError):settings.save(key,value)
            self.assertEqual(self.path.read_bytes(),before)
        self.path.write_text('{invalid')
        with self.assertRaises(ValueError):settings.save('model','new')
        self.assertEqual(self.path.read_text(),'{invalid')

    def test_write_failure_preserves_original_file(self):
        settings.save('model','old');before=self.path.read_bytes()
        with patch.object(settings.os,'replace',side_effect=OSError('disk full')):
            with self.assertRaises(OSError):settings.save('model','new')
        self.assertEqual(before,self.path.read_bytes())
        self.assertEqual(list(self.path.parent.glob('.fullsend-*')),[])

    def test_empty_credentials_path_reuses_adc_discovery(self):
        with patch.dict(os.environ,{'CLOUDSDK_CONFIG':self.tmp.name}):
            credentials=funnel_dispatch.default_adc_path();credentials.write_text('{}')
            self.assertEqual(settings.save('GOOGLE_APPLICATION_CREDENTIALS',''),str(credentials))

    def test_symlink_is_not_followed_or_replaced(self):
        target=self.path.parent/'external';target.write_text('{"version":1,"values":{}}')
        self.path.symlink_to(target)
        with self.assertRaises(OSError):settings.save('model','new')
        self.assertTrue(self.path.is_symlink())
        self.assertEqual(json.loads(target.read_text())['values'],{})

    def test_launch_precedence_runtime_pins_and_restart(self):
        settings.save('concurrency',4);settings.save('repo','new/repository')
        args=funnel_dispatch.parse_args(['agent/scope/test','--dry-run','--concurrency','9'])
        self.assertEqual(args.concurrency,9)
        self.assertEqual(args.repo,'new/repository')
        os.environ['FULLSEND_RUNTIME_SETTINGS']=json.dumps({'repo':'old/repository','concurrency':2})
        active=funnel_dispatch.parse_args(['agent/scope/test','--dry-run','--concurrency','9'])
        self.assertEqual((active.repo,active.concurrency),('old/repository',2))
        self.assertEqual(settings.read()['repo'],'new/repository')
        os.environ.pop('FULLSEND_RUNTIME_SETTINGS')
        self.assertEqual(funnel_dispatch.parse_args(['agent/scope/test','--dry-run']).concurrency,4)

    def test_registration_checks_the_saved_repository(self):
        settings.save('repo','saved/repository')
        scope='agent/scope/saved'
        from subprocess import CompletedProcess
        with patch.object(sys,'argv',['scope-check',scope]), patch.object(local_scope.subprocess,'run',return_value=CompletedProcess([],0,json.dumps({'name':scope}))) as run:
            self.assertEqual(local_scope.main(),0)
            self.assertIn('repos/saved/repository/labels/',run.call_args[0][0][-1])

    def test_effective_paths_do_not_accumulate_repository_suffix(self):
        args=funnel_dispatch.parse_args(['agent/scope/test','--dry-run','--state-dir',self.tmp.name])
        os.environ['FULLSEND_RUNTIME_SETTINGS']=json.dumps(args.settings_inputs)
        again=funnel_dispatch.parse_args(['agent/scope/test','--dry-run'])
        self.assertEqual(args.state_dir,again.state_dir)

    def test_secret_values_never_enter_display_or_preferences(self):
        with patch.dict(os.environ,{'GH_TOKEN':'DO-NOT-DISPLAY','JIRA_TOKEN':'ALSO-SECRET'}):
            args=funnel_dispatch.parse_args(['agent/scope/test','--dry-run'])
            args.future_secret='FUTURE-SECRET'
            output=json.dumps(settings.view(args))
            self.assertNotIn('FUTURE-SECRET',output)
            self.assertNotIn('DO-NOT-DISPLAY',output);self.assertNotIn('ALSO-SECRET',output)
            self.assertIn('Exported (hidden)',output)

if __name__=='__main__':unittest.main()
