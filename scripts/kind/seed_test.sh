#!/usr/bin/env bash
# Exercise operator-mode seeding without a cluster or credentials.
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


class SeedTest(unittest.TestCase):
    def run_seed(self, mode='', failure='', operator=True):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        seed = root / 'scripts/kind/seed.sh'
        seed.parent.mkdir(parents=True)
        shutil.copyfile(scripts / 'seed.sh', seed)
        shutil.copyfile(scripts.parents[1] / 'OPENSHELL_VERSION', root / 'OPENSHELL_VERSION')
        (root / 'components/cli').mkdir(parents=True)
        (root / 'tmp').mkdir()
        (seed.parent / 'lib.sh').write_text('''
source "$SEED_TEST_LIB"
require_cluster() { :; }
reconcile_keycloak_seed_users() { :; }
kube() { echo 'Forwarding from 127.0.0.1'; exec sleep 30; }
''')
        bins = root / 'bin'
        bins.mkdir()
        stub = bins / 'stub'
        stub.write_text('''#!/usr/bin/env python3
import json, os, shutil, sys
from pathlib import Path
name, args = Path(sys.argv[0]).name, sys.argv[1:]
root = Path(os.environ['SEED_TEST_ROOT'])
with (root / 'calls').open('a') as log: log.write(json.dumps([name, *args]) + '\\n')
failure = os.environ['SEED_TEST_FAIL']
if name == 'go':
    target = Path(args[args.index('-o') + 1])
    shutil.copyfile(__file__, target); target.chmod(0o700)
elif name == 'hsctl':
    config = Path(os.environ['HYPERSHELL_CONFIG'])
    assert config.parent.stat().st_mode & 0o777 == 0o700
    if args[0] == 'login':
        assert args[-2:] == ['--token-file', '/dev/stdin']
        assert sys.stdin.read() == 'fake-access-token'
        if failure == 'login': sys.exit(42)
        config.write_text('{}')
    else:
        assert config.is_file()
        assert args == ['create', 'gateway', '--name', 'dev-gateway', '--placement-selector', 'provider/kind', '--operator-workspaces']
        if failure == 'create': sys.exit(42)
        print(json.dumps({'id': 'gw-one', 'workspace_mode': 'operator'} if failure != 'missing-id' else {}, indent=2))
elif name == 'curl':
    url = next(arg for arg in args if arg.startswith(('http://', 'https://')))
    if 'openid-connect/token' in ' '.join(args):
        print('{"access_token":"fake-access-token"}')
    elif url.endswith('/managed_clusters'):
        print('{"items":[{"id":"cluster-one","name":"local-kind","oidc_subject":"controller"}]}\\n200')
    elif '-w' in args:
        if args[args.index('-X') + 1] == 'POST':
            body = json.loads(args[args.index('-d') + 1])
            assert body == {'name': 'dev-gateway', 'placement': {'selector': ['provider/kind']}}
            print('{"id":"gw-one"}\\n201')
        else:
            mode = os.environ['SEED_TEST_MODE']
            items = [{'id': 'gw-one', 'name': 'dev-gateway', 'workspace_mode': mode}] if mode else []
            print(json.dumps({'items': items}, separators=(',', ':')) + '\\n' + ('503' if failure == 'list' else '200'))
''')
        stub.chmod(0o700)
        for name in ('go', 'curl'):
            (bins / name).symlink_to(stub)
        env = dict(os.environ, PATH=str(bins) + os.pathsep + os.environ['PATH'],
                   SEED_TEST_LIB=str(scripts / 'lib.sh'), SEED_TEST_ROOT=str(root),
                   SEED_TEST_MODE=mode, SEED_TEST_FAIL=failure, SEED_STRICT='true',
                   SEED_OPERATOR_WORKSPACES=str(operator).lower(), TMPDIR=str(root / 'tmp'))
        result = subprocess.run(['bash', str(seed)], env=env, capture_output=True, text=True, timeout=10)
        calls = [json.loads(line) for line in (root / 'calls').read_text().splitlines()]
        self.assertEqual(list((root / 'tmp').iterdir()), [], result.stderr)
        self.assertNotIn('fake-access-token', result.stdout + result.stderr)
        return result, calls

    def test_operator_creation_uses_hsctl_and_accepts_pretty_json(self):
        result, calls = self.run_seed()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Gateway created: gw-one', result.stdout)
        self.assertEqual([c[:2] for c in calls if c[0] == 'hsctl'], [['hsctl', 'login'], ['hsctl', 'create']])

    def test_existing_operator_gateway_is_reused_without_building_or_mutating(self):
        result, calls = self.run_seed(mode='operator')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(c[0] in ('hsctl', 'go') for c in calls))

    def test_wrong_mode_and_failed_list_do_not_create_a_gateway(self):
        for mode, failure in (('managed', ''), ('shared', ''), ('', 'list')):
            with self.subTest(mode=mode, failure=failure):
                result, calls = self.run_seed(mode=mode, failure=failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(c[0] in ('hsctl', 'go') for c in calls))
                self.assertIn('immutable' if mode else 'refusing to create', result.stderr)

    def test_cli_errors_and_missing_id_fail_seeding(self):
        for failure in ('login', 'create', 'missing-id'):
            with self.subTest(failure=failure):
                result, calls = self.run_seed(failure=failure)
                self.assertNotEqual(result.returncode, 0)

    def test_ordinary_seed_keeps_managed_default_and_rest_flow(self):
        result, calls = self.run_seed(operator=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(c[0] in ('hsctl', 'go') for c in calls))
        self.assertIn('Gateway created: gw-one', result.stdout)


unittest.main()
PY
