"""Apply local transport and scheduling overrides to canonical harnesses."""
import shutil
import sys
import tempfile
from pathlib import Path


def yaml_module():
    try:
        import yaml
    except ImportError:
        raise ValueError('Local Jira lifecycle needs PyYAML; install python3-yaml or python3-pyyaml') from None
    return yaml


def prepare(directory, agent):
    yaml = yaml_module()
    directory = Path(directory)
    source = directory / 'harness' / f'{agent}.yaml'
    if agent not in ('ingest', 'reconcile') or not source.is_file():
        raise ValueError(f'Missing canonical harness for {agent}')
    harness = yaml.safe_load(source.read_text())
    # The coordinator owns scheduling; all task settings remain canonical.
    harness['trigger'] = 'false'
    providers = harness.setdefault('providers', [])
    if 'providers/local-dispatch-jira.yaml' not in providers:
        providers.append('providers/local-dispatch-jira.yaml')
    profiles = harness.setdefault('openshell', {}).setdefault('profiles', [])
    if 'profiles/local-dispatch-jira.yaml' not in profiles:
        profiles.append('profiles/local-dispatch-jira.yaml')
    harness.setdefault('host_files', []).append({
        'src': '${LOCAL_DISPATCH_JIRA_CREDENTIALS_FILE}',
        'dest': '/tmp/.local-dispatch-jira.json',
    })
    sandbox = harness.setdefault('env', {}).setdefault('sandbox', {})
    sandbox.pop('JIRA_EMAIL', None)
    sandbox.pop('JIRA_API_TOKEN', None)
    sandbox.update(JIRA_BASE_URL='${JIRA_BASE_URL}',
                   JIRA_CREDENTIALS_FILE='/tmp/.local-dispatch-jira.json')
    target = Path(tempfile.mkdtemp(prefix='local-dispatch-harness-'))
    try:
        shutil.copytree(directory, target, dirs_exist_ok=True)
        (target / 'harness' / f'{agent}.yaml').write_text(
            yaml.safe_dump(harness, sort_keys=False))
        return str(target)
    except BaseException:
        shutil.rmtree(target)
        raise


if __name__ == '__main__':
    print(prepare(sys.argv[1], sys.argv[2]))
