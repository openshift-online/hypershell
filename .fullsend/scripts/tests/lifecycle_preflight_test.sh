#!/usr/bin/env bash
# Validate review preflight through the installed Fullsend environment loader.
# OpenShell is replaced with a failing stub: no gateway or sandbox can be used.
set -euo pipefail
python3 - "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)" <<'PY'
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

root = Path(sys.argv[1])
fullsend = shutil.which('fullsend')
if not fullsend:
    print('SKIP: installed Fullsend is unavailable; native environment loading was not tested')
    sys.exit(0)
with tempfile.TemporaryDirectory() as directory:
    tmp = Path(directory)
    (tmp/'bin').mkdir()
    stub = tmp/'bin/openshell'
    stub.write_text('#!/bin/sh\necho "test: OpenShell disabled" >&2\nexit 1\n')
    stub.chmod(0o755)
    adc = tmp/'adc.json'
    adc.write_text('{}')
    for phase in ('code-review','spec-review'):
        text = (root/f'harness/{phase}.yaml').read_text()
        env = {'PATH':str(tmp/'bin')+os.pathsep+os.environ['PATH'], 'HOME':str(tmp),
               'NO_COLOR':'1'}
        for key in re.findall(r'\$\{([A-Z][A-Z0-9_]*)\}', text):
            env[key] = 'fixture'
        env.update(GOOGLE_APPLICATION_CREDENTIALS=str(adc),
                   FULLSEND_WORK_ITEM_URL='https://github.com/test/repo/issues/1',
                   REPOSITORY='test/repo', ISSUE_NUMBER='1', PR_NUMBER='5',
                   PR_URL='https://github.com/test/repo/pull/5', DRY_RUN='true')
        result = subprocess.run([fullsend,'run',phase,'--fullsend-dir',str(root),
                                 '--target-repo',str(tmp)],env=env,capture_output=True,text=True,timeout=30)
        output = result.stdout+result.stderr
        assert result.returncode != 0, 'OpenShell stub must prevent execution'
        assert 'Environment validation failed' not in output, output
        assert 'unresolved host variable' not in output, output
        assert 'missing is not set' not in output, output
        assert 'Harness loaded' in output, output
        assert 'OpenShell disabled' in output or 'openshell available' in output.lower() or 'openshell' in output.lower(), output
        print(f'PASS: {phase} loads through native Fullsend environment validation; gateway execution blocked')
PY
