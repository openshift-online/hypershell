#!/usr/bin/env bash
# Exercise retries without a cluster, Helm release, or database.
set -euo pipefail
python3 - "$(dirname -- "${BASH_SOURCE[0]}")" <<'PY'
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

helpers = Path(sys.argv.pop()).resolve()

STUB = r'''
import json, os, sys
from pathlib import Path
name, args = Path(sys.argv[0]).name, sys.argv[1:]
state = Path(os.environ['OCE_KIND_STATE'])
with open(state / 'calls', 'a') as log:
    log.write(json.dumps([name, *args]) + '\n')
if name == 'helm':
    if args == ['version', '--short']: print('v3.14.0')
    elif 'list' in args: print('[{}]' if (state / 'release').exists() else '[]')
    elif 'status' in args:
        status = os.environ.get('TEST_STATUS', 'deployed')
        if status == 'unreadable': sys.exit(42)
        print(json.dumps({'info': {'status': status}}))
    else: sys.exit(99)
elif name == 'occ':
    if args[:2] == ['installation', 'doctor']:
        if os.environ.get('TEST_DOCTOR_EXIT', '0') != '0':
            print('Error: Kubernetes connection does not match the recorded installation target', file=sys.stderr)
        sys.exit(int(os.environ.get('TEST_DOCTOR_EXIT', '0')))
    assert args[:2] == ['installation', 'install']
    (state / 'install-receipt').mkdir()
    (state / 'install-receipt/intent.json').write_text('original receipt')
    (state / 'release').touch()
elif name == 'python3':
    if args == ['-']:
        os.execv(sys.executable, [sys.executable, '-'])
    assert args[0].endswith('/render-oce.py')
    for filename in ('installation.json', 'values.json', 'management-policy.json',
                     'gateway-ingress.json', 'runtime-admission.yaml'):
        (state / filename).write_text('{}\n')
    (state / 'gateway-namespace').write_text('openshell-0123456789abcdef\n')
elif name == 'kubectl':
    args = args[4:]  # Explicit kubeconfig and context are always supplied.
    if args[:1] == ['-n']: args = args[2:]
    if args == ['config', 'current-context']: print('kind-hypershell-oce')
    elif args[:3] == ['get', 'namespace', 'kube-system']:
        print(os.environ.get('TEST_CLUSTER_UID', 'cluster-one'))
    elif args[:3] == ['get', 'secret', 'postgres-tls']:
        print('{"data":{"ca.crt":"dGVzdA=="}}')
    elif args[:2] == ['get', 'secret']: print('{"data":{}}')
    elif args[:1] == ['exec']:
        if 'deployment/postgres' in args:
            sys.stdin.read()
            sys.exit(int(os.environ.get('TEST_DATABASE_EXIT', '0')))
        assert 'oce-owner-bootstrap-reader' in args
        if args[-1].endswith('service-key.json') and os.environ.get('TEST_READER_FAIL'):
            print('partial credential')
            sys.exit(42)
        print('credential-from-pvc-' + Path(args[-1]).name)
    elif args[:1] in (['apply'], ['create']):
        if args[-2:] == ['-f', '-']: sys.stdin.read()
        if 'create' in args and '-o' in args: print('{}')
    elif args[:1] not in (['wait'], ['delete'], ['rollout']): sys.exit(99)
elif name != 'prepare-bootstrap-volume': sys.exit(99)
'''


class InstallRetryTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.state = Path(self.temp.name)
        bins = self.state / 'bin'
        bins.mkdir()
        stub = bins / 'stub'
        stub.write_text('#!' + sys.executable + '\n' + STUB)
        stub.chmod(0o700)
        for name in ('helm', 'kubectl', 'python3'):
            (bins / name).symlink_to(stub)
        (self.state / 'occ').symlink_to(stub)
        (self.state / 'scripts').mkdir()
        (self.state / 'scripts/prepare-bootstrap-volume').symlink_to(stub)
        assets = self.state / 'docs/snippets/openshell-quickstart/kubernetes'
        assets.mkdir(parents=True)
        (assets / 'bootstrap-reader.jq').write_text('{}')
        (self.state / 'images.json').write_text('{"controller":"example.com/controller@sha256:abc"}')
        self.env = dict(os.environ, PATH=str(bins) + os.pathsep + os.environ['PATH'],
                        OCE_KIND_STATE=str(self.state), OCE_REPO=str(self.state))

    def install(self, **changes):
        (self.state / 'calls').write_text('')
        result = subprocess.run(['bash', str(helpers / 'install-oce.sh')],
                                env=dict(self.env, **changes), capture_output=True, text=True, timeout=15)
        self.calls = [json.loads(line) for line in (self.state / 'calls').read_text().splitlines()]
        return result

    def successful_install(self):
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.state / 'cluster-uid').read_text(), 'cluster-one\n')

    def assert_no_initialization(self):
        self.assertFalse(any(call[0] in ('python3', 'prepare-bootstrap-volume') for call in self.calls))
        self.assertNotIn(['occ', 'installation', 'install'], [call[:3] for call in self.calls])
        self.assertFalse(any('deployment/postgres' in call for call in self.calls))

    def test_repeat_preserves_activated_configuration_credentials_and_receipt(self):
        self.successful_install()
        (self.state / 'installation.json').write_text('{"activated":true}')
        (self.state / 'values.json').write_text('{"worker":{"replicaCount":1}}')
        files = ['installation.json', 'values.json', 'initial-admin-password',
                 'initial-admin-service-key.json', 'install-receipt/intent.json',
                 'secrets/application-url', 'secrets/migration-url', 'secrets/auth-secret', 'secrets/routing-key']
        before = {name: (self.state / name).read_bytes() for name in files}
        # Later upgrades replace the original Job, making the original doctor check fail.
        result = self.install(TEST_DOCTOR_EXIT='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_no_initialization()
        self.assertFalse(any(call[0] == 'occ' or 'exec' in call for call in self.calls))
        self.assertEqual(before, {name: (self.state / name).read_bytes() for name in files})

    def test_interrupted_credential_read_recovers_only_missing_file(self):
        result = self.install(TEST_READER_FAIL='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.state / 'initial-admin-service-key.json').exists())
        password = (self.state / 'initial-admin-password').read_bytes()
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_no_initialization()
        reads = [call[-1] for call in self.calls if 'exec' in call]
        self.assertEqual(reads, ['/bootstrap/initial-admin-service-key.json'])
        self.assertEqual((self.state / 'initial-admin-password').read_bytes(), password)
        self.assertNotIn('partial', (self.state / 'initial-admin-service-key.json').read_text())

    def test_wrong_cluster_and_unhealthy_releases_stop_before_mutations(self):
        self.successful_install()
        for changes in ({'TEST_CLUSTER_UID': 'replacement-cluster'},
                        *({'TEST_STATUS': status} for status in ('failed', 'pending-install', 'pending-upgrade', 'unreadable'))):
            with self.subTest(changes=changes):
                result = self.install(**changes)
                self.assertNotEqual(result.returncode, 0)
                self.assert_no_initialization()
                self.assertFalse(any(set(call) & {'exec', 'apply', 'create', 'delete'} for call in self.calls))
                if 'TEST_CLUSTER_UID' in changes:
                    self.assertIn(str(self.state), result.stderr)
                    self.assertIn('export OCE_KIND_STATE=', result.stderr)

    def test_legacy_receipt_requires_doctor_before_adopting_cluster(self):
        self.successful_install()
        (self.state / 'cluster-uid').unlink()
        result = self.install(TEST_DOCTOR_EXIT='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.state / 'cluster-uid').exists())
        self.assert_no_initialization()
        self.assertIn('Kubernetes connection does not match the recorded installation target', result.stderr)
        self.assertIn(str(self.state / 'kubeconfig'), result.stderr)
        for variable in ('OCE_KIND_STATE', 'KUBECONFIG', 'PATH', 'HELM_PLUGINS'):
            self.assertIn('export ' + variable + '=', result.stderr)
        self.assertIn('current terminal', result.stderr)
        self.assertIn('Keep the old directory', result.stderr)
        self.assertFalse(any(set(call) & {'exec', 'apply', 'create', 'delete'} for call in self.calls))
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.state / 'cluster-uid').exists())
        self.assert_no_initialization()

    def test_missing_receipt_does_not_overwrite_existing_release(self):
        (self.state / 'release').touch()
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('no saved receipt', result.stderr)
        self.assert_no_initialization()
        self.assertFalse((self.state / 'secrets').exists())

    def test_interrupted_database_does_not_regenerate_passwords(self):
        result = self.install(TEST_DATABASE_EXIT='42')
        self.assertNotEqual(result.returncode, 0)
        before = (self.state / 'secrets/application-url').read_bytes()
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Database setup was interrupted', result.stderr)
        self.assertEqual((self.state / 'secrets/application-url').read_bytes(), before)
        self.assertFalse(any('deployment/postgres' in call for call in self.calls))

    def test_concurrent_installer_is_rejected(self):
        with (self.state / 'install.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Another OCE installer', result.stderr)
        self.assert_no_initialization()


class ActivationRetryTest(unittest.TestCase):
    def test_failed_upgrade_keeps_local_configuration_and_successful_repeat_skips_upgrade(self):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            assets = state / 'docs/snippets/openshell-quickstart'
            (assets / 'kubernetes').mkdir(parents=True)
            (assets / 'request.mjs').touch()
            (assets / 'kubernetes/trusted-envoy-peers.sh').write_text('echo \'["10.0.0.1/32"]\'\n')
            (state / 'gateway-namespace').write_text('openshell-0123456789abcdef\n')
            (assets / 'kubernetes/service-policies.py').write_text('''
import json, os, sys
assert sys.argv[1:] == ['--tenant', 'tenant', '--namespace-id', 'test-id',
                        '--openshell-namespace', 'openshell-0123456789abcdef']
if os.environ.get('TEST_NETWORK_FAIL') == 'render':
    sys.exit(43)
print(json.dumps({'apiVersion': 'v1', 'kind': 'List', 'items': []}))
''')
            (assets / 'operator-session.sh').write_text('''
kube() {
  case "$*" in
    'get ns -l '*) echo '{"items":[{"metadata":{"name":"tenant"}}]}' ;;
    'get ns tenant -o json') echo '{"metadata":{"labels":{"pod-security.kubernetes.io/enforce":"restricted"}}}' ;;
    *'--dry-run=client -o json') echo '{"rules":[]}' ;;
    'apply -f -') cat >/dev/null ;;
    'apply -f '*'/namespace-service-peers.json')
      [[ ${TEST_NETWORK_FAIL:-} != apply ]] || return 44
      jq -e '.kind == "List"' "$3" >/dev/null || return
      echo network-applied >> "$RUN_DIR/network-calls" ;;
  esac
}
request() {
  if [[ $2 == /namespaces ]]; then echo '{"data":[{"name":"default","id":"test-id"}]}'
  else echo '{"data":{"status":"ready"}}'; fi
}
upgrade_installation() {
  echo upgrade >> "$RUN_DIR/upgrade-calls"
  [[ ${TEST_UPGRADE_FAIL:-} != yes ]] || return 42
  jq '.controlPlane.installationChecksum="test-checksum"' "$VALUES_FILE" > "$RUN_DIR/checksummed.json"
  mv "$RUN_DIR/checksummed.json" "$VALUES_FILE"
}
''')
            installation = state / 'installation.json'
            values = state / 'values.json'
            installation.write_text('{"drivers":{"compute":{"configuration":{"network":{}}}}}')
            values.write_text('{"worker":{"replicaCount":0}}')
            before = installation.read_bytes(), values.read_bytes()
            env = dict(os.environ, OCE_KIND_STATE=str(state), OCE_REPO=str(state),
                       OCE_NAMESPACE_NAME='default', TEST_UPGRADE_FAIL='yes')
            result = subprocess.run(['bash', str(helpers / 'activate-oce.sh')], env=env,
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 42, result.stderr)
            self.assertEqual(before, (installation.read_bytes(), values.read_bytes()))
            env['TEST_UPGRADE_FAIL'] = ''
            result = subprocess.run(['bash', str(helpers / 'activate-oce.sh')], env=env,
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(values.read_text())['worker']['replicaCount'], 1)
            self.assertEqual(json.loads(values.read_text())['controlPlane']['installationChecksum'], 'test-checksum')
            self.assertEqual(json.loads(installation.read_text())['drivers']['compute']['configuration']['network']
                             ['gatewayTrustedProxyCidrs'], ['10.0.0.1/32'])
            env['TEST_UPGRADE_FAIL'] = 'yes'
            result = subprocess.run(['bash', str(helpers / 'activate-oce.sh')], env=env,
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((state / 'upgrade-calls').read_text().splitlines(), ['upgrade', 'upgrade'])
            self.assertEqual((state / 'network-calls').read_text().splitlines(),
                             ['network-applied', 'network-applied'])
            for failure, status in [('render', 43), ('apply', 44)]:
                with self.subTest(network_failure=failure):
                    env['TEST_NETWORK_FAIL'] = failure
                    result = subprocess.run(['bash', str(helpers / 'activate-oce.sh')], env=env,
                                            capture_output=True, text=True, timeout=15)
                    self.assertEqual(result.returncode, status, result.stderr)
                    self.assertNotIn('OCE Namespace ready:', result.stdout)
                    self.assertEqual((state / 'network-calls').read_text().splitlines(),
                                     ['network-applied', 'network-applied'])


unittest.main()
PY
