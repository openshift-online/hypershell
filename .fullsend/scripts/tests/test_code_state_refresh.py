import copy
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('refresh', Path(__file__).resolve().parents[1]/'refresh-code-state.py')
refresh = importlib.util.module_from_spec(spec)
spec.loader.exec_module(refresh)


class RefreshTests(unittest.TestCase):
    def setUp(self):
        self.labels = {1: {'agent/allowed', 'agent/scope/test', 'agent/review-code-approved'},
                       5: {'agent/review-code-approved'}}
        self.item = dict(state='open', body='', number=1)
        self.pr = dict(state='open', body='Fixes #1', head={'sha':'head'}, base={'sha':'base'},
                       mergeable=False, mergeable_state='dirty')
        self.state = dict(head_sha='head', base_sha='base', state='open', draft=False,
                          mergeable=False, mergeable_state='dirty', checks='green')
        self.writes = []
        self.timeline = []
        def api(path, *args):
            if path.endswith('/timeline'):
                return [self.timeline]
            if '/pulls/' in path:
                return copy.deepcopy(self.pr)
            number = int(path.split('/issues/')[1].split('/')[0])
            if path.endswith('/labels'):
                return [{'name': name} for name in self.labels[number]]
            return dict(self.item, labels=[{'name': name} for name in self.labels[number]])
        def run(*args, input=None):
            if args[0] == 'bash' and any(name in args[2] for name in (
                    'lifecycle_transition', 'lifecycle_block_rule', 'lifecycle_count_rejections')):
                return refresh.subprocess.check_output(args, input=input, text=True)
            self.writes.append(args)
            if args[0]=='bash':
                if 'gh_add_labels' in args[2]:
                    self.labels[int(args[-2])].add(args[-1])
            else:
                number = int(args[-1].split('/issues/')[1].split('/')[0])
                from urllib.parse import unquote
                self.labels[number].discard(unquote(args[-1].split('/labels/')[1]))
            return ''
        for name, replacement in [('api',api), ('run',run), ('observe',lambda *args: dict(self.state))]:
            mock = patch.object(refresh, name, replacement)
            mock.start()
            self.addCleanup(mock.stop)

    def call(self, inspect=False):
        return refresh.refresh('test/repo', 1, 5, 'agent/scope/test', inspect)

    def test_conflict_after_approval_revokes_queue_then_requests_repair(self):
        self.assertTrue(self.call()['changed'])
        self.assertNotIn('agent/review-code-approved', self.labels[5])
        self.assertNotIn('agent/review-code-approved', self.labels[1])
        self.assertIn('agent/code-conflicts', self.labels[1])
        self.assertNotIn('agent/review-code-rejected', self.labels[1])
        self.assertIn('/issues/5/labels/', self.writes[0][-1])
        self.writes.clear()
        self.call()
        self.assertEqual(self.writes, [])

    # Verifies: SDLC-HOOK-14.04
    def test_unknown_mergeability_defers_without_claiming_conflict(self):
        self.state.update(mergeable=None, mergeable_state='unknown')
        self.call()
        self.assertIn('agent/reviewable-code',self.labels[1])
        self.assertNotIn('agent/code-conflicts',self.labels[1])

    def test_inspection_block_revocation_and_closed_pr_never_write(self):
        self.call(inspect=True)
        self.labels[1].add('agent/blocked')
        self.call()
        self.labels[1].remove('agent/blocked')
        self.labels[1].remove('agent/allowed')
        self.call()
        self.labels[1].add('agent/allowed')
        self.state['state']='closed'
        self.call()
        self.assertEqual(self.writes, [])

    def test_dry_run_environment_never_mutates_labels(self):
        for key in ('DRY_RUN','AGENT_DRY_RUN'):
            with patch.dict(refresh.os.environ, {key:'true'}):
                self.call()
        self.assertEqual(self.writes, [])

    def test_new_head_or_base_during_refresh_never_writes(self):
        for which in ('head','base'):
            self.pr[which]['sha']='changed'
            with self.assertRaisesRegex(ValueError,'changed'):
                self.call()
            self.pr[which]['sha']=which
        self.assertEqual(self.writes, [])

    def test_unrelated_pr_never_receives_approval_edits(self):
        self.pr['body']='mentions #11'
        with self.assertRaisesRegex(ValueError,'linked'):
            self.call()
        self.assertEqual(self.writes, [])

    def test_pending_checks_after_approval_reopens_review_and_revokes_queue(self):
        self.state.update(checks='pending', mergeable=True, mergeable_state='clean')
        self.call()
        self.assertIn('agent/reviewable-code',self.labels[1])
        self.assertNotIn('agent/review-code-approved',self.labels[5])

    def test_failed_ci_returns_deferred_or_approved_review_to_implementation(self):
        for phase in ('agent/reviewable-code', 'agent/review-code-approved'):
            self.labels[1] = {'agent/allowed', 'agent/scope/test', phase}
            self.state.update(checks='failing', mergeable=True, mergeable_state='blocked')
            self.pr.update(mergeable=True, mergeable_state='blocked')
            self.assertTrue(self.call()['changed'])
            self.assertIn('agent/review-code-rejected', self.labels[1])
            self.assertNotIn('agent/reviewable-code', self.labels[1])
            self.assertNotIn('agent/review-code-approved', self.labels[5])
            self.writes.clear()
            self.assertFalse(self.call()['changed'])
            self.assertEqual(self.writes, [])

    def test_ci_recovery_uses_timeline_limit(self):
        self.state.update(checks='failing', mergeable=True, mergeable_state='blocked')
        self.timeline = [{'event':'labeled', 'label':{'name':'agent/review-code-rejected'}}] * 9
        self.call()
        self.assertIn('agent/blocked', self.labels[1])

    def test_ci_rerun_or_new_head_before_write_aborts_recovery(self):
        self.state.update(checks='failing', mergeable=True, mergeable_state='blocked')
        for change in ({'checks':'pending'}, {'head_sha':'new'}):
            with patch.object(refresh, 'observe', side_effect=[dict(self.state), dict(self.state, **change)]):
                with self.assertRaisesRegex(ValueError, 'CI state changed'):
                    self.call()
        self.assertEqual(self.writes, [])

    def test_ci_recovery_cannot_silently_lose_the_record(self):
        self.state.update(checks='failing', mergeable=True, mergeable_state='blocked')
        original = refresh.run
        def run(*args, **kwargs):
            if 'gh_post_sticky_comment' in args[2]:
                raise RuntimeError('record failed')
            return original(*args, **kwargs)
        with patch.object(refresh, 'run', run):
            with self.assertRaisesRegex(RuntimeError, 'record failed'):
                self.call()
        self.assertNotIn('agent/review-code-rejected', self.labels[1])


class RevocationFailureTests(unittest.TestCase):
    def test_failed_delete_and_failed_verification_cannot_report_success(self):
        import os
        import tempfile
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            gh = Path(tmp)/'gh'
            gh.write_text('#!/bin/sh\nexit 1\n')
            gh.chmod(0o755)
            library = str(Path(__file__).resolve().parents[1]/'lib/github.sh')
            result = subprocess.run(['bash','-c','. "$1"; gh_remove_label test/repo 5 agent/review-code-approved',
                                     'test',library], env=dict(os.environ, PATH=tmp+':/usr/bin:/bin', GH_MAX_ATTEMPTS='1'),
                                    capture_output=True, timeout=5)
            self.assertNotEqual(result.returncode,0)
