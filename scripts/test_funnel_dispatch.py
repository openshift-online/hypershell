import fcntl
import hashlib
import sqlite3
import importlib.util
import json
import os
import pty
import select
import struct
import termios
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch, Mock

ROOT = Path(__file__).resolve().parents[1]
ENTRY = ROOT/'.hypershell/execute-sdlc/funnel-dispatch.sh'
sys.path.insert(0, str(ENTRY.parent))
spec = importlib.util.spec_from_file_location('funnel', ENTRY.with_name('funnel_dispatch.py'))
funnel = importlib.util.module_from_spec(spec)
spec.loader.exec_module(funnel)


def issue(number=1, **kwargs):
    return dict(number=number, html_url=f'https://github.com/test/repo/issues/{number}',
                labels=['agent/scope/test', 'agent/allowed'], state='open', title='test', body='', **kwargs)


def pr(number=10, body='Fixes #1'):
    return dict(number=number, html_url=f'https://github.com/test/repo/pull/{number}',
                body=body, state='open', head={'sha': 'abc'})


class SharedAgentTests(unittest.TestCase):
    def test_local_harness_keeps_shared_agents_and_hosted_config_unchanged(self):
        from local_harness import prepare
        import shutil
        import yaml
        source = ROOT/'.fullsend'
        original = (source/'harness/ingest.yaml').read_bytes()
        for agent in ('ingest', 'reconcile'):
            directory = Path(prepare(source, agent))
            try:
                config = yaml.safe_load((directory/'config.yaml').read_text())
                entries = [entry for entry in config['agents'] if entry['name'] == agent]
                self.assertEqual(len(entries), 1)
                harness = yaml.safe_load((directory/entries[0]['source']).read_text())
                self.assertEqual(harness['agent'], f'agents/{agent}.md')
                self.assertEqual(harness['trigger'], 'false')
                self.assertEqual((directory/harness['agent']).read_bytes(),
                                 (source/harness['agent']).read_bytes())
                self.assertNotIn('JIRA_API_TOKEN', harness['env']['sandbox'])
                self.assertEqual(harness['env']['sandbox']['JIRA_CREDENTIALS_FILE'], '/tmp/.jira-credentials.json')
            finally:
                shutil.rmtree(directory)
        self.assertEqual((source/'harness/ingest.yaml').read_bytes(), original)
        self.assertFalse((source/'agents/funnel-ingest.md').exists())

    def test_agent_instructions_do_not_depend_on_execution_environment(self):
        for path in (ROOT/'.fullsend/agents').glob('*.md'):
            text = path.read_text()
            for phrase in ('local dispatcher', 'FUNNEL_', '/tmp/funnel-',
                           'OAuth refresh stays on the host', 'Sandbox checkout and build tools'):
                self.assertNotIn(phrase, text, str(path))

    def test_renamed_lifecycle_jobs_retain_persisted_keys(self):
        self.assertEqual(funnel.make_job('scope', 'reconcile', issue())['key'], '1:funnel-reconcile')


class InferenceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root/'harness').mkdir()
        (self.root/'harness/test.yaml').write_text('CLAUDE_CODE_USE_VERTEX: "1"')
        self.adc = self.root/'application_default_credentials.json'
        self.adc.write_text('{}')
        env = patch.dict(os.environ, {'CLOUDSDK_CONFIG': str(self.root),
                         'ANTHROPIC_VERTEX_PROJECT_ID': 'test-project',
                         'ANTHROPIC_VERTEX_REGION': 'test-region'}, clear=True)
        env.start()
        self.addCleanup(env.stop)

    def test_standard_adc_and_region_fallback(self):
        funnel.prepare_inference(self.root)
        self.assertEqual(os.environ['GOOGLE_APPLICATION_CREDENTIALS'], str(self.adc))
        self.assertEqual(os.environ['CLOUD_ML_REGION'], 'test-region')

    def test_explicit_missing_file_never_falls_back(self):
        os.environ['GOOGLE_APPLICATION_CREDENTIALS'] = str(self.root/'missing')
        with self.assertRaisesRegex(funnel.DispatchError, 'readable credentials file'):
            funnel.prepare_inference(self.root)

    def test_missing_project_fails_before_launch(self):
        del os.environ['ANTHROPIC_VERTEX_PROJECT_ID']
        with self.assertRaisesRegex(funnel.DispatchError, 'ANTHROPIC_VERTEX_PROJECT_ID'):
            funnel.prepare_inference(self.root)

    def test_non_vertex_harness_needs_no_adc(self):
        (self.root/'harness/test.yaml').write_text('providers: [anthropic]')
        self.adc.unlink()
        funnel.prepare_inference(self.root)
        self.assertNotIn('GOOGLE_APPLICATION_CREDENTIALS', os.environ)


class StoreTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)/'state.db'
        self.store = funnel.Store(self.path)
        self.job = funnel.make_job('agent/scope/test', 'triage', issue())

    def test_backoff_is_per_item_persistent_exponential_and_capped(self):
        for now, delay in [(100, 60), (160, 120), (280, 200), (480, 200)]:
            run = self.store.start(self.job, 'run.log', now)
            self.store.finish(self.job['key'], run, 1, 60, 200, now=now)
            other = funnel.Store(self.path)
            self.assertFalse(other.eligible(self.job, now + delay - 1))
            self.assertTrue(other.eligible(self.job, now + delay))
            self.assertTrue(other.eligible(funnel.make_job('scope', 'triage', issue(2)), now))
            other.db.close()

    def test_success_deduplicates_restart_but_allows_changed_inputs(self):
        run = self.store.start(self.job, 'run.log', 100)
        self.store.finish(self.job['key'], run, 0, 60, 200, now=100)
        reopened = funnel.Store(self.path)
        self.assertFalse(reopened.eligible(self.job, 1000))
        self.assertTrue(reopened.eligible(dict(self.job, fingerprint='new-inputs'), 1000))
        reopened.db.close()

    def test_uncertain_outcome_requires_explicit_recovery(self):
        self.store.start(self.job, 'run.log', 100)
        self.assertEqual(self.store.recover(), [self.job['key']])
        self.assertFalse(self.store.eligible(self.job, 10000))
        self.assertFalse(self.store.eligible(funnel.make_job('scope', 'another-phase', issue()), 10000))
        self.store.retry_uncertain(self.job['key'])
        self.assertTrue(self.store.eligible(self.job, 10000))

    def test_stale_worker_cannot_overwrite_new_attempt(self):
        old = self.store.start(self.job, 'old.log', 100)
        self.store.start(self.job, 'new.log', 200)
        with self.assertRaises(funnel.DispatchError):
            self.store.finish(self.job['key'], old, 0, 60, 200)


