#!/usr/bin/env bash
# Manual runbook steps must stop on failure without exiting the interactive parent shell.
set -euo pipefail
python3 - "$(dirname -- "${BASH_SOURCE[0]}")/manual-setup.md" <<'PY'
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile
import unittest

helpers = Path(sys.argv.pop()).resolve().parent
readme = (helpers / 'manual-setup.md').read_text()
section = readme.split('## Build and load OCE\n', 1)[1].split('\n## ', 1)[0]
block = re.search(r'```bash\n(.*?)\n```', section, re.S).group(1)


class BuildShellTest(unittest.TestCase):
    def test_build_success_and_failures_preserve_parent_shell(self):
        for failure in ('go', 'docker-save', ''):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as directory:
                state = Path(directory)
                bins, oce, repo = [state / name for name in ('bin', 'oce source', 'hypershell')]
                for path in (bins, oce, repo):
                    path.mkdir()
                stub = bins / 'stub'
                stub.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
name = Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ['OCE_TEST_LOG'], 'a') as output:
    output.write(json.dumps([name, *args]) + '\\n')
failure = os.environ['OCE_TEST_FAILURE']
if (failure == 'go' and name == 'go') or (failure == 'docker-save' and name == 'docker' and args[0] == 'save'):
    print('simulated build failure', file=sys.stderr)
    sys.exit(42)
if name == 'jq':
    print('example.com/node:test' if args[0] == '-er' else '{}')
elif name == 'node' and args[0].endswith('openclaw-source.mjs'):
    print('OPENCLAW_COMMIT=test')
elif name == 'docker' and 'import' in args:
    sys.stdin.read()
elif name == 'docker' and 'ls' in args:
    for role in ('controller', 'runtime'):
        print('localhost/oce-hypershell/' + role + ':local unused sha256:' + 'a' * 64)
