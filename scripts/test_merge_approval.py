# Verifies: SDLC-LOC-12
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'.hypershell/execute-sdlc'))
from funnel_dispatch import Coordinator, DispatchError, GitHub, Store, parse_args, make_job
import merge_approval as approval


class MergeApprovalTests(unittest.TestCase):
    def setUp(self):
        self.github=Mock(repo='o/r', scope='agent/scope/test')
        self.github.labels=GitHub.labels
        self.github.review_request.return_value=None
        self.item=dict(number=42,state='open',html_url='https://github.com/o/r/issues/42',title='Test',labels=['agent/allowed','agent/scope/test',approval.LABEL])
        self.pr=dict(number=5,state='open',html_url='https://github.com/o/r/pull/5',labels=[])
        self.state=dict(changed=False,state='open',draft=False,mergeable=True,checks='green',head_sha='head',base_sha='base')
        self.github.current.return_value=self.item
        self.github.review_pr.return_value=self.pr
        self.github.code_state.return_value=self.state
        self.request=approval.pending(self.github,self.item,self.pr,self.state)
        self.commands=[]
        def command(args):
            self.commands.append(args)
            return [[{'name':approval.LABEL}]] if args[-1]=='--slurp' else []
        patcher=patch.object(approval,'json_command',side_effect=command)
        patcher.start();self.addCleanup(patcher.stop)
        remover=patch.object(approval,'command',side_effect=command)
        remover.start();self.addCleanup(remover.stop)

    def test_only_approved_authorized_ready_unqueued_items_are_listed(self):
        self.assertEqual(self.request['status'],'awaiting_approval')
        for labels in ([], ['agent/allowed',approval.LABEL], self.item['labels']+['agent/blocked'], self.item['labels']+['agent/reviewable-code']):
            self.assertIsNone(approval.pending(self.github,dict(self.item,labels=labels),self.pr,self.state))
        for delta in ({'checks':'failing'},{'draft':True},{'mergeable':None},{'state':'closed'}):
            self.assertIsNone(approval.pending(self.github,self.item,self.pr,dict(self.state,**delta)))
        self.assertIsNone(approval.pending(self.github,self.item,dict(self.pr,labels=[approval.LABEL]),self.state))

    def test_manual_approval_checks_live_inputs_and_verifies_label(self):
        approval.approve(self.github,self.request)
        self.assertEqual(sum('-X' in c and 'POST' in c for c in self.commands),1)
        self.assertEqual(self.github.code_state.call_count,3)

    def test_changed_head_base_ci_link_or_authorization_prevents_mutation(self):
        for delta in ({'head_sha':'new'},{'base_sha':'new'},{'checks':'pending'},{'checks':'failing'},{'mergeable':False}):
            self.github.code_state.return_value=dict(self.state,**delta)
            with self.assertRaises(DispatchError):approval.approve(self.github,self.request)
            self.assertEqual(self.commands,[])
        self.github.code_state.return_value=self.state
        self.github.review_pr.return_value=dict(self.pr,number=6)
        with self.assertRaises(DispatchError):approval.approve(self.github,self.request)
        self.github.review_pr.return_value=self.pr
        self.github.current.return_value=dict(self.item,labels=[])
        with self.assertRaises(DispatchError):approval.approve(self.github,self.request)
        self.assertEqual(self.commands,[])

    def test_changes_during_confirmation_checks_fail_closed(self):
        self.github.code_state.side_effect=[self.state,dict(self.state,head_sha='new')]
        with self.assertRaises(DispatchError):approval.approve(self.github,self.request)
        self.assertEqual(self.commands,[])

    def test_change_after_write_revokes_queue_label(self):
        self.github.code_state.side_effect=[self.state,self.state,dict(self.state,checks='failing')]
        with self.assertRaises(DispatchError):approval.approve(self.github,self.request)
        self.assertIn('POST',self.commands[0]);self.assertIn('DELETE',self.commands[-1])

    def test_unconfirmed_or_wrong_scope_requests_cannot_mutate(self):
        for delta in ({'scope':'other'},{'repo':'other/r'},{'pr_head_sha':''},{'pr_base_sha':''},{'item':'../42'}):
            with self.assertRaises(DispatchError):approval.approve(self.github,dict(self.request,**delta))
        self.assertEqual(self.commands,[])

    def test_discovery_tracks_holds_without_launching_an_agent(self):
        with tempfile.TemporaryDirectory() as tmp:
            args=parse_args(['agent/scope/test','--dry-run','--lifecycle','github-only'])
            store=Store(Path(tmp)/'state.db')
            try:
                coordinator=Coordinator(args,store,-1)
                coordinator.github=self.github
                self.github.code_states={}
                self.github.discover.return_value=[self.item]
                self.github.pages.return_value=[self.pr]
                self.github.linked_prs.return_value=[self.pr]
                self.github.classify.return_value=[]
                coordinator.discover()
                self.assertEqual(len(coordinator.awaiting_approval),1)
                self.assertEqual(coordinator.candidates,[])
                self.assertEqual(coordinator.errors,[])
                self.pr['labels']=[approval.LABEL]
                coordinator.discover()
                self.assertEqual(coordinator.awaiting_approval,[])
                self.pr['labels']=[]
                job=make_job(args.scope,'code-review',self.item,self.pr)
                job['pr_head_sha']=self.state['head_sha']
                store.start(job,'log',100)
                coordinator.discover()
                self.assertEqual(coordinator.awaiting_approval,[])
            finally:store.db.close()
