#!/usr/bin/env bash
set -euo pipefail
python3 - "$(dirname -- "${BASH_SOURCE[0]}")" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

helpers = Path(sys.argv.pop()).resolve()
registry = 'quay.io/aipcc/base-images/agentic/oce-'
revision = 'a' * 40

STUB = r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
name, args = Path(sys.argv[0]).name, sys.argv[1:]
state = Path(os.environ['OCE_KIND_STATE'])
with (state / 'calls').open('a') as log: log.write(json.dumps([name, *args]) + '\n')
failure = os.environ.get('TEST_FAIL', '')
if name == 'docker':
    if failure == 'pull' and args[:1] == ['pull']: sys.exit(42)
    if failure == 'import' and args[:1] == ['exec']: sys.exit(42)
    if args[:2] == ['image', 'inspect']:
        if 'Labels' in args[3]: print('invalid' if failure == 'label' else 'a' * 40)
        else:
            role = 'controller' if 'controller' in args[-1] else 'runtime'
            digit = ('d' if role == 'controller' else 'e') if os.environ.get('LATEST_MOVED') else ('b' if role == 'controller' else 'c')
            reference = args[-1] if '@sha256:' in args[-1] else 'quay.io/aipcc/base-images/agentic/oce-' + role + '@sha256:' + digit * 64
            print(os.environ.get('TEST_DIGESTS', json.dumps([reference])))
elif name == 'git':
    if args[0] == 'init': Path(args[1]).mkdir(parents=True, exist_ok=True)
    elif 'fetch' in args:
        assert args[-1] == 'a' * 40 and '--depth=1' in args
        if failure == 'fetch': sys.exit(42)
    elif 'checkout' in args:
        assert '--force' in args
        helper = Path(os.environ['OCE_REPO']) / 'docs/snippets/openshell-quickstart/kubernetes/service-policies.py'
        helper.parent.mkdir(parents=True, exist_ok=True)
        helper.write_text('print(' + repr('--openshell-namespace' if os.environ.get('ALREADY_FIXED') else 'upstream') + ')')
    elif 'apply' in args:
        assert Path(args[-1]).is_file()
        if failure == 'patch': sys.exit(42)
    elif 'rev-parse' in args:
        if failure == 'rev-parse': sys.exit(42)
        print(os.environ.get('SOURCE_REVISION', 'a' * 40))
elif name == 'go':
    if failure == 'build': sys.exit(42)
    assert args[0] == '-C' and args[-1] == './cmd/occ'
    binary = Path(args[args.index('-o') + 1]); binary.touch(); binary.chmod(0o700)
else: raise AssertionError([name, args])
'''


class PullTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.state = Path(temporary.name) / 'private state'
        self.state.mkdir()
        bins = self.state / 'bin'
        bins.mkdir()
        for name in ('docker', 'git', 'go'):
            stub = bins / name
            stub.write_text(STUB)
            stub.chmod(0o700)
        self.env = dict(os.environ, OCE_KIND_STATE=str(self.state),
                        OCE_REPO=str(self.state / 'oce-source'),
                        PATH=str(bins) + os.pathsep + os.environ['PATH'], TEST_FAIL='', ALREADY_FIXED='',
                        LATEST_MOVED='', SOURCE_REVISION=revision)
        self.env.pop('TEST_DIGESTS', None)

    def run_pull(self, **env):
        return subprocess.run(['bash', str(helpers / 'pull-oce.sh')],
                              env=dict(self.env, **env), capture_output=True, text=True, timeout=15)

    def calls(self):
        return [json.loads(line) for line in (self.state / 'calls').read_text().splitlines()]

    def test_latest_images_are_pinned_and_namespace_patch_is_applied(self):
        result = self.run_pull()
        self.assertEqual(result.returncode, 0, result.stderr)
        images = json.loads((self.state / 'images.json').read_text())
        self.assertEqual(images, {role: registry + role + '@sha256:' + digit * 64
                                  for role, digit in (('controller', 'b'), ('runtime', 'c'))})
        for role in images:
            self.assertIn(['docker', 'pull', registry + role + ':latest'], self.calls())
            self.assertIn(['docker', 'exec', 'hypershell-oce-control-plane', 'ctr', '-n',
                           'k8s.io', 'images', 'pull', images[role]], self.calls())
        self.assertTrue(any('fetch' in c and c[-1] == revision for c in self.calls()))
        self.assertTrue(any('apply' in c for c in self.calls()))
        self.assertTrue((self.state / 'occ').is_file())

    def test_existing_namespace_support_skips_patch(self):
        result = self.run_pull(ALREADY_FIXED='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any('apply' in c for c in self.calls()))

    def test_recovery_uses_saved_digests(self):
        self.assertEqual(self.run_pull().returncode, 0)
        images = (self.state / 'images.json').read_bytes()
        (self.state / 'calls').unlink()
        result = self.run_pull()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.state / 'images.json').read_bytes(), images)
        self.assertFalse(any(':latest' in arg for c in self.calls() for arg in c))

    def test_pull_failure_does_not_checkpoint_images(self):
        result = self.run_pull(TEST_FAIL='pull')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.state / 'images.json').exists())

    def test_preparation_failures_keep_same_images_when_latest_moves(self):
        for failure in ('import', 'label', 'fetch', 'patch', 'build'):
            with self.subTest(failure=failure):
                result = self.run_pull(TEST_FAIL=failure)
                self.assertNotEqual(result.returncode, 0)
                images = (self.state / 'images.json').read_bytes()
                (self.state / 'calls').unlink()
                result = self.run_pull(LATEST_MOVED='1')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((self.state / 'images.json').read_bytes(), images)
                self.assertFalse(any(':latest' in arg for c in self.calls() for arg in c))
                (self.state / 'images.json').unlink()
                (self.state / 'occ').unlink()

    def test_digest_selection_uses_matching_repository(self):
        images = {role: registry + role + '@sha256:' + digit * 64
                  for role, digit in (('controller', 'b'), ('runtime', 'c'))}
        result = self.run_pull(TEST_DIGESTS=json.dumps([
            'mirror.example/oce@sha256:' + 'a' * 64, registry + 'controller@sha256:invalid',
            *images.values()]))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads((self.state / 'images.json').read_text()), images)

    def test_missing_or_invalid_digest_explains_failure(self):
        for digests in ([], ['mirror.example/oce@sha256:' + 'a' * 64],
                        [registry + 'controller@sha256:invalid']):
            with self.subTest(digests=digests):
                result = self.run_pull(TEST_DIGESTS=json.dumps(digests))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('No ' + registry + 'controller digest found', result.stderr)
                self.assertFalse((self.state / 'images.json').exists())

    def test_custom_checkout_warns_on_mismatch_without_modifying_source(self):
        for head in (revision, 'f' * 40, 'unknown'):
            with self.subTest(head=head):
                (self.state / 'calls').unlink(missing_ok=True)
                result = self.run_pull(OCE_REPO=str(self.state / 'custom source'), SOURCE_REVISION=head,
                                       TEST_FAIL='rev-parse' if head == 'unknown' else '')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual('warning: OCE_REPO' in result.stderr, head != revision)
                if head != revision:
                    self.assertIn(head, result.stderr)
                    self.assertIn(revision, result.stderr)
                self.assertFalse(any(c[0] == 'git' and 'rev-parse' not in c for c in self.calls()))


unittest.main()
PY
