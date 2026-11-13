#!/usr/bin/env bash
# Exercise the make entrypoint and OCE API workflow without Docker or real credentials.
set -euo pipefail
python3 - "$(dirname -- "${BASH_SOURCE[0]}")" <<'PY'
import fcntl
import errno
import json
import os
from pathlib import Path
import pty
import re
import select
import shutil
import subprocess
import sys
import tempfile
import unittest

helpers = Path(sys.argv.pop()).resolve()

TOOLS = r'''#!/usr/bin/env python3
import json, os, sys, yaml
from pathlib import Path
name, args = Path(sys.argv[0]).name, sys.argv[1:]
infra = Path(os.environ['OCE_TEST_INFRA'])
state = Path(os.environ.get('OCE_KIND_STATE') or infra)
with (infra / 'calls').open('a') as log: log.write(json.dumps([name, *args]) + '\n')
if name == 'kind':
    if args == ['get', 'clusters']:
        if (infra / 'cluster-exists').exists(): print('hypershell-oce')
    elif args[:2] == ['get', 'kubeconfig']:
        print('live cluster kubeconfig')
    else:
        assert args[:2] == ['create', 'cluster']
        (infra / 'cluster-exists').touch()
        (state / 'kubeconfig').write_text('private kubeconfig')
elif name == 'kubectl':
    if 'current-context' in args: print('kind-hypershell-oce')
    elif 'kube-system' in args and 'namespace' in args:
        if os.environ.get('TEST_DISCOVERY_FAILURE'): sys.exit(42)
        print(os.environ.get('TEST_CLUSTER_UID', 'cluster-one'))
    elif 'keycloak' in args and 'get' in args:
        print(json.dumps({'spec': {'template': {'spec': {'containers': [{'env': [
            {'name': 'KC_HOSTNAME', 'value': 'https://keycloak.hypershell.localhost:32443'}]}]}}}}))
    elif 'deployment/hypershell-controller' in args and 'get' in args:
        if os.environ.get('TEST_FAIL') == 'controller-image': sys.exit(42)
        print(os.environ.get('TEST_CONTROLLER_IMAGE', 'registry.example.com/controller:latest'))
    elif 'openshell-gateway' in args and 'grpcroute' in args:
        print('{"spec":{"hostnames":["gw-openshell-0123456789abcdef.hypershell.localhost"]}}')
    elif 'hypershell-gw' in args:
        print('{"status":{"addresses":[{"type":"IPAddress","value":"172.18.0.3"}]}}')
    elif 'openshell-gateway' in args and 'service' in args:
        print('{"metadata":{"namespace":"openshell-0123456789abcdef"},"spec":{"selector":{"app":"gateway"}}}')
    elif 'hypershell-ca-secret' in args: print('{"data":{"ca.crt":"Y2VydA=="}}')
    elif 'metadata.name=openshell-gateway' in args:
        print('{"items":[{"metadata":{"namespace":"openshell-0123456789abcdef"}}]}')
    elif 'get' in args and 'ns' in args: print('{"items":[{"metadata":{"name":"tenant-one"}}]}')
    elif 'envoyproxy' in args:
        print('{"spec":{"provider":{"kubernetes":{"envoyDeployment":{"container":{"image":"envoy:pinned"}}}}}}')
    elif 'port-forward' in args: sys.exit('unexpected port forward')
elif name == 'docker':
    if args[0] == 'create': print('temporary-container')
    elif args[0] == 'cp':
        target = Path(args[-1]); target.write_text('#!/bin/sh\necho v3.14.0\n'); target.chmod(0o700)
elif name == 'make':
    assert os.environ['KUBECONFIG'] == str(state / 'kubeconfig')
    assert os.environ['KIND_NO_SUDO'] == 'true'
    if args[0] == 'kind-up':
        assert 'SKIP_SEED=true' in args
        settings = dict(os.environ)
        settings.update(arg.split('=', 1) for arg in args[1:] if '=' in arg)
        for key in ('LOCAL_IMAGES', 'BUILD_SOURCE', 'KIND_SKIP_BUILD', 'KIND_COVERAGE'):
            assert settings.get(key, '') == os.environ.get('TEST_EXPECT_' + key, ''), key
    elif args[0] == 'kind-seed':
        assert 'SEED_STRICT=true' in args and 'SEED_OPERATOR_WORKSPACES=true' in args
        calls = [json.loads(line) for line in (infra / 'calls').read_text().splitlines()]
        patches = [yaml.safe_load(call[-1]) for call in calls if 'patch' in call]
        assert patches and all(p['metadata']['name'] == 'hypershell-controller' for p in patches)
        settings = {e['name']: e['value'] for e in patches[-1]['spec']['template']['spec']['containers'][0]['env']}
        assert settings['GATEWAY_OPERATOR_NAMESPACE_LABEL_KEY'] == 'hypershell.io/gateway'
        assert settings['GATEWAY_OPERATOR_SANDBOX_RUNTIME_CLASS'] == 'openshell-sandbox'
        assert calls.index(['prepare-oce']) < len(calls) - 1
    else: raise AssertionError(args)
    if os.environ.get('TEST_FAIL') == args[0]: sys.exit(42)
elif name == 'curl':
    if '--dump-header' in args:
        Path(args[args.index('--dump-header') + 1]).write_text('HTTP/2 200\r\ngrpc-status: ' + ('14' if os.environ.get('TEST_FAIL') == 'health' else '0') + '\r\n')
    print('{"data":null}')
elif name == 'sleep': pass
else:
    raise AssertionError([name, args])
'''