class DiscoveryTests(unittest.TestCase):
    def setUp(self):
        self.github = funnel.GitHub('test/repo', 'agent/scope/test', '.fullsend')

    @patch.object(funnel, 'json_command')
    def test_pagination_includes_more_than_200_and_filters_prs_and_unauthorized(self, run):
        items = [issue(i) for i in range(1, 252)]
        items += [issue(252, pull_request={})]
        unauthorized = issue(253)
        unauthorized['labels'] = ['agent/scope/test']
        items.append(unauthorized)
        run.return_value = [items[:100], items[100:200], items[200:]]
        self.assertEqual(len(self.github.discover()), 251)
        args = run.call_args.args[0]
        self.assertIn('--paginate', args)
        self.assertIn('--slurp', args)
        self.assertIn('state=all', args)

    @patch.object(funnel, 'json_command')
    def test_hosted_lifecycle_agents_are_not_dispatched_as_phases(self, run):
        run.return_value = [{'agent': agent} for agent in
                            ('ingest', 'reconcile', 'code-implementation')]
        self.assertEqual(self.github.classify(issue()), ['code-implementation'])

    def test_pr_requires_exact_link_and_rejects_ambiguity(self):
        self.assertEqual(self.github.review_pr(issue(), [pr()])['number'], 10)
        for prs in ([pr(10), pr(11)], [pr(body='Fixes #11')], [pr(body='mentions #1')], []):
            with self.subTest(prs=prs), self.assertRaises(funnel.DispatchError):
                self.github.review_pr(issue(), prs)
        item = issue()
        item['body'] = 'Implementation: https://github.com/test/repo/pull/10'
        self.assertEqual(self.github.review_pr(item, [pr(body='no closing link')])['number'], 10)

    def test_pr_head_changes_fingerprint(self):
        before = funnel.make_job('scope', 'code-review', issue(), pr())
        changed = pr()
        changed['head']['sha'] = 'new-sha'
        self.assertNotEqual(before['fingerprint'], funnel.make_job('scope', 'code-review', issue(), changed)['fingerprint'])

    @patch.object(funnel, 'json_command', side_effect=funnel.DispatchError('CEL failed'))
    def test_classification_failure_is_not_silently_empty(self, _):
        with self.assertRaises(funnel.DispatchError):
            self.github.classify(issue())


class ImplementationCycleTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.store = funnel.Store(Path(self.tmp.name)/'state.db')
        self.addCleanup(self.store.db.close)
        args = funnel.parse_args(['agent/scope/test', '--dry-run', '--lifecycle', 'github-only', '--target-repo', self.tmp.name])
        self.coordinator = funnel.Coordinator(args, self.store, -1)
        self.github = self.coordinator.github

    def test_repeated_rejections_distinct_but_polling_and_restart_stable(self):
        for phase in ('code', 'spec'):
            agent = phase+'-implementation'
            item = issue()
            label = f'agent/review-{phase}-rejected'
            item['labels'] += ['agent/review-spec-approved', label]
            events = [dict(id=10, event='labeled', label=dict(name=label))]
            pull = pr()
            def pages(resource, **fields):
                return [pull] if resource == 'pulls' else events
            with patch.object(self.github, 'pages', side_effect=pages), patch.object(self.github, 'discover', return_value=[item]), patch.object(self.github, 'classify', return_value=[agent]), patch.object(self.github, 'current', return_value=item):
                self.coordinator.discover()
                first = self.coordinator.candidates[0]
                run = self.store.start(first, 'run.log', 100)
                self.store.finish(first['key'], run, 0, 60, 200, now=110)
                self.assertFalse(self.store.eligible(self.github.phase_job('agent/scope/test', agent, item), 200))
                # Unrelated issue activity must not defeat deduplication.
                events.append(dict(id=11, event='labeled', label=dict(name='unrelated')))
                self.assertEqual(first['fingerprint'], self.github.phase_job('agent/scope/test', agent, item)['fingerprint'])
                # Labels and PR head can be identical after a second review.
                events.append(dict(id=12, event='labeled', label=dict(name=label)))
                self.assertFalse(self.coordinator.authorized(first))
                self.coordinator.discover()
                second = self.coordinator.candidates[0]
                self.assertNotEqual(first['fingerprint'], second['fingerprint'])
                self.assertTrue(self.store.eligible(second, 200))
                self.assertTrue(self.coordinator.authorized(second))
                run = self.store.start(second, 'run.log', 200)
                self.store.finish(second['key'], run, 0, 60, 200, now=210)
                reopened = funnel.Store(Path(self.tmp.name)/'state.db')
                try:
                    self.assertFalse(reopened.eligible(self.github.phase_job('agent/scope/test', agent, item), 300))
                finally:
                    reopened.db.close()
                pull['head']['sha'] = 'new-commit'
                self.assertFalse(self.coordinator.authorized(second))
                self.assertTrue(self.store.eligible(self.github.phase_job('agent/scope/test', agent, item), 300))

    def test_initial_implementation_allows_no_pr_but_rejection_requires_evidence(self):
        item = issue()
        with patch.object(self.github, 'pages', return_value=[]):
            self.assertEqual(self.github.phase_job('scope', 'spec-implementation', item)['pr_number'], '')
            item['labels'].append('agent/review-spec-rejected')
            with self.assertRaises(funnel.DispatchError):
                self.github.phase_job('scope', 'spec-implementation', item)
        with patch.object(self.github, 'pages', return_value=[]):
            with self.assertRaises(funnel.DispatchError):
                self.github.phase_job('scope', 'spec-implementation', item, [pr()])
        with self.assertRaises(funnel.DispatchError):
            self.github.phase_job('scope', 'spec-implementation', item, [pr(), pr(11)])


class LocalLaunchTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root/'.hypershell').mkdir()
        (self.root/'.hypershell/execute-sdlc').symlink_to(ENTRY.parent, target_is_directory=True)
        self.fake = self.root/'capture'
        self.fake.write_text('#!/usr/bin/env python3\nimport json,os,sys\nprint(json.dumps(dict(args=sys.argv[1:],model=os.environ.get("FULLSEND_MODEL"),concurrency=os.environ.get("CONCURRENCY"),project=os.environ.get("ANTHROPIC_VERTEX_PROJECT_ID"))))\n')
        self.fake.chmod(0o755)
        self.env = dict(os.environ, FUNNEL_DISPATCH_BINARY=str(self.fake))
        tools = self.root/'bin'
        tools.mkdir()
        gh = tools/'gh'
        gh.write_text('#!/usr/bin/env python3\nimport json,os,sys,urllib.parse\nmode=os.environ.get("TEST_SCOPE_LOOKUP", "ok")\nif mode == "error": sys.exit(1)\nprint(json.dumps({"name": "wrong-label" if mode == "wrong" else urllib.parse.unquote(sys.argv[-1].split("/labels/")[-1])}))\n')
        gh.chmod(0o755)
        self.env['PATH'] = str(tools)+os.pathsep+self.env['PATH']

        for key in ('SCOPE','MODEL','ENV_FILE','CONCURRENCY','FULLSEND_SCOPE','FULLSEND_MODEL','FULLSEND_LIFECYCLE','FULLSEND_LAUNCH_SCOPE','FULLSEND_LAUNCH_MODEL','FULLSEND_LAUNCH_ENV_FILE','FULLSEND_LAUNCH_CONCURRENCY'):
            self.env.pop(key, None)

    def invoke(self, target='fullsend', *values):
        return subprocess.run(['make','--no-print-directory','-s','-f',str(ROOT/'Makefile'),target,*values],cwd=self.root,env=self.env,text=True,capture_output=True,timeout=15)

    def test_local_config_make_overrides_and_action_modes(self):
        (self.root/'.hypershell/fullsend.env').write_text('FULLSEND_SCOPE=agent/scope/local\nFULLSEND_MODEL=file-model\nCONCURRENCY=2\n')
        self.env['CONCURRENCY']='7'
        for target, flag in [('fullsend','--watch'),('fullsend-status','--status')]:
            result=self.invoke(target)
            self.assertEqual(result.returncode,0,result.stderr)
            value=json.loads(result.stdout)
            self.assertEqual(value['args'],['agent/scope/local',flag])
            self.assertEqual(value['model'],'file-model')
            self.assertEqual(value['concurrency'],'2')
        result=self.invoke('fullsend','SCOPE=agent/scope/override','MODEL=chosen-model','CONCURRENCY=10')
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(json.loads(result.stdout)['args'],['agent/scope/override','--model','chosen-model','--concurrency','10','--watch'])

    def test_external_file_with_spaces_and_literal_arguments(self):
        path=self.root/'existing inference.env'
        path.write_text('FULLSEND_SCOPE=agent/scope/external\nANTHROPIC_VERTEX_PROJECT_ID=existing-project\n')
        result=self.invoke('fullsend','ENV_FILE='+str(path),"MODEL=model; touch SHOULD_NOT_EXIST")
        self.assertEqual(result.returncode,0,result.stderr)
        value=json.loads(result.stdout)
        self.assertIn('model; touch SHOULD_NOT_EXIST',value['args'])
        self.assertEqual(value['project'],'existing-project')
        self.assertFalse((self.root/'SHOULD_NOT_EXIST').exists())

    def test_missing_scope_and_explicit_file_fail_before_launch(self):
        result=self.invoke()
        self.assertNotEqual(result.returncode,0)
        self.assertIn('set FULLSEND_SCOPE',result.stderr)
        result=self.invoke('fullsend','ENV_FILE=missing.env','SCOPE=agent/scope/test')
        self.assertNotEqual(result.returncode,0)
        self.assertIn('ENV_FILE does not exist',result.stderr)

    def test_exported_environment_and_help_without_config(self):
        self.env.update(FULLSEND_SCOPE='agent/scope/shell',FULLSEND_MODEL='shell-model')
        result=self.invoke()
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(json.loads(result.stdout)['args'],['agent/scope/shell','--watch'])
        self.env.pop('FULLSEND_SCOPE')
        result=self.invoke('fullsend-help')
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertIn('make fullsend-status',result.stdout)
        self.assertNotIn('usage:',result.stdout)
        self.assertNotIn('make fullsend-plan',result.stdout)

    def test_unregistered_or_unverifiable_scope_blocks_start_but_not_status(self):
        self.env['FULLSEND_SCOPE'] = 'agent/scope/test'
        for mode in ('error', 'wrong'):
            self.env['TEST_SCOPE_LOOKUP'] = mode
            result = self.invoke()
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('cannot verify registered scope', result.stderr)
            self.assertEqual(result.stdout, '')
            self.assertEqual(self.invoke('fullsend-status').returncode, 0)

    def test_doctor_reports_setup_without_launching_or_leaking_tokens(self):
        spec=importlib.util.spec_from_file_location('local_doctor',ENTRY.with_name('local_doctor.py'))
        doctor=importlib.util.module_from_spec(spec)
        spec.loader.exec_module(doctor)
        import contextlib
        import io
        output=io.StringIO()
        with patch.object(doctor.shutil,'which',return_value='/fake/tool'), patch.object(doctor,'prepare_inference'), patch.object(doctor,'selected_auth',return_value=dict(mode='acli',site_url='https://example.atlassian.net')), patch.object(doctor,'read_keyring',return_value=dict(authorization='SECRET_NEVER_PRINT')), patch.dict(os.environ,{},clear=True), contextlib.redirect_stdout(output):
            self.assertEqual(doctor.main(),0)
        self.assertNotIn('SECRET_NEVER_PRINT',output.getvalue())
        output=io.StringIO()
        with patch.object(doctor.shutil,'which',return_value=None), patch.object(doctor,'prepare_inference',side_effect=doctor.DispatchError('missing ADC')), patch.object(doctor,'selected_auth',side_effect=doctor.JiraAuthError('missing Jira profile')), contextlib.redirect_stdout(output):
            self.assertEqual(doctor.main(),1)
        self.assertIn('missing ADC',output.getvalue())
        self.assertIn('missing Jira profile',output.getvalue())


