# Verifies: SDLC-LOC-06.03
import json
from pathlib import Path
import sqlite3
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]/'.hypershell/execute-sdlc'))
from intake_probe import IntakeProbe, pending_items
from reconcile_probe import JiraReader, ProbeError
from funnel_dispatch import parse_args

SCOPE='agent/scope/test'


def github(number=1, linked=True):
    return {'number':number,'html_url':f'https://github.com/test/repo/issues/{number}',
            'labels':[SCOPE,'agent/allowed']+(['agent/execution-record'] if linked else []),
            'body':'TEST-1' if linked else 'New idea','title':'Example','state':'closed'}


def jira():
    return {'key':'TEST-1','fields':{'summary':'Example','labels':[SCOPE,'agent/allowed'],
            'description':{'type':'doc','content':[{'type':'paragraph','content':[{'type':'text','text':github()['html_url']}]}]}}}


class IntakeTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.db=sqlite3.connect(Path(self.tmp.name)/'state.db');self.db.row_factory=sqlite3.Row
        self.addCleanup(self.db.close)
        self.read=Mock(return_value=[jira()]);self.probe=IntakeProbe(self.db,SCOPE,self.read)

    def test_healthy_closed_pairs_stay_quiet_across_intervals_and_restart(self):
        for now in (1000,1100,1180,1360):
            self.assertIsNone(self.probe.observe([github()],180,now))
        self.assertEqual(self.read.call_count,3)
        self.probe=IntakeProbe(self.db,SCOPE,self.read)
        self.assertIsNone(self.probe.observe([github()],180,1361))
        self.assertEqual(self.read.call_count,3)

    def test_github_origin_counterpart_does_not_need_jira_scope_labels(self):
        item=jira();item['fields']['labels']=[]
        self.assertEqual([],pending_items(SCOPE,[github()],[item]))
        reader=JiraReader({},SCOPE,Path(self.tmp.name))
        reader.search=Mock(return_value=[])
        reader.request=Mock(return_value=item)
        self.assertEqual(reader.intake_items([github()]),[item])
        reader.request.assert_called_once()
        reader.request.reset_mock()
        unauthorized=github();unauthorized['labels']=[SCOPE,'agent/execution-record']
        self.assertEqual(reader.intake_items([unauthorized]),[])
        reader.request.assert_not_called()

    def test_new_unlinked_and_changed_inputs_have_stable_work_identity(self):
        first=self.probe.observe([github(linked=False)],180,1000)
        self.assertIsNotNone(first)
        self.assertEqual(first,self.probe.observe([github(linked=False)],180,1180))
        changed=github(linked=False);changed['title']='Changed intent'
        self.assertNotEqual(first,self.probe.observe([changed],180,1360))
        self.assertIsNone(self.probe.observe([github()],180,1540))

    def test_healthy_pair_field_changes_belong_to_reconciliation(self):
        item=jira();item['fields']['summary']='New title'
        self.assertEqual([],pending_items(SCOPE,[github()],[item]))
        duplicate=github(2)
        self.assertTrue(pending_items(SCOPE,[github(),duplicate],[item]))
        unauthorized=github(linked=False);unauthorized['labels']=[SCOPE]
        self.assertEqual([],pending_items(SCOPE,[unauthorized],[]))

    def test_error_is_visible_and_does_not_overwrite_successful_observation(self):
        self.assertIsNone(self.probe.observe([github()],180,1000))
        self.read.side_effect=ProbeError('network')
        with self.assertRaises(ProbeError):self.probe.observe([github()],180,1180)
        with self.assertRaises(ProbeError):self.probe.observe([github()],180,1181)
        self.assertEqual(self.read.call_count,2)
        self.read.side_effect=None
        self.assertIsNone(self.probe.observe([github()],180,1241))

    def test_interval_defaults_environment_override_and_legacy_flag(self):
        with patch.dict('os.environ',{},clear=True):
            self.assertEqual(parse_args([SCOPE,'--dry-run']).lifecycle_interval,180)
        with patch.dict('os.environ',{'FULLSEND_INTAKE_INTERVAL':'300'}):
            self.assertEqual(parse_args([SCOPE,'--dry-run']).lifecycle_interval,300)
            self.assertEqual(parse_args([SCOPE,'--dry-run','--intake-interval','60']).lifecycle_interval,60)
            self.assertEqual(parse_args([SCOPE,'--dry-run','--lifecycle-interval','90']).lifecycle_interval,90)

    def test_jira_search_paginates_and_rejects_truncation(self):
        reader=JiraReader({},SCOPE,Path(self.tmp.name))
        reader.request=Mock(side_effect=[{'issues':[jira()],'nextPageToken':'next','isLast':False},
                                        {'issues':[{'key':'TEST-2'}],'isLast':True}])
        self.assertEqual(len(reader.search()),2)
        self.assertEqual(reader.request.call_args[0][1]['nextPageToken'],'next')
        reader.request=Mock(return_value={'issues':[]})
        with self.assertRaises(ProbeError):reader.search()
        reader.request=Mock(return_value={'issues':[],'isLast':False})
        with self.assertRaises(ProbeError):reader.search()
        reader.request=Mock(return_value={'issues':[],'isLast':False,'nextPageToken':'repeated'})
        with self.assertRaises(ProbeError):reader.search()

if __name__=='__main__':unittest.main()
