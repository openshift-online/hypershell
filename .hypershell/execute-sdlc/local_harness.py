"""Select runtime configuration without changing shared agent definitions."""
import shutil
import sys
import tempfile
from pathlib import Path


def prepare(directory, agent):
    directory = Path(directory)
    override = directory / 'harness' / f'local-{agent}.yaml'
    if agent not in ('ingest', 'reconcile') or not override.is_file():
        raise ValueError(f'Missing local harness for {agent}')
    target = Path(tempfile.mkdtemp(prefix='funnel-harness-'))
    try:
        shutil.copytree(directory, target, dirs_exist_ok=True)
        shutil.copyfile(override, target / 'harness' / f'{agent}.yaml')
        return str(target)
    except BaseException:
        shutil.rmtree(target)
        raise


if __name__ == '__main__':
    print(prepare(sys.argv[1], sys.argv[2]))
