"""Exercise real Git histories through the CI detector, including speculative PRs."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DetectComponentsTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        # Hooks export GIT_DIR/GIT_INDEX_FILE for the parent repository. Never
        # let fixture commands inherit them, even with cwd set to a temp repo.
        self.env = {k: v for k, v in os.environ.items() if not k.startswith('GIT_')}
        (self.repo / '.github/scripts').mkdir(parents=True)
        for path in ('.github/scripts/detect-components.sh', '.github/component-paths.json'):
            shutil.copy(ROOT / path, self.repo / path)
        self.git('init', '-q', '-b', 'main')
        self.git('config', 'user.email', 'test@example.invalid')
        self.git('config', 'user.name', 'Detector test')
        self.git('config', 'commit.gpgsign', 'false')
        self.git('config', 'core.hooksPath', '/dev/null')
        self.base = self.commit('README.md', 'base\n')

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.repo, env=self.env, text=True, stderr=subprocess.PIPE).strip()

    def commit(self, path, text):
        target = self.repo / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text)
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')
        return self.git('rev-parse', 'HEAD')

    def detect(self, event='pull_request', payload=None, files=None, success=True):
        event_path = self.repo / 'event.json'
        event_path.write_text(json.dumps(payload or {'pull_request': {
            'base': {'sha': self.base}, 'head': {'sha': self.git('rev-parse', 'HEAD')}
        }}))
        env = {k: v for k, v in self.env.items()
               if k not in ('CHANGED_FILES', 'GITHUB_OUTPUT', 'GITHUB_EVENT_NAME', 'GITHUB_EVENT_PATH')}
        env.update(GITHUB_EVENT_NAME=event, GITHUB_EVENT_PATH=str(event_path))
        if files is not None:
            env['CHANGED_FILES'] = '\n'.join(files)
        result = subprocess.run(['bash', '.github/scripts/detect-components.sh'],
                                cwd=self.repo, env=env, text=True, capture_output=True)
        if not success:
            self.assertNotEqual(0, result.returncode)
            self.assertEqual('', result.stdout)
            return
        self.assertEqual(0, result.returncode, result.stderr)
        return {line.split('=')[0] for line in result.stdout.splitlines() if line.endswith('=true')}

    # Verifies: CICD-MRG-01.02, CICD-MRG-09
    def test_speculative_pr_includes_preceding_pr_and_excludes_base_drift(self):
        self.git('checkout', '-qb', 'mergify/merge-queue/main/pr-2')
        self.commit('components/api-server/openapi/openapi.yaml', 'contract\n')
        head = self.commit('components/fleet-dashboard/src/app.ts', 'fleet\n')
        self.git('checkout', 'main')
        main = self.commit('components/pr-test/unrelated.sh', 'base drift\n')
        self.git('checkout', 'mergify/merge-queue/main/pr-2')
        selected = self.detect(payload={'pull_request': {'base': {'sha': main}, 'head': {'sha': head}}})
        self.assertEqual({'api_server', 'cli', 'control_plane', 'sdk_go',
                          'sdk_typescript', 'web_console', 'agents', 'fleet_dashboard'}, selected)

    def test_cross_component_rename_tests_both_ends(self):
        self.base = self.commit('components/api-server/old.go', 'same content\n')
        self.git('checkout', '-qb', 'feature/rename')
        (self.repo / 'components/cli').mkdir(parents=True)
        self.git('mv', 'components/api-server/old.go', 'components/cli/new.go')
        self.git('commit', '-qm', 'rename')
        self.assertEqual({'api_server', 'cli', 'control_plane', 'agents'}, self.detect())

    def test_missing_sha_fails_instead_of_skipping_ci(self):
        self.detect(payload={'pull_request': {'base': {'sha': 'deadbeef'},
                                             'head': {'sha': self.base}}}, success=False)

    def test_invalid_metadata_fails_instead_of_skipping_ci(self):
        (self.repo / '.github/component-paths.json').write_text('{')
        self.detect(success=False)

    def test_docs_only(self):
        self.assertEqual(set(), self.detect(files=['docs/guide.md']))

    def test_each_component_selects_itself(self):
        config = json.loads((ROOT / '.github/component-paths.json').read_text())
        for name, entry in config.items():
            with self.subTest(component=name):
                self.assertIn(name, self.detect(files=[entry['directory'] + '/source.txt']))

    def test_shared_dependencies(self):
        cases = {
            'components/cli/pkg/kustomize/x.go': {'cli', 'control_plane', 'agents'},
            'components/api-server/pkg/secretstore/x.go': {'api_server', 'control_plane'},
            'scripts/openapi-ir/main.py': {'sdk_go', 'sdk_typescript', 'web_console'},
            'components/web-console/domain-probes/src/index.ts':
                {'web_console', 'gateway_management_ui', 'operational_dashboard_ui'},
            'scripts/generate-cli.sh': {'cli', 'control_plane', 'agents'},
            'components/sdk-typescript/src/client.ts': {'sdk_typescript', 'web_console'},
        }
        for path, expected in cases.items():
            with self.subTest(path=path):
                self.assertEqual(expected, self.detect(files=[path]))

    def test_dispatch_selects_every_component(self):
        config = json.loads((ROOT / '.github/component-paths.json').read_text())
        self.assertEqual(set(config) | {'bash_tests'}, self.detect(event='workflow_dispatch'))

    def test_empty_diff_selects_nothing(self):
        self.assertEqual(set(), self.detect())

    def test_push_and_merge_group_cover_cumulative_changes(self):
        self.commit('components/api-server/internal/a.go', 'api\n')
        head = self.commit('components/cli/cmd/a.go', 'cli\n')
        expected = {'api_server', 'cli', 'control_plane', 'agents'}
        self.assertEqual(expected, self.detect(event='push', payload={'before': self.base}))
        self.assertEqual(expected, self.detect(event='merge_group', payload={
            'merge_group': {'base_sha': self.base, 'head_sha': head}}))


if __name__ == '__main__':
    unittest.main()
