import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

script = Path(__file__).resolve().parents[1]/'lib/run-state.py'
spec = importlib.util.spec_from_file_location('run_state',script)
state = importlib.util.module_from_spec(spec)
spec.loader.exec_module(state)


class RunStateTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)/'.lifecycle-state.json'
        subprocess.run([sys.executable,'-I',str(script),'write',str(self.path),
                        'code-review','test/repo','1','5','abc1234'],check=True)

    # Verifies: SDLC-HOOK-03.04
    def test_private_snapshot_loads_without_any_relay_environment(self):
        result = subprocess.run([sys.executable,'-I',str(script),'read',str(self.path),
                                 'code-review','test/repo','1'],env={},capture_output=True,text=True,check=True)
        self.assertEqual(self.path.stat().st_mode & 0o777, 0o600)
        self.assertIn('LIFECYCLE_PR_HEAD_SHA=abc1234',result.stdout)
        self.assertIn('LIFECYCLE_PR_NUMBER=5',result.stdout)

    def test_wrong_phase_repo_or_issue_cannot_borrow_state(self):
        for phase,repo,issue in [('spec-review','test/repo',1),('code-review','other/repo',1),('code-review','test/repo',2)]:
            with self.assertRaisesRegex(ValueError,'different'):
                state.load(self.path,phase,repo,issue)

    def test_symlinks_and_nonprivate_files_are_rejected(self):
        link = self.path.with_name('link')
        link.symlink_to(self.path)
        with self.assertRaises(ValueError):state.load(link,'code-review','test/repo',1)
        self.path.chmod(0o644)
        with self.assertRaises(ValueError):state.load(self.path,'code-review','test/repo',1)

    def test_invalid_head_and_shell_content_are_never_interpreted(self):
        data = json.loads(self.path.read_text())
        data['head']='$(touch SHOULD_NOT_EXIST)'
        self.path.write_text(json.dumps(data))
        with self.assertRaises(ValueError):state.load(self.path,'code-review','test/repo',1)
        self.assertFalse(self.path.with_name('SHOULD_NOT_EXIST').exists())