API = r'''import json, os, re, sys
from pathlib import Path
state = Path(os.environ['OCE_KIND_STATE'])
method, path = sys.argv[1:3]
body = json.load(sys.stdin) if method == 'POST' else None
with (state / 'api-calls').open('a') as log: log.write(json.dumps([method, path, body]) + '\n')
namespace = 'ns_one'
base = '/namespaces/' + namespace
store = state / 'api-store.json'
db = json.loads(store.read_text()) if store.exists() else {}
failure = os.environ.get('TEST_FAIL', '')
if method == 'GET':
    if path == base: data = {'status': 'ready'}
    elif '/agents/provision/' in path:
        data = {'status': 'failed' if failure == 'deployment' else 'succeeded',
                'agentId': 'agt_one', 'revisionId': 'rev_one'}
    else: data = db[path]
else:
    assert (state / 'provider-ready').exists(), 'provider must be installed before credentials'
    if path == base + '/agents/provision':
        assert re.fullmatch(r'req_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}', body['requestId'])
        assert body['name'] == 'codex-test' and body['nativeAdmin'] is True
        assert body['executionMode'] == 'dedicated' and body['gatewayManagedMode'] is True
        assert body['harnessAuth'] == {'method': 'credential_source', 'sourceId': 'src_one'}
        assert body['configuration']['values']['agents']['defaults']['model'] == 'codex/gpt-6-luna'
        if 'provision-body' in db: assert body == db['provision-body'], 'retry changed the request'
        db['provision-body'] = body
        data = {'provisioning': {'workId': 'work_one'}}
    else:
        assert path in (base + '/secrets', base + '/credential-sources')
        ident = 'sec_one' if path.endswith('/secrets') else 'src_one'
        assert path + '/' + ident not in db, 'duplicate credential mutation'
        data = {**body, 'id': ident, 'namespaceId': namespace, 'ref': 'secret:sec_one', 'state': 'ready'}
        data.pop('value', None)
        db[path + '/' + ident] = data
    store.write_text(json.dumps(db))
    marker = state / ('lost-' + failure)
    if ((failure == 'lost-secret' and path.endswith('/secrets')) or
        (failure == 'lost-provision' and path.endswith('/agents/provision'))) and not marker.exists():
        marker.touch(); sys.exit(42)
print(json.dumps({'data': data}))
'''


class AutomationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo, self.oce, self.state, self.bin = [self.root / p for p in
                                                    ('HyperShell repo', 'OCE repo', 'private state', 'bin')]
        for path in (self.repo, self.oce, self.state, self.bin): path.mkdir()
        self.infra = self.root / 'infrastructure'
        self.infra.mkdir()
        self.state_home = self.root / 'state home'
        self.state_home.mkdir()
        self.key = self.root / 'api key'
        self.key.write_text('fake-key-$-"-\\\n')
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        OCE_KIND_STATE=str(self.state), OCE_REPO=str(self.oce),
                        XDG_STATE_HOME=str(self.state_home), OCE_TEST_INFRA=str(self.infra),
                        ENMAAS_API_KEY_FILE=str(self.key), TEST_FAIL='')
        for name in ('GATEWAY_NAMESPACE', 'MAKEFLAGS', 'MFLAGS', 'MAKEOVERRIDES',
                     'LOCAL_IMAGES', 'BUILD_SOURCE', 'KIND_SKIP_BUILD', 'KIND_COVERAGE', 'CONTROL_PLANE_IMAGE'):
            self.env.pop(name, None)
        for tool in ('kind', 'kubectl', 'docker', 'make', 'sleep', 'curl'):
            stub = self.bin / tool; stub.write_text(TOOLS); stub.chmod(0o700)
        self.assets = self.oce / 'docs/snippets/openshell-quickstart'
        (self.assets / 'kubernetes').mkdir(parents=True)

    def run_script(self, path, **env):
        return subprocess.run(['bash', str(path)], env=dict(self.env, **env),
                              capture_output=True, text=True, timeout=20)

    def calls(self, file='calls'):
        path = (self.infra if file == 'calls' else self.state) / file
        return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []

    def prepare_up(self):
        target = self.repo / 'deploy/oce-kind'; target.mkdir(parents=True)
        for file in ('up.sh', 'select-state.py', 'kind.yaml', 'pod-security.yaml', 'hypershell-operator.yaml'):
            shutil.copyfile(helpers / file, target / file)
        for file in ('cmd/occ/main.go', 'deploy/helm/openclaw-enterprise/Chart.yaml', 'docs/assets/openshell-quickstart/chat.mjs',
                     'apps/controller/src/gateway/native-admin-host.mjs',
                     'docs/snippets/openshell-quickstart/operator-session.sh',
                     'docs/snippets/openshell-quickstart/platform-kubernetes.sh',
                     'docs/snippets/openshell-quickstart/codex-configuration.jq'):
            path = self.oce / file; path.parent.mkdir(parents=True, exist_ok=True); path.touch()
        (self.assets / 'kubernetes/service-policies.py').write_text('print("--openshell-namespace")')
        self.env['OCE_TEST_SOURCE'] = str(self.oce)
        for stage in ('pull-oce', 'prepare-oce', 'install-oce', 'activate-oce', 'browser-access', 'seed-agent', 'access'):
            (target / (stage + '.sh')).write_text('set -eu\n'
                f'echo \'["{stage}"]\' >> "$OCE_TEST_INFRA/calls"\n'
                f'[[ $TEST_FAIL != {stage} ]]\n'
                + ('''echo '{}' > "$OCE_KIND_STATE/images.json"
touch "$OCE_KIND_STATE/occ"
chmod 700 "$OCE_KIND_STATE/occ"
if [[ $OCE_REPO != "$OCE_TEST_SOURCE" ]]; then
  mkdir -p "$OCE_REPO"
  cp -a "$OCE_TEST_SOURCE/." "$OCE_REPO/"
fi
''' if stage == 'pull-oce' else ''))
        return target / 'up.sh'

    def prepare_seed(self):
        (self.root / 'api.py').write_text(API)
        self.env['OCE_TEST_API'] = str(self.root / 'api.py')
        for name, value in {'namespace-id': 'ns_one\n', 'gateway-namespace': 'openshell-0123456789abcdef\n',
                            'installation.json': '{"drivers":{"compute":{"configuration":{"runtime":{}}}}}',
                            'values.json': '{}', 'kubeconfig': 'test'}.items():
            (self.state / name).write_text(value)
        (self.assets / 'operator-session.sh').write_text('''
request() { python3 "$OCE_TEST_API" "$@"; }
kube() { kubectl "$@"; }
upgrade_installation() {
  [[ $TEST_FAIL != provider ]]
  jq -e '.drivers.compute.configuration.runtime.modelProvider.baseUrl=="https://api.enmaas.devshift.net/v1"' "$INSTALLATION_FILE" >/dev/null
  touch "$RUN_DIR/provider-ready"
}
wait_deployment() {
  [[ $OPENSHELL_NAMESPACE == openshell-0123456789abcdef && $TENANT_NAMESPACE == tenant-one ]]
  [[ $agent == agt_one && $1 == rev_one ]]
  echo '["network-and-deployment"]' >> "$OCE_TEST_INFRA/calls"
  [[ $TEST_FAIL != network ]]
}
turn() {
  [[ $TEST_FAIL != probe ]]
  echo '{"outcome":{"status":"ok","terminalReply":{"text":"OCE_READY"}}}'
}
''')
        (self.assets / 'platform-kubernetes.sh').write_text('deployment_preflight() { [[ $ENVOY_IMAGE == envoy:pinned ]]; }\n')
        for name in ('request.mjs', 'kubernetes/service-policies.py', 'kubernetes/trusted-envoy-peers.sh'):
            (self.assets / name).touch()
        chat = self.oce / 'docs/assets/openshell-quickstart/chat.mjs'
        chat.parent.mkdir(parents=True); chat.touch()
        (self.assets / 'codex-configuration.jq').write_text('{kind:"agent",values:{agents:{defaults:{model:("codex/"+$model)}}}}')

    def test_up_orders_steps_and_resumes_without_reapplying_baseline(self):
        script = self.prepare_up()
        for _ in range(2):
            result = self.run_script(script)
            self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertEqual(sum(c[:2] == ['make', 'kind-up'] for c in calls), 1)
        self.assertEqual(sum(c == ['pull-oce'] for c in calls), 1)
        self.assertEqual(sum(c[:3] == ['kind', 'create', 'cluster'] for c in calls), 1)
        stages = [c[0] for c in calls if len(c) == 1]
        expected = ['prepare-oce', 'install-oce', 'activate-oce', 'browser-access', 'seed-agent', 'access']
        self.assertEqual(stages, ['prepare-oce', 'pull-oce', *expected[1:], *expected])
        self.assertEqual(self.state.stat().st_mode & 0o777, 0o700)
        self.assertIn(str(self.repo), (self.state / 'kind.yaml').read_text())

    def test_make_without_oce_checkout_fetches_assets_and_resumes(self):
        self.prepare_up()
        for file in ('Makefile', 'OPENSHELL_VERSION'):
            shutil.copyfile(helpers.parents[1] / file, self.repo / file)
        env = dict(self.env)
        env.pop('OCE_REPO')
        for _ in range(2):
            result = subprocess.run([shutil.which('make'), '--no-print-directory', 'kind-up-with-oce'],
                                    cwd=self.repo, env=env, capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.state / 'oce-source/deploy/helm/openclaw-enterprise/Chart.yaml').exists())
        self.assertEqual(self.calls().count(['pull-oce']), 1)

    def test_make_preserves_image_build_settings_and_extracts_helm_from_deployed_image(self):
        self.prepare_up()
        for file in ('Makefile', 'OPENSHELL_VERSION'):
            shutil.copyfile(helpers.parents[1] / file, self.repo / file)
        for settings, controller_image in (
            ({}, 'registry.example.com/controller:latest'),
            ({'LOCAL_IMAGES': 'false'}, 'registry.example.com/controller:latest'),
            ({'LOCAL_IMAGES': 'true', 'BUILD_SOURCE': 'baseline', 'KIND_SKIP_BUILD': 'true'},
             'localhost/hypershell-controller:dev'),
            ({'KIND_COVERAGE': 'true'}, 'localhost/hypershell-controller:dev'),
            ({'CONTROL_PLANE_IMAGE': 'registry.example.com/controller:custom'},
             'registry.example.com/controller:custom'),
        ):
            for source in ('environment', 'arguments'):
                with self.subTest(settings=settings, source=source):
                    (self.state / 'hypershell-ready').unlink(missing_ok=True)
                    (self.state / 'bin/helm').unlink(missing_ok=True)
                    env = dict(self.env, TEST_CONTROLLER_IMAGE=controller_image,
                               **{'TEST_EXPECT_' + key: value for key, value in settings.items()})
                    arguments = []
                    if source == 'environment':
                        env.update(settings)
                    else:
                        arguments = [key + '=' + value for key, value in settings.items()]
                    result = subprocess.run([shutil.which('make'), '--no-print-directory',
                                             'kind-up-with-oce', *arguments], cwd=self.repo,
                                            env=env, capture_output=True, text=True, timeout=20)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    creates = [c for c in self.calls() if c[:2] == ['docker', 'create']]
                    self.assertEqual(creates[-1], ['docker', 'create', controller_image])

    def test_up_stops_if_controller_image_cannot_be_read(self):
        script = self.prepare_up()
        result = self.run_script(script, TEST_FAIL='controller-image')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Helm preparation', result.stderr)
        self.assertFalse(any(c[:2] == ['docker', 'create'] for c in self.calls()))
        self.assertNotIn(['install-oce'], self.calls())

    def test_up_rejects_unrecorded_or_replaced_cluster_and_missing_inputs(self):
        script = self.prepare_up()
        (self.infra / 'cluster-exists').touch()
        result = self.run_script(script)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('unrecorded', result.stderr)
        (self.state / 'kubeconfig').write_text('private kubeconfig')
        (self.state / 'cluster-uid').write_text('old-cluster')
        result = self.run_script(script)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('identity differs', result.stderr)
        self.assertFalse(any(c[0] == 'make' for c in self.calls()))
        self.key.write_text('\n')
        result = self.run_script(script)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('nonempty API key', result.stderr)

    def test_up_stops_on_installation_failure(self):
        script = self.prepare_up()
        result = self.run_script(script, TEST_FAIL='install-oce')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(['activate-oce'], self.calls())
        self.assertNotIn(['access'], self.calls())
        self.assertIn('State preserved', result.stderr)

    def test_up_requires_gateway_prerequisites_before_seeding(self):
        script = self.prepare_up()
        result = self.run_script(script, TEST_FAIL='prepare-oce')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(c[:2] == ['make', 'kind-seed'] for c in self.calls()))
        self.assertNotIn(['install-oce'], self.calls())

    def test_existing_installation_preserves_baseline(self):
        script = self.prepare_up()
        (self.infra / 'cluster-exists').touch()
        (self.state / 'cluster-uid').write_text('cluster-one')
        (self.state / 'kubeconfig').write_text('private kubeconfig')
        (self.state / 'install-receipt').mkdir()
        result = self.run_script(script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(c[:2] == ['make', 'kind-up'] for c in self.calls()))
        self.assertEqual((self.state_home / 'oce-hypershell-kind-current').resolve(), self.state)
        # An explicit recovery selection is remembered even outside the managed directory.
        result = self.run_script(script, OCE_KIND_STATE='')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(c[:2] == ['make', 'kind-up'] for c in self.calls()))

    def test_automatic_state_survives_reruns_and_is_replaced_after_cluster_deletion(self):
        script = self.prepare_up()
        current = self.state_home / 'oce-hypershell-kind-current'
        result = self.run_script(script, OCE_KIND_STATE='')
        self.assertEqual(result.returncode, 0, result.stderr)
        first = current.resolve(strict=True)
        self.assertEqual(first.stat().st_mode & 0o777, 0o700)
        (first / 'credentials-to-preserve').write_text('old credentials')
        result = self.run_script(script, OCE_KIND_STATE='')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(current.resolve(), first)
        (self.infra / 'cluster-exists').unlink()
        result = self.run_script(script, OCE_KIND_STATE='', TEST_CLUSTER_UID='cluster-two')
        self.assertEqual(result.returncode, 0, result.stderr)
        second = current.resolve(strict=True)
        self.assertNotEqual(first, second)
        self.assertEqual((first / 'credentials-to-preserve').read_text(), 'old credentials')
        self.assertFalse((second / 'credentials-to-preserve').exists())
        self.assertEqual((second / 'cluster-uid').read_text().strip(), 'cluster-two')

    def test_stale_state_input_does_not_block_recreation_or_subsequent_runs(self):
        script = self.prepare_up()
        result = self.run_script(script)
        self.assertEqual(result.returncode, 0, result.stderr)
        (self.state / 'secrets').mkdir()
        (self.state / 'secrets/key').write_text('credentials from the deleted cluster')
        (self.infra / 'cluster-exists').unlink()
        # Old README exports or a previously copied make command can retain this path.
        for file in ('Makefile', 'OPENSHELL_VERSION'):
            shutil.copyfile(helpers.parents[1] / file, self.repo / file)
        for arguments in ([], ['OCE_KIND_STATE=' + str(self.state)]):
            result = subprocess.run([shutil.which('make'), '--no-print-directory', 'kind-up-with-oce',
                                     *arguments], cwd=self.repo,
                                    env=dict(self.env, TEST_CLUSTER_UID='cluster-two'),
                                    capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, 0, result.stderr)
        selected = (self.state_home / 'oce-hypershell-kind-current').resolve(strict=True)
        self.assertNotEqual(selected, self.state)
        self.assertEqual((selected / 'cluster-uid').read_text().strip(), 'cluster-two')
        self.assertFalse((selected / 'secrets/key').exists())
        self.assertEqual((self.state / 'secrets/key').read_text(), 'credentials from the deleted cluster')
        self.assertEqual((self.state / 'cluster-uid').read_text().strip(), 'cluster-one')

    def test_automatic_selection_discovers_legacy_state_by_live_identity(self):
        script = self.prepare_up()
        (self.infra / 'cluster-exists').touch()
        for suffix, uid in (('', 'old-cluster'), ('-manual', 'cluster-one')):
            legacy = self.state_home / ('oce-hypershell-kind' + suffix)
            legacy.mkdir()
            (legacy / 'cluster-uid').write_text(uid)
            (legacy / 'kubeconfig').write_text('saved kubeconfig')
        result = self.run_script(script, OCE_KIND_STATE='')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.state_home / 'oce-hypershell-kind-current').resolve(),
                         self.state_home / 'oce-hypershell-kind-manual')
        self.assertEqual((self.state_home / 'oce-hypershell-kind/cluster-uid').read_text(), 'old-cluster')

    def test_automatic_selection_does_not_guess_or_treat_failed_discovery_as_absence(self):
        script = self.prepare_up()
        (self.infra / 'cluster-exists').touch()
        for count in (0, 2):
            for number in range(count):
                candidate = self.state_home / f'oce-hypershell-kind-copy-{number}'
                candidate.mkdir()
                (candidate / 'cluster-uid').write_text('cluster-one')
                (candidate / 'kubeconfig').write_text('saved kubeconfig')
            result = self.run_script(script, OCE_KIND_STATE='')
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(f'Found {count} saved installations', result.stderr)
        result = self.run_script(script, OCE_KIND_STATE='', TEST_DISCOVERY_FAILURE='true')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(c[:2] == ['kind', 'create'] or c[0] == 'make' for c in self.calls()))
        self.assertFalse((self.state_home / 'oce-hypershell-kind-current').exists())

    def test_setup_lock_prevents_mutations(self):
        script = self.prepare_up()
        with (self.state_home / 'oce-hypershell-kind.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_script(script)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(c[0] == 'kind' for c in self.calls()))

    def test_seed_uses_provisioning_contract_and_reuses_resources(self):
        self.prepare_seed()
        for _ in range(2):
            result = self.run_script(helpers / 'seed-agent.sh')
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('Codex agent answered OCE_READY', result.stdout)
            self.assertNotIn(self.key.read_text().strip(), result.stdout + result.stderr)
        posts = [c for c in self.calls('api-calls') if c[0] == 'POST']
        self.assertEqual(len(posts), 3)
        self.assertEqual(posts[0][2]['value'], self.key.read_text().strip())
        self.assertEqual(posts[1][2]['config']['authorized_base_url'], 'https://api.enmaas.devshift.net/v1')
        self.assertEqual((self.state / 'codex-agent-id').read_text(), 'agt_one\n')
        self.assertEqual((self.state / 'codex-secret.json').stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.calls().count(['network-and-deployment']), 2)

    def test_seed_resumes_same_provision_request_after_lost_response(self):
        self.prepare_seed()
        first = self.run_script(helpers / 'seed-agent.sh', TEST_FAIL='lost-provision')
        self.assertNotEqual(first.returncode, 0)
        second = self.run_script(helpers / 'seed-agent.sh', TEST_FAIL='lost-provision')
        self.assertEqual(second.returncode, 0, second.stderr)
        posts = [c for c in self.calls('api-calls') if c[1].endswith('/agents/provision')]
        self.assertEqual(len(posts), 2)
        self.assertEqual(posts[0][2], posts[1][2])

    def test_seed_does_not_replay_uncertain_secret_write(self):
        self.prepare_seed()
        for _ in range(2):
            result = self.run_script(helpers / 'seed-agent.sh', TEST_FAIL='lost-secret')
            self.assertNotEqual(result.returncode, 0)
        self.assertIn('Unconfirmed creation', result.stderr)
        self.assertEqual(len([c for c in self.calls('api-calls') if c[0] == 'POST']), 1)

    def test_provider_failure_stops_before_credentials(self):
        self.prepare_seed()
        result = self.run_script(helpers / 'seed-agent.sh', TEST_FAIL='provider')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(c[0] == 'POST' for c in self.calls('api-calls')))
        self.assertNotIn('modelProvider', (self.state / 'installation.json').read_text())

    def test_deployment_network_and_probe_failures_never_report_success(self):
        self.prepare_seed()
        for failure in ('deployment', 'network', 'probe'):
            with self.subTest(failure=failure):
                result = self.run_script(helpers / 'seed-agent.sh', TEST_FAIL=failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('Codex agent answered', result.stdout)

    def test_access_reuses_verified_forward_and_prints_urls_not_passwords(self):
        for file, value in {'gateway-namespace': 'openshell-0123456789abcdef',
                            'namespace-id': 'ns_one', 'codex-agent-id': 'agt_one'}.items():
            (self.state / file).write_text(value + '\n')
        result = self.run_script(helpers / 'access.sh')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('https://console.hypershell.localhost:32443', result.stdout)
        self.assertIn('https://gw-openshell-0123456789abcdef.hypershell.localhost:32443', result.stdout)
        self.assertIn('/console/agents/agt_one?namespace=ns_one', result.stdout)
        self.assertIn('initial-admin-password', result.stdout)
        self.assertNotIn('port-forward', str(self.calls()))
        self.assertIn((self.state / 'access.txt').read_text(), result.stdout)
        policy = json.loads((self.state / 'kind-gateway-ingress.json').read_text())['spec']
        self.assertEqual(policy['podSelector'], {'matchLabels': {'app': 'gateway'}})
        self.assertEqual(policy['ingress'], [{'from': [{'ipBlock': {'cidr': '172.18.0.3/32'}}],
                                            'ports': [{'protocol': 'TCP', 'port': p} for p in (8080, 8090)]}])
        (self.state / 'access.txt').unlink()
        failed = self.run_script(helpers / 'access.sh', TEST_FAIL='health')
        self.assertNotEqual(failed.returncode, 0)
        self.assertNotIn('HyperShell + OCE are ready', failed.stdout)
        self.assertFalse((self.state / 'access.txt').exists())

    def test_access_colors_terminal_but_never_saved_summary(self):
        for file, value in {'gateway-namespace': 'openshell-0123456789abcdef',
                            'namespace-id': 'ns_one', 'codex-agent-id': 'agt_one',
                            'initial-admin-password': 'private-password-not-for-output'}.items():
            (self.state / file).write_text(value + '\n')
        for no_color in ('', '1'):
            with self.subTest(no_color=no_color):
                master, slave = pty.openpty()
                process = subprocess.Popen(['bash', str(helpers / 'access.sh')],
                                           env=dict(self.env, NO_COLOR=no_color),
                                           stdout=slave, stderr=slave)
                os.close(slave)
                output = bytearray()
                try:
                    while True:
                        self.assertTrue(select.select([master], [], [], 20)[0], 'access output timed out')
                        try:
                            chunk = os.read(master, 65536)
                        except OSError as error:
                            if error.errno != errno.EIO:
                                raise
                            break
                        if not chunk:
                            break
                        output.extend(chunk)
                    self.assertEqual(process.wait(timeout=5), 0, output.decode())
                finally:
                    os.close(master)
                    if process.poll() is None:
                        process.kill()
                        process.wait()
                terminal = output.decode().replace('\r\n', '\n')
                saved = (self.state / 'access.txt').read_text()
                self.assertEqual('\x1b[' in terminal, not no_color)
                self.assertNotIn('\x1b', saved)
                self.assertIn(saved, re.sub(r'\x1b\[[0-9;]*m', '', terminal))
                self.assertNotIn('private-password-not-for-output', terminal)
                for section in ('HyperShell', 'OpenShell gateway', 'OCE and Codex', 'Local access'):
                    self.assertIn('\n==> ' + section + '\n', saved)

    def test_make_exports_paths_with_spaces(self):
        target = self.repo / 'deploy/oce-kind'; target.mkdir(parents=True)
        (target / 'up.sh').write_text('set -eu\nprintf "%s\\n" "$OCE_REPO" "$ENMAAS_API_KEY_FILE" "$OCE_KIND_STATE"')
        shutil.copyfile(helpers.parents[1] / 'Makefile', self.repo / 'Makefile')
        shutil.copyfile(helpers.parents[1] / 'OPENSHELL_VERSION', self.repo / 'OPENSHELL_VERSION')
        result = subprocess.run([shutil.which('make'), '--no-print-directory', 'kind-up-with-oce',
                                 'OCE_REPO=' + str(self.oce), 'ENMAAS_API_KEY_FILE=' + str(self.key),
                                 'OCE_KIND_STATE=' + str(self.state)], cwd=self.repo,
                                capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), list(map(str, (self.oce, self.key, self.state))))


unittest.main()
PY
