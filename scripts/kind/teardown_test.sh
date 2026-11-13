#!/usr/bin/env bash
# Exercise teardown without deleting real clusters or host networking.
set -euo pipefail
python3 - "$(dirname -- "${BASH_SOURCE[0]}")" <<'PY'
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

scripts = Path(sys.argv.pop()).resolve()


class TeardownTest(unittest.TestCase):
    def run_teardown(self, existing, selected='hypershell-dev', failure=''):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        shutil.copyfile(scripts / 'teardown.sh', root / 'teardown.sh')
        (root / 'lib.sh').write_text('''
fixture_dir="$SCRIPT_DIR"
SCRIPT_DIR=$(dirname "$TEARDOWN_TEST_LIB")
source "$TEARDOWN_TEST_LIB"
SCRIPT_DIR="$fixture_dir"
CPK_SHA_MARKER="$TEARDOWN_TEST_ROOT/$KIND_CLUSTER_NAME-cpk.sha"
KUBECTL_PF_DIR="$TEARDOWN_TEST_ROOT/$KIND_CLUSTER_NAME-pf"
stop_port_forward() {
  printf '["stop_port_forward","%s"]\\n' "$KIND_CLUSTER_NAME" >> "$TEARDOWN_TEST_ROOT/calls"
}
''')
        bins = root / 'bin'
        bins.mkdir()
        stub = bins / 'stub'
        stub.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
name, args = Path(sys.argv[0]).name, sys.argv[1:]
root = Path(os.environ['TEARDOWN_TEST_ROOT'])
with (root / 'calls').open('a') as log:
    log.write(json.dumps([name, *args]) + '\\n')
if name == 'kind':
    if args == ['get', 'clusters']:
        print(os.environ['TEARDOWN_TEST_EXISTING'])
    else:
        assert args[:3] == ['delete', 'cluster', '--name']
        if args[-1] == 'hypershell-oce':
            assert os.environ['KIND_EXPERIMENTAL_PROVIDER'] == 'docker'
            assert os.environ['CONTAINER_ENGINE'] == 'docker'
        if args[-1] == os.environ['TEARDOWN_TEST_FAILURE']: sys.exit(42)
elif name in ('docker', 'podman'):
    if args[0] == 'inspect': sys.exit(1)
    assert args == ['rm', '-f', os.environ['KIND_CLUSTER_NAME'] + '-dns']
elif name in ('pgrep', 'sudo'): sys.exit(1)
else: raise AssertionError([name, args])
''')
        stub.chmod(0o700)
        for name in ('kind', 'docker', 'podman', 'pgrep', 'sudo'):
            (bins / name).symlink_to(stub)
        clusters = ('hypershell-dev', 'hypershell-oce', 'custom', 'unrelated')
        for cluster in clusters:
            (root / (cluster + '-cpk.sha')).touch()
            (root / (cluster + '-pf')).mkdir()
        saved_state = root / 'oce-hypershell-kind-state'
        saved_state.mkdir()
        (saved_state / 'credentials').write_text('preserve')
        env = dict(os.environ, PATH=str(bins) + os.pathsep + os.environ['PATH'],
                   TEARDOWN_TEST_LIB=str(scripts / 'lib.sh'), TEARDOWN_TEST_ROOT=str(root),
                   TEARDOWN_TEST_EXISTING='\n'.join(existing), TEARDOWN_TEST_FAILURE=failure,
                   KIND_CLUSTER_NAME=selected, CONTAINER_ENGINE='podman',
                   KIND_EXPERIMENTAL_PROVIDER='podman')
        result = subprocess.run(['bash', str(root / 'teardown.sh')], cwd=root, env=env,
                                capture_output=True, text=True, timeout=10)
        self.assertTrue((root / 'calls').exists(), result.stderr)
        calls = [json.loads(line) for line in (root / 'calls').read_text().splitlines()]
        self.assertEqual((saved_state / 'credentials').read_text(), 'preserve')
        return result, calls, root

    # Verifies: DEV-LOC-03.02, DEV-LOC-03.03
    def test_default_handles_neither_either_or_both_standard_clusters(self):
        for existing in ([], ['hypershell-dev'], ['hypershell-oce'],
                         ['hypershell-dev', 'hypershell-oce']):
            with self.subTest(existing=existing):
                result, calls, root = self.run_teardown([*existing, 'unrelated'])
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual([c[-1] for c in calls if c[:2] == ['kind', 'delete']], existing)
                for cluster, engine in (('hypershell-dev', 'podman'), ('hypershell-oce', 'docker')):
                    self.assertIn([engine, 'rm', '-f', cluster + '-dns'], calls)
                    self.assertIn(['stop_port_forward', cluster], calls)
                    self.assertFalse((root / (cluster + '-cpk.sha')).exists())
                    self.assertFalse((root / (cluster + '-pf')).exists())
                self.assertTrue((root / 'unrelated-cpk.sha').exists())
                self.assertTrue((root / 'unrelated-pf').is_dir())

    # Verifies: DEV-LOC-03.04
    def test_non_default_name_targets_only_selected_cluster(self):
        for selected in ('custom', 'hypershell-oce'):
            with self.subTest(selected=selected):
                result, calls, root = self.run_teardown(
                    ['hypershell-dev', 'hypershell-oce', 'custom'], selected=selected)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual([c[-1] for c in calls if c[:2] == ['kind', 'delete']], [selected])
                self.assertTrue((root / 'hypershell-dev-cpk.sha').exists())

    def test_delete_failure_is_reported(self):
        result, _, _ = self.run_teardown(['hypershell-dev', 'hypershell-oce'], failure='hypershell-oce')
        self.assertEqual(result.returncode, 42, result.stderr)
        self.assertNotIn('Done.', result.stdout)


unittest.main()
PY