class AuthTests(unittest.TestCase):
    @patch.object(funnel, 'command', side_effect=[funnel.DispatchError('cached OIDC token has expired'), '{}'])
    @patch.object(funnel.sys.stdin, 'isatty', return_value=False)
    @patch.object(funnel.subprocess, 'Popen')
    def test_headless_waits_and_recovers_without_login(self, launch, _, probe):
        auth = funnel.GatewayAuth(30)
        self.assertFalse(auth.ready(100))
        self.assertFalse(auth.ready(129))
        self.assertTrue(auth.ready(130))
        self.assertEqual(probe.call_count, 2)
        launch.assert_not_called()


    @patch.object(funnel, 'command', side_effect=[funnel.DispatchError('Unauthenticated'), funnel.DispatchError('Unauthenticated'), '{}'])
    @patch.object(funnel.sys.stdin, 'isatty', return_value=True)
    @patch.object(funnel.sys.stderr, 'isatty', return_value=True)
    @patch.object(funnel.subprocess, 'Popen')
    def test_failed_interactive_login_keeps_retrying_and_rechecks(self, launch, _a, _b, probe):
        login = Mock()
        login.poll.side_effect = [None, 1, 0]
        launch.return_value = login
        auth = funnel.GatewayAuth(30)
        self.assertFalse(auth.ready(100))
        self.assertFalse(auth.ready(101))
        self.assertFalse(auth.ready(102))
        self.assertFalse(auth.ready(132))
        self.assertTrue(auth.ready(162))
        self.assertEqual(launch.call_count, 2)
        self.assertEqual(probe.call_count, 3)

    @patch.object(funnel, 'command', side_effect=[funnel.DispatchError('connection refused'), '{}'])
    @patch.object(funnel.subprocess, 'Popen')
    def test_network_failure_retries_without_browser(self, launch, _):
        auth = funnel.GatewayAuth(1)
        self.assertFalse(auth.ready(100))
        self.assertTrue(auth.ready(101))
        launch.assert_not_called()


class ReconcileScheduleTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.store=funnel.Store(Path(self.tmp.name)/'state.db')
        self.addCleanup(self.store.db.close)
        args=funnel.parse_args(['agent/scope/test','--dry-run','--target-repo',self.tmp.name])
        self.coordinator=funnel.Coordinator(args,self.store,-1)

    def job(self, now, item=None, prs=None):
        with patch.object(funnel.time,'time',return_value=now):
            return self.coordinator.reconcile_job(item or issue(), prs or [])

    def complete(self, job, started=1000, finished=1100):
        run=self.store.start(job,'/tmp/sync.log',started)
        self.store.finish(job['key'],run,0,60,3600,now=finished)

    def test_unchanged_sync_does_not_repeat_at_clock_boundary(self):
        first=self.job(1000)
        self.complete(first)
        for now in (1801,3601,10000):
            self.assertEqual(self.job(now)['fingerprint'],first['fingerprint'])

    def test_periodic_drift_is_measured_from_success_not_start(self):
        first=self.job(1000)
        self.complete(first,finished=5000)
        self.assertEqual(self.job(5000+21600-1)['fingerprint'],first['fingerprint'])
        due=self.job(5000+21600)
        self.assertNotEqual(due['fingerprint'],first['fingerprint'])
        self.assertEqual(self.job(5000+21601)['fingerprint'],due['fingerprint'])
        self.complete(due,started=26601,finished=27000)
        self.assertEqual(self.job(27001)['fingerprint'],due['fingerprint'])

    def test_issue_and_pr_changes_sync_immediately(self):
        first=self.job(1000,prs=[pr()])
        self.complete(first)
        changed=issue()
        changed['title']='Changed title'
        self.assertNotEqual(self.job(1101,changed,[pr()])['fingerprint'],first['fingerprint'])
        changed_pr=pr()
        changed_pr['head']['sha']='changed-head'
        self.assertNotEqual(self.job(1101,prs=[changed_pr])['fingerprint'],first['fingerprint'])

    def test_worker_completion_triggers_sync_immediately(self):
        first=self.job(1000)
        self.complete(first)
        worker=funnel.make_job('agent/scope/test','code-implementation',issue())
        run=self.store.start(worker,'/tmp/work.log',1101)
        self.store.finish(worker['key'],run,1,60,3600,now=1102)
        self.assertNotEqual(self.job(1103)['fingerprint'],first['fingerprint'])

    def test_old_successful_clock_fingerprint_is_reused_on_upgrade(self):
        legacy=self.job(1000)
        legacy.pop('source_fingerprint')
        legacy['fingerprint']=funnel.fingerprint([
            funnel.make_job('agent/scope/test','reconcile',issue())['fingerprint'],[],None,1])
        self.complete(legacy)
        self.assertEqual(self.job(4000)['fingerprint'],legacy['fingerprint'])


class AuthorizationTests(unittest.TestCase):
    def test_removed_authorization_and_stale_labels_cannot_launch(self):
        args=funnel.parse_args(['agent/scope/test','--lifecycle','github-only','--dry-run'])
        store=Mock()
        store.db.execute.return_value=[]
        coordinator=funnel.Coordinator(args, store, -1)
        job=funnel.make_job(args.scope,'issue-triage',issue())
        for labels in (['agent/scope/test'], ['agent/allowed'],
                       ['agent/scope/test','agent/allowed','agent/blocked']):
            changed=issue()
            changed['labels']=labels
            with patch.object(coordinator.github, 'current', return_value=changed):
                self.assertFalse(coordinator.authorized(job))

    def test_changed_review_head_requires_rediscovery(self):
        args=funnel.parse_args(['agent/scope/test','--lifecycle','github-only','--dry-run'])
        store=Mock()
        store.db.execute.return_value=[]
        coordinator=funnel.Coordinator(args, store, -1)
        job=funnel.make_job(args.scope,'code-review',issue(),pr())
        changed=pr()
        changed['head']['sha']='new-sha'
        with patch.object(coordinator.github, 'current', return_value=issue()), patch.object(coordinator.github, 'pages', return_value=[changed]):
            self.assertFalse(coordinator.authorized(job))