''')
                stub.chmod(0o700)
                for command in ('go', 'node', 'jq', 'docker'):
                    (bins / command).symlink_to(stub)
                build = repo / 'deploy/oce-kind/build-oce.sh'
                build.parent.mkdir(parents=True)
                build.write_text((helpers / 'build-oce.sh').read_text())
                log = state / 'commands.jsonl'
                env = dict(os.environ, PATH=str(bins) + os.pathsep + os.environ['PATH'],
                           OCE_REPO=str(oce), HYPERSHELL_REPO=str(repo), OCE_KIND_STATE=str(state),
                           OCE_TEST_LOG=str(log), OCE_TEST_FAILURE=failure)
                result = subprocess.run(['bash', '--noprofile', '--norc', '-ic',
                                         'set -euo pipefail\n' + block + '\necho PARENT_SURVIVED\npwd'],
                                        cwd=repo, env=env, capture_output=True, text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('PARENT_SURVIVED\n' + str(repo), result.stdout)
                commands = [json.loads(line) for line in log.read_text().splitlines()]
                if failure:
                    self.assertIn('simulated build failure', result.stderr)
                    self.assertIn('Do not continue', result.stderr)
                    self.assertFalse((state / 'images.json').exists())
                    if failure == 'go':
                        self.assertEqual([command[0] for command in commands], ['go'])
                    else:
                        self.assertFalse(any('ls' in command or 'tag' in command for command in commands))
                else:
                    self.assertEqual(json.loads((state / 'images.json').read_text()), {})
                    for role in ('controller', 'runtime'):
                        self.assertEqual((state / (role + '-image')).read_text().strip(),
                                         'localhost/oce-hypershell/' + role + '@sha256:' + 'a' * 64)


class WorkflowShellTest(unittest.TestCase):
    def test_enmaas_registration_requires_provider_and_resumes_saved_secret(self):
        section = readme.split('#### 2. Register the EnMaaS API key\n', 1)[1]
        script = re.search(r'```bash\n(.*?)\n```', section, re.S).group(1)
        # Supply terminal input from a fixture while exercising the pasted block.
        script = script.replace('< /dev/tty', '< "$OCE_KIND_STATE/key-input"')
        for scenario in ('new', 'empty', 'unconfigured', 'resume', 'stale'):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                helper = root / 'docs/snippets/openshell-quickstart/operator-session.sh'
                helper.parent.mkdir(parents=True)
                helper.write_text('''request() {
  test "$RUN_DIR" = "$OCE_KIND_STATE"
  test "$KUBECONFIG" = "$RUN_DIR/kubeconfig"
  test "$CONTEXT" = kind-hypershell-oce
  test "$PLATFORM_NS" = oce-system
  test "$RELEASE" = oce
  test "$OCC_SERVICE_KEY_FILE" = "$RUN_DIR/initial-admin-service-key.json"
  if [[ $1 == GET && $2 == /namespaces/test-namespace/secrets/test-secret-id ]]; then
    cat "$RUN_DIR/live-secret.json"
    return
  fi
  jq -c . >> "$RUN_DIR/requests.jsonl"
  case "$2" in
    */secrets) cat "$RUN_DIR/live-secret.json" ;;
    */credential-sources) printf '%s\\n' '{"data":{"state":"ready","id":"test-source-id"}}' ;;
    *) return 1 ;;
  esac
}
''')
                (root / 'namespace-id').write_text('test-namespace\n')
                key = '' if scenario == 'empty' else 'test-enmaas-"quoted"-\\-$key'
                if scenario in ('new', 'empty'):
                    (root / 'key-input').write_text(key + '\n')
                url = 'https://api.enmaas.devshift.net/v1'
                provider = {'protocol': 'openai-responses', 'baseUrl': url}
                (root / 'installation.json').write_text(json.dumps({
                    'drivers': {'compute': {'configuration': {'runtime': {
                        'modelProvider': None if scenario == 'unconfigured' else provider,
                    }}}},
                }))
                secret = {'data': {'id': 'test-secret-id', 'namespaceId': 'test-namespace',
                                   'name': 'enmaas-api-key', 'ref': 'test-secret-ref'}}
                (root / 'live-secret.json').write_text(json.dumps(secret))
                if scenario in ('resume', 'stale'):
                    if scenario == 'stale':
                        secret['data']['ref'] = 'stale-secret-ref'
                    (root / 'enmaas-secret.json').write_text(json.dumps(secret))
                env = dict(os.environ, OCE_REPO=directory, OCE_KIND_STATE=directory)
                # A new terminal retains only the documented checkout and state inputs.
                for name in ('RUN_DIR', 'KUBECONFIG', 'CONTEXT', 'PLATFORM_NS', 'RELEASE',
                             'OCC_SERVICE_KEY_FILE', 'MODEL_BASE_URL', 'ENMAAS_API_KEY',
                             'namespace', 'base', 'modelSource'):
                    env.pop(name, None)
                result = subprocess.run(['bash', '--noprofile', '--norc', '-c',
                                         'set -euo pipefail\n' + script +
                                         '\necho PARENT_SURVIVED\n'
                                         'printf "%s\\n" "${modelSource:-}" "${ENMAAS_API_KEY:-}"'],
                                        env=env, capture_output=True, text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('PARENT_SURVIVED', result.stdout)
                self.assertNotIn('unbound variable', result.stderr)
                requests = root / 'requests.jsonl'
                if scenario in ('new', 'resume'):
                    expected = ([{'name': 'enmaas-api-key', 'value': key}]
                                if scenario == 'new' else [])
                    expected.append({'name': 'enmaas', 'type': 'openai',
                                     'config': {'authorized_base_url': url},
                                     'secrets': {'api_key': 'test-secret-ref'}})
                    self.assertEqual([json.loads(line) for line in requests.read_text().splitlines()],
                                     expected)
                    self.assertIn('PARENT_SURVIVED\ntest-source-id\n\n', result.stdout)
                    self.assertNotIn(key, result.stdout + result.stderr)
                else:
                    self.assertFalse(requests.exists())
                    self.assertIn('Model credential registration failed', result.stderr)
                    if scenario == 'empty':
                        self.assertIn('An EnMaaS API key is required', result.stderr)
                    elif scenario == 'unconfigured':
                        self.assertIn('Complete step 1', result.stderr)

    def test_all_blocks_have_valid_bash_syntax(self):
        docs = readme + (helpers / 'README.md').read_text()
        for index, script in enumerate(re.findall(r'```(?:bash|shell)\n(.*?)\n```', docs, re.S)):
            with self.subTest(block=index):
                subprocess.run(['bash', '-n'], input=script, text=True, check=True)

    def test_local_documentation_links_resolve(self):
        for name in ('README.md', 'manual-setup.md'):
            for target in re.findall(r'\]\(([^)]+)\)', (helpers / name).read_text()):
                if '://' not in target:
                    with self.subTest(document=name, link=target):
                        self.assertTrue((helpers / target.split('#')[0]).exists())

    def test_every_block_stops_child_failure_without_exiting_parent(self):
        for index, script in enumerate(re.findall(r'```bash\n(.*?)\n```', readme, re.S)):
            with self.subTest(block=index), tempfile.TemporaryDirectory() as directory:
                child = re.search(r"(if bash -euo pipefail <<'([A-Z_]+)'\n)(.*?)(\n\2\n)", script, re.S)
                self.assertIsNotNone(child, 'every operational block needs a guarded child shell')
                script = script[:child.start(3)] + 'echo STEP_FAILED >&2\nfalse\necho CHILD_CONTINUED' + script[child.end(3):]
                script = script.replace('/absolute/path/to/hypershell', shlex.quote(directory))
                script = script.replace('/absolute/path/to/openclaw-enterprise', shlex.quote(directory))
                script = script.replace('"$HOME/.local/state/oce-hypershell-kind"', shlex.quote(directory))
                env = dict(os.environ, OCE_KIND_STATE=directory, OCE_REPO=directory,
                           HYPERSHELL_REPO=directory, RUN_DIR=directory)
                result = subprocess.run(['bash', '--noprofile', '--norc', '-ic',
                                         'set -euo pipefail\n' + script + '\necho PARENT_SURVIVED'],
                                        env=env, capture_output=True, text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('STEP_FAILED', result.stderr)
                self.assertIn('PARENT_SURVIVED', result.stdout)
                self.assertNotIn('CHILD_CONTINUED', result.stdout)

    def test_prerequisites_disable_interactive_exit_options(self):
        script = re.search(r'```bash\n(.*?)\n```', readme, re.S).group(1)
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            script = script.replace('/absolute/path/to/hypershell', shlex.quote(directory))
            script = script.replace('/absolute/path/to/openclaw-enterprise', shlex.quote(directory))
            script = script.replace('"$HOME/.local/state/oce-hypershell-kind"', shlex.quote(directory))
            result = subprocess.run(['bash', '--noprofile', '--norc', '-ic',
                                     'set -euo pipefail\n' + script +
                                     '\nif [[ $- == *e* || $- == *u* ]]; then exit 99; fi\nfalse\necho PARENT_SURVIVED'],
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('PARENT_SURVIVED', result.stdout)
            self.assertEqual((state / 'curl/.curlrc').read_text(), 'ipv4\n')
            self.assertEqual((state.stat().st_mode & 0o777), 0o700)

    def test_gateway_results_are_exported_to_later_steps(self):
        section = readme.split('## Configure the HyperShell-owned gateway\n', 1)[1]
        script = re.search(r'```bash\n(.*?)\n```', section, re.S).group(1)
        child = re.search(r"(if bash -euo pipefail <<'CONFIGURE_GATEWAY'\n)(.*?)(\nCONFIGURE_GATEWAY\n)", script, re.S)
        results = '''printf '%s\\n' openshell-0123456789abcdef > "$OCE_KIND_STATE/gateway-namespace"