MOCK_CLI = r'''#!/usr/bin/python3
import gzip, json, os, sys, time
from datetime import datetime, timedelta, timezone
from pathlib import Path
root=Path(os.environ['MOCK_STATE'])
name=Path(sys.argv[0]).name
args=sys.argv[1:]
def record(event):
    with (root/'calls').open('a') as f:
        f.write(json.dumps(dict(cmd=name,args=args,event=event,item=os.environ.get('ISSUE_NUMBER'),time=time.time(),gateway=os.environ.get('OPENSHELL_GATEWAY'),endpoint=os.environ.get('OPENSHELL_GATEWAY_ENDPOINT')))+'\n')
record('start')
if name=='acli':
    print('0')
elif name=='secret-tool':
    token=dict(access_token='test-access-token',refresh_token='NEVER-COPY-REFRESH',token_type='Bearer',expiry=(datetime.now(timezone.utc)+timedelta(hours=1)).isoformat())
    sys.stdout.buffer.write(gzip.compress(json.dumps(token).encode()))
elif name=='gh':
    def item(i):
        labels=['agent/allowed','agent/scope/test']
        if os.environ.get('MOCK_JIRA'):
            labels.append('agent/execution-record')
        if (root/('triaged-'+str(i))).exists():
            labels.append('agent/workable')
        return dict(number=i,html_url=f'https://github.com/test/repo/issues/{i}',labels=labels,state='closed' if os.environ.get('MOCK_JIRA') and i==2 else 'open',body='',title='test')
    if args[-1].split('/')[-1].isdigit():
        print(json.dumps(item(int(args[-1].split('/')[-1]))))
    elif 'issues' in ' '.join(args):
        time.sleep(float(os.environ.get('MOCK_DISCOVERY_DELAY','0')))
        p=root/'discovered'
        count=int(p.read_text())+1 if p.exists() else 1
        p.write_text(str(count))
        n=3 if os.environ.get('MOCK_NEW_ITEM') and count>1 else 2
        print(json.dumps([[item(i) for i in range(1,n+1)]]))
    else:
        print('[]')
elif name=='fullsend':
    if args[0]=='dispatch':
        event=json.load(sys.stdin)
        print('[]' if os.environ.get('MOCK_GATEWAY_TEST') or 'agent/workable' in event['state']['labels'] else '[{"agent":"issue-triage"}]')
    else:
        checkout=Path(args[args.index('--target-repo')+1])
        if checkout != root:  # Legacy Python workers still use the original path.
            assert (checkout/'.git').is_dir(), 'worker needs independent Git checkout'
        delays=json.loads(os.environ.get('MOCK_DELAYS','{}'))
        time.sleep(delays.get(os.environ.get('ISSUE_NUMBER'),0.05))
        if os.environ.get('MOCK_JIRA') and args[1]=='issue-triage':
            (root/('triaged-'+os.environ['ISSUE_NUMBER'])).touch()
        if args[1] in ('ingest','reconcile'):
            credentials=json.loads(Path(os.environ['FUNNEL_JIRA_CREDENTIALS_FILE']).read_text())
            assert credentials['version']==1 and credentials['authorization']
            assert 'refresh_token' not in credentials
            assert 'NEVER-COPY-REFRESH' not in json.dumps(credentials)
            if os.environ.get('MOCK_EXPECTED_JIRA_URL'):
                assert credentials['site_url']==os.environ['MOCK_EXPECTED_JIRA_URL']
            if os.environ.get('MOCK_EXPECTED_JIRA_AUTH'):
                assert credentials['mode']==os.environ['MOCK_EXPECTED_JIRA_AUTH']
        record('end')
        sys.exit(int(os.environ.get('MOCK_RECONCILE_EXIT','0') if args[1]=='reconcile' else os.environ.get('MOCK_EXIT','0')))
elif name=='hsctl':
    gateway=dict(id='gateway-test',name='test-gateway',phase='running',
                 route_address='grpcs://gateway.example.test:443',
                 oidc=dict(issuer='https://issuer.example.test',client_id='public-client',audience='test'))
    if args[:2]==['list','gateways'] and os.environ.get('MOCK_API_EXPIRED')=='1' and not (root/'api-signed-in').exists():
        print('session expired',file=sys.stderr)
        sys.exit(1)
    if args[0]=='login':
        (root/'api-signed-in').touch()
        print('MOCK_API_SIGNED_IN')
    elif args[0]=='config':
        print('API URL: https://api.hyp0.infra.hypershell.app')
    elif args[:2]==['list','managedClusters']:
        print(json.dumps(dict(items=[dict(id='cluster-test',name='test-cluster')])))
    elif args[:2]==['create','gateway']:
        gateway['name']=args[args.index('--name')+1]
        print(json.dumps(gateway))
    elif args[0]=='get':
        print(json.dumps(gateway))
    else:
        print('{}')
elif name=='openshell':
    if os.environ.get('MOCK_GATEWAY_TEST'):
        if args[0]=='whoami' and not (root/'signed-in').exists():
            print('token expired',file=sys.stderr)
            sys.exit(1)
        if args[:2]==['gateway','list']:
            print('[]')
        elif args[:2] in (['gateway','add'],['gateway','login']):
            (root/'signed-in').touch()
            print('MOCK_GATEWAY_SIGNED_IN',flush=True)
        else:
            print('{}')
    else:
        print('{}')
'''


class ProcessTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        subprocess.run(['git', 'init', '-q', str(self.root)], check=True)
        subprocess.run(['git', '-C', str(self.root), '-c', 'user.name=Test',
                        '-c', 'user.email=test@example.com', '-c', 'core.hooksPath=/dev/null', 'commit', '--allow-empty', '-qm', 'initial'], check=True)
        (self.root/'.gitignore').write_text('*\n')
        harnesses = self.root/'.fullsend/harness'
        harnesses.mkdir(parents=True)
        for agent in ('ingest', 'reconcile'):
            (harnesses/f'local-{agent}.yaml').write_text(f'agent: agents/{agent}.md\ntrigger: "false"\n')

        bindir = self.root/'bin'
        bindir.mkdir()
        for name in ('gh', 'fullsend', 'openshell', 'acli', 'secret-tool', 'hsctl'):
            path = bindir/name
            path.write_text(MOCK_CLI)
            path.chmod(0o755)
        self.env = dict(os.environ, PATH=str(bindir)+':/usr/bin:/bin', GH_TOKEN='test', MOCK_STATE=str(self.root))
        self.cmd = ['bash', str(ENTRY), 'agent/scope/test', '--repo', 'test/repo', '--state-dir', str(self.root/'state'),
                    '--concurrency', '2', '--poll-interval', '1', '--lifecycle', 'github-only']
        self.processes = []
        self.addCleanup(self.cleanup)

    def test_worker_keeps_ownership_until_result_is_saved(self):
        proc = self.start(MOCK_DELAYS='{"1":2,"2":2}')
        self.wait_for(lambda: len(self.runs()) == 2)
        rows = self.rows()
        self.assertEqual(len(rows), 2)
        dbpath = next((self.root/'state').glob('*/state.sqlite3'))
        connection = sqlite3.connect(dbpath)
        try:
            connection.execute('BEGIN IMMEDIATE')
            self.wait_for(lambda: len(self.calls('fullsend', 'end')) == 2)
            time.sleep(.2)
            for row in rows:
                digest = hashlib.sha256(json.dumps(row['run_id']).encode()).hexdigest()
                with (dbpath.parent/'workers'/(digest+'.lock')).open('a') as lock:
                    with self.assertRaises(BlockingIOError):
                        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        finally:
            connection.rollback()
            connection.close()
        self.assertEqual(proc.wait(timeout=8), 0)
        self.assertTrue(all(row['status']=='succeeded' for row in self.rows()))
        self.assertEqual(len(self.runs()), 2)

    def test_worker_uses_private_checkout_and_cleans_it_up(self):
        self.assertEqual(self.start().wait(timeout=8), 0)
        runs = [c for c in self.calls('fullsend') if c['args'][0] == 'run']
        self.assertTrue(runs)
        for call in runs:
            args = call['args']
            checkout = Path(args[args.index('--target-repo')+1])
            self.assertNotEqual(checkout, self.root)
            self.assertFalse(checkout.exists())

    def cleanup(self):
        for proc in self.processes:
            if proc.poll() is None:
                proc.terminate()
                proc.wait(timeout=5)
        deadline = time.time()+8
        while time.time()<deadline:
            rows = self.rows()
            if not any(row['status']=='running' for row in rows):
                break
            time.sleep(.1)

    def start(self, *args, **env):
        output = (self.root/f'output-{len(self.processes)}').open('w')
        proc = subprocess.Popen(self.cmd+list(args), env=dict(self.env, **env), cwd=self.root,
                                stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT)
        output.close()
        self.processes.append(proc)
        return proc

    def calls(self, cmd=None, event=None):
        path = self.root/'calls'
        calls = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        return [c for c in calls if (cmd is None or c['cmd']==cmd) and (event is None or c['event']==event)]

    def rows(self):
        files = list((self.root/'state').glob('*/state.sqlite3'))
        if not files:
            return []
        store = funnel.Store(files[0])
        rows = [dict(row) for row in store.db.execute('SELECT * FROM jobs')]
        store.db.close()
        return rows

    def wait_for(self, predicate, timeout=8):
        deadline = time.time()+timeout
        while time.time()<deadline:
            if predicate():
                return
            time.sleep(.05)
        self.fail('condition timed out: '+repr([p.read_text() for p in self.root.glob('output-*')]))

    def runs(self):
        return [c for c in self.calls('fullsend','start') if c['args'][0]=='run']

    def test_restart_deduplicates_completed_jobs_and_gateway_override(self):
        (self.root/'gateway.env').write_text('OPENSHELL_GATEWAY=wrong\nOPENSHELL_GATEWAY_ENDPOINT=https://wrong.example.test\n')
        self.assertEqual(self.start('--env-file', 'gateway.env', '--gateway', 'correct').wait(timeout=8), 0)
        self.assertEqual(self.start().wait(timeout=8), 0)
        self.assertEqual(sorted(c['item'] for c in self.runs()), ['1','2'])
        self.assertTrue(all(c['gateway']=='correct' and c['endpoint'] is None for c in self.runs()))

    def test_watch_discovers_new_work_before_slow_worker_finishes(self):
        proc = self.start('--watch', MOCK_DELAYS='{"1":4,"2":0.1,"3":0.1}', MOCK_NEW_ITEM='1')
        self.wait_for(lambda: any(c['item']=='3' for c in self.runs()))
        self.assertFalse(any(c['item']=='1' for c in self.calls('fullsend','end')))
        proc.terminate()
        proc.wait(timeout=5)

    def test_second_dispatcher_cannot_duplicate_surviving_workers(self):
        proc = self.start(MOCK_DELAYS='{"1":2,"2":2}')
        self.wait_for(lambda: len(self.runs())==2)
        proc.kill()
        proc.wait(timeout=5)
        second = self.start()
        time.sleep(.3)
        self.assertEqual(len(self.runs()), 2)
        self.wait_for(lambda: '2/2 running' in (self.root/'output-1').read_text())
        second.terminate()
        second.wait(timeout=5)
        self.wait_for(lambda: all(r['status']=='succeeded' for r in self.rows()))
        self.assertEqual(self.start().wait(timeout=8), 0)
        self.assertEqual(len(self.runs()), 2)

    def test_restart_uses_free_capacity_while_adopted_worker_runs(self):
        first = self.start('--watch', MOCK_DELAYS='{"1":5,"2":0.1}')
        self.wait_for(lambda: any(row['item']=='2' and row['status']=='succeeded' for row in self.rows()))
        first.terminate()
        first.wait(timeout=5)
        second = self.start('--watch', MOCK_NEW_ITEM='1')
        self.wait_for(lambda: any(call['item']=='3' for call in self.runs()))
        self.assertFalse(any(call['item']=='1' for call in self.calls('fullsend','end')))
        self.assertEqual(sum(call['item']=='1' for call in self.runs()), 1)
        second.terminate()
        second.wait(timeout=5)

    def test_live_second_dispatcher_monitors_without_duplicate_launches(self):
        first = self.start('--watch', MOCK_DELAYS='{"1":2,"2":2}')
        self.wait_for(lambda: len(self.runs())==2)
        second = self.start('--watch')
        self.wait_for(lambda: 'Monitoring existing work' in (self.root/'output-1').read_text())
        self.assertEqual(len(self.runs()), 2)
        first.terminate()
        first.wait(timeout=5)
        self.wait_for(lambda: '2/2 running' in (self.root/'output-1').read_text())
        second.terminate()
        second.wait(timeout=5)

    def test_legacy_worker_lock_shows_logs_and_resumes_after_release(self):
        directory = self.root/'state'/funnel.fingerprint('test/repo')[:16]
        store = funnel.Store(directory/'state.sqlite3')
        self.addCleanup(store.db.close)
        job = funnel.make_job('agent/scope/test', 'issue-triage', issue())
        run = store.start(job, '/tmp/legacy-worker.log', time.time())
        with (directory/'dispatcher.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            second = self.start()
            self.wait_for(lambda: 'issue-triage | running' in (self.root/'output-0').read_text())
            self.assertFalse(self.runs())
            second.terminate()
            second.wait(timeout=5)
            self.assertIn('/tmp/legacy-worker.log', (self.root/'output-0').read_text().splitlines())
            third = self.start()
            self.wait_for(lambda: 'Monitoring existing work' in (self.root/'output-1').read_text())
            store.finish(job['key'], run, 0, 60, 3600)
        self.assertEqual(third.wait(timeout=8), 0)
        self.assertEqual([call['item'] for call in self.runs()], ['2'])

    def test_auth_refresh_continues_when_all_worker_slots_are_busy(self):
        proc = self.start('--watch', '--auth-check-interval', '1', MOCK_DELAYS='{"1":5,"2":5}')
        self.wait_for(lambda: len(self.runs())==2)
        initial = len(self.calls('openshell', 'start'))
        self.wait_for(lambda: len(self.calls('openshell', 'start')) >= initial+2)
        self.assertFalse(self.calls('fullsend', 'end'))
        proc.terminate()
        proc.wait(timeout=5)

    def test_go_runner_adopts_a_python_worker(self):
        output = (self.root/'output-0').open('w')
        first = subprocess.Popen([sys.executable, str(ENTRY.with_name('funnel_dispatch.py'))]+self.cmd[2:],
                                 env=dict(self.env, MOCK_DELAYS='{"1":4,"2":4}'), cwd=self.root,
                                 stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT)
        output.close()
        self.processes.append(first)
        self.wait_for(lambda: len(self.runs())==2)
        first.terminate()
        first.wait(timeout=5)
        second = self.start()
        self.wait_for(lambda: '2/2 running' in (self.root/'output-1').read_text())
        self.assertEqual(second.wait(timeout=8), 0)
        self.assertEqual(len(self.runs()), 2)

    def test_auth_checks_continue_during_slow_discovery(self):
        proc = self.start('--watch', '--auth-check-interval', '1', MOCK_DISCOVERY_DELAY='4')
        self.wait_for(lambda: len(self.calls('openshell', 'start'))>=3)
        self.assertFalse(self.runs())
        proc.terminate()
        proc.wait(timeout=5)

    def test_tui_navigation_and_detach_keep_workers_alive(self):
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 24, 100, 0, 0))
        proc = subprocess.Popen(self.cmd+['--watch'], env=dict(self.env, TERM='xterm-256color', NO_COLOR='1', MOCK_DELAYS='{"1":4,"2":4}'),
                                cwd=self.root, stdin=slave, stdout=slave, stderr=slave)
        os.close(slave)
        self.processes.append(proc)
        self.wait_for(lambda: len(self.runs())==2)
        os.write(master, b'\x1b[B\r')
        time.sleep(.5)
        os.write(master, b'\x1b')
        time.sleep(.2)
        os.write(master, b'q')
        self.assertEqual(proc.wait(timeout=5), 130)
        output=b''
        while select.select([master], [], [], .1)[0]:
            try:
                chunk=os.read(master,65536)
                if not chunk:
                    break
                output+=chunk
            except OSError:
                break
        self.assertIn(b'FULLSEND',output)
        self.assertIn(b'h back',output)
        self.assertIn(b'Detached. Workers keep running.',output)
        self.assertTrue(any(row['status']=='running' for row in self.rows()))
        self.assertEqual(self.start().wait(timeout=8), 0)
        self.assertEqual(len(self.runs()), 2)

    def gateway_prompt_with_controlling_terminal(self, choice, expired=False):
        state=self.root/'.hypershell/.funnel-gateway.json'
        state.parent.mkdir()
        state.write_text(json.dumps(dict(version=1,id='gateway-test',name='test-gateway',
                                        api_url='https://api.hyp0.infra.hypershell.app')))
        master,slave=pty.openpty()
        self.addCleanup(os.close,master)
        fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,100,0,0))
        def foreground_terminal():
            os.setsid()
            fcntl.ioctl(0,termios.TIOCSCTTY,0)
        proc=subprocess.Popen(self.cmd+['--watch'],cwd=self.root,
                              env=dict(self.env,TERM='xterm-256color',MOCK_GATEWAY_TEST='1',MOCK_API_EXPIRED='1' if expired else '0'),
                              stdin=slave,stdout=slave,stderr=slave,preexec_fn=foreground_terminal)
        os.close(slave)
        self.processes.append(proc)
        pending=b''
        def read_until(needle):
            nonlocal pending
            deadline=time.time()+8
            marker=needle.encode()
            while time.time()<deadline:
                if marker in pending:
                    _,pending=pending.split(marker,1)
                    return
                if select.select([master],[],[],.1)[0]:
                    pending+=os.read(master,65536)
            self.fail('terminal did not reach '+needle+': '+repr(pending))
        read_until('FULLSEND')
        os.write(master,b's')
        read_until('Connect your gateway')
        if choice == 'cancel':
            os.write(master, b'\x1b')
            read_until('Sign-in cancelled')
            os.write(master, b'q')
            self.assertEqual(proc.wait(timeout=5), 130)
            self.assertFalse((self.root/'signed-in').exists())
            self.assertFalse(self.runs())
            return
        os.write(master,(b'\x1b[B' if choice == '2' else b'')+b'\r')
        if expired:
            read_until('hsctl authentication failed. Log in again?')
            os.write(master,b'y\n')
        if choice=='2':
            read_until('Select a cluster')
            os.write(master,b'1\n')
            read_until('New gateway name')
            os.write(master,b'test-gateway\n')
            read_until('Create this gateway?')
            os.write(master,b'y\n')
        read_until('Gateway access verified.')
        os.write(master,b'q')
        self.assertEqual(proc.wait(timeout=5),130)
        self.assertTrue((self.root/'signed-in').exists())
        creates=[call for call in self.calls('hsctl','start') if call['args'][:2]==['create','gateway']]
        self.assertEqual(len(creates),int(choice=='2'))
        self.assertFalse(self.runs())

    def test_embedded_signin_cancellation_returns_control(self):
        self.gateway_prompt_with_controlling_terminal('cancel')

    def test_gateway_reconnect_reads_choice_in_foreground(self):
        self.gateway_prompt_with_controlling_terminal('1')

    def test_gateway_create_reads_all_prompts_in_foreground(self):
        self.gateway_prompt_with_controlling_terminal('2')

    def test_gateway_reconnect_renews_expired_api_session(self):
        self.gateway_prompt_with_controlling_terminal('1', expired=True)
        self.assertTrue((self.root/'api-signed-in').exists())

    def test_manual_retry_command_is_picked_up_by_running_dispatcher(self):
        first=self.start('--watch',MOCK_EXIT='7')
        self.wait_for(lambda: len(self.rows())==2 and all(r['status']=='failed' for r in self.rows()))
        self.assertEqual(self.start('--retry','1:issue-triage').wait(timeout=5),0)
        self.wait_for(lambda: len(self.runs())==3)
        self.assertEqual(sum(c['item']=='1' for c in self.runs()),2)
        self.assertEqual(sum(c['item']=='2' for c in self.runs()),1)
        first.terminate()
        first.wait(timeout=5)

    def test_failure_backoff_survives_restart(self):
        self.assertEqual(self.start(MOCK_EXIT='7').wait(timeout=8), 1)
        self.assertEqual(self.start(MOCK_EXIT='7').wait(timeout=8), 1)
        self.assertEqual(len(self.runs()), 2)
        self.assertTrue(all(row['failures']==1 for row in self.rows()))

    def test_dry_run_does_not_login_or_run_agents(self):
        self.assertEqual(self.start('--dry-run').wait(timeout=8), 0)
        self.assertFalse(self.calls('openshell'))
        self.assertFalse(self.runs())

    def test_jira_lifecycle_orders_intake_reconcile_work_and_reconciles_closed_records(self):
        self.env.pop('JIRA_BASE_URL', None)
        proc = self.start('--lifecycle', 'jira', MOCK_JIRA='1', MOCK_EXPECTED_JIRA_URL='https://redhat.atlassian.net',
                          JIRA_USER_EMAIL='test@example.test', JIRA_TOKEN='not-a-real-token')
        self.assertEqual(proc.wait(timeout=12), 0)
        runs=self.runs()
        self.assertEqual(runs[0]['args'][1], 'ingest')
        for number in ('1','2'):
            self.assertTrue(any(c['item']==number and c['args'][1]=='reconcile' for c in runs))
        phases=[c for c in runs if c['args'][1]=='issue-triage']
        self.assertEqual([c['item'] for c in phases], ['1'])
        reconciles=[c for c in runs if c['item']=='1' and c['args'][1]=='reconcile']
        self.assertEqual(len(reconciles), 2)
        self.assertLess(reconciles[0]['time'], phases[0]['time'])
        self.assertGreater(reconciles[1]['time'], phases[0]['time'])
        self.assertNotIn('not-a-real-token', repr(self.rows()))

    def test_failed_reconcile_does_not_dispatch_phase(self):
        proc=self.start('--lifecycle', 'jira', MOCK_JIRA='1', MOCK_RECONCILE_EXIT='1',
                        JIRA_BASE_URL='https://test.atlassian.net', MOCK_EXPECTED_JIRA_URL='https://test.atlassian.net',
                        JIRA_USER_EMAIL='test@example.test', JIRA_TOKEN='fake')
        self.assertEqual(proc.wait(timeout=12), 1)
        self.assertEqual(next(row['status'] for row in self.rows() if row['agent']=='ingest'), 'succeeded')
        self.assertFalse(any(c['args'][1]=='issue-triage' for c in self.runs()))

    def test_missing_jira_credentials_fail_before_any_external_command(self):
        proc=self.start('--lifecycle', 'jira', '--jira-auth', 'api-token', JIRA_TOKEN='')
        self.assertEqual(proc.wait(timeout=8), 1)
        self.assertFalse(self.calls())

    def test_acli_session_reaches_workers_without_api_or_refresh_tokens(self):
        config=self.root/'acli'
        config.mkdir()
        (config/'jira_config.yaml').write_text(json.dumps(dict(current_profile='cloud:account',profiles=[
            dict(site='redhat.atlassian.net',cloud_id='cloud',account_id='account',auth_type='oauth_global')
        ])))
        for key in ('JIRA_BASE_URL','JIRA_USER_EMAIL','JIRA_TOKEN','JIRA_AUTH_MODE'):
            self.env.pop(key,None)
        proc=self.start('--lifecycle','jira',MOCK_JIRA='1',MOCK_EXPECTED_JIRA_AUTH='acli',
                        ACLI_CONFIG_DIR=str(config),XDG_STATE_HOME=str(self.root/'xdg-state'))
        self.assertEqual(proc.wait(timeout=12),0)
        self.assertTrue(self.calls('acli'))
        self.assertTrue(self.calls('secret-tool'))
        self.assertTrue(any(c['args'][1]=='reconcile' for c in self.runs()))
        self.assertNotIn('NEVER-COPY-REFRESH',repr(self.rows()))
        self.assertNotIn('test-access-token',repr(self.rows()))


class LifecycleValidationTests(unittest.TestCase):
    def test_partial_failures_and_missing_results_are_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp)/'output').mkdir()
            path=Path(tmp)/'output/result.json'
            for result, expected in (({'status':'success','errors':[],'records':[]},0),
                                     ({'status':'success','errors':['failed write'],'records':[]},1),
                                     ({'status':'failed','errors':[],'records':[]},1)):
                path.write_text(json.dumps(result))
                proc=subprocess.run(['bash',str(ROOT/'.fullsend/scripts/validate-lifecycle.sh')],cwd=tmp,capture_output=True)
                self.assertEqual(proc.returncode==0,expected==0)