printf '%s\\n' keycloak.localhost:8443 > "$OCE_KIND_STATE/keycloak-host"'''
        script = script[:child.start(2)] + results + script[child.end(2):]
        with tempfile.TemporaryDirectory() as directory:
            env = dict(os.environ, OCE_KIND_STATE=directory)
            result = subprocess.run(['bash', '--noprofile', '--norc', '-ic',
                                     'set -euo pipefail\n' + script +
                                     '\nprintenv GATEWAY_NAMESPACE keycloak_host'],
                                    env=env, capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, 'openshell-0123456789abcdef\nkeycloak.localhost:8443\n')

    def test_installation_stops_at_each_failed_helper_without_exiting_parent(self):
        section = readme.split('Run the supplied helpers in order:', 1)[1]
        script = re.search(r'```bash\n(.*?)\n```', section, re.S).group(1)
        stages = ('prepare-oce', 'install-oce', 'activate-oce', 'browser-access')
        for failure in (*stages, ''):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                helpers = root / 'deploy/oce-kind'
                helpers.mkdir(parents=True)
                for stage in stages:
                    (helpers / (stage + '.sh')).write_text(
                        'set -euo pipefail\n'
                        f'echo {stage}-start >> "$OCE_TEST_LOG"\n'
                        f'if [[ $OCE_TEST_FAILURE == {stage} ]]; then false; fi\n'
                        'printf "%s\\n" openshell-0123456789abcdef > "$OCE_KIND_STATE/gateway-namespace"\n'
                        f'echo {stage}-done >> "$OCE_TEST_LOG"\n')
                log = root / 'commands'
                env = dict(os.environ, OCE_TEST_LOG=str(log), OCE_TEST_FAILURE=failure,
                           OCE_KIND_STATE=str(root))
                env.pop('GATEWAY_NAMESPACE', None)
                result = subprocess.run(['bash', '--noprofile', '--norc', '-ic',
                                         'set -euo pipefail\n' + script +
                                         '\necho PARENT_SURVIVED\nprintf "%s\\n" "${GATEWAY_NAMESPACE:-}"'],
                                        cwd=root, env=env, capture_output=True, text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('PARENT_SURVIVED', result.stdout)
                expected = []
                for stage in stages:
                    expected.append(stage + '-start')
                    if stage == failure:
                        break
                    expected.append(stage + '-done')
                self.assertEqual(log.read_text().splitlines(), expected)
                if failure:
                    self.assertIn(failure + '.sh', result.stderr)
                    self.assertIn('Do not continue', result.stderr)
                else:
                    self.assertIn('PARENT_SURVIVED\nopenshell-0123456789abcdef', result.stdout)


unittest.main()
PY
