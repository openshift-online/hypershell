#!/usr/bin/env bash
# Exercise Fullsend's actual skipped/reason parser. All external CLIs are stubs.
set -euo pipefail
python3 - "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)" <<'PY'
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

root = Path(sys.argv[1])
fullsend = shutil.which('fullsend')
if not fullsend:
    print('SKIP: Fullsend unavailable; native skip protocol was not exercised')
    sys.exit(0)
with tempfile.TemporaryDirectory() as directory:
    path = Path(directory)
    (path/'bin').mkdir()
    (path/'harness').mkdir()
    for name, body in {
        'openshell': 'echo "openshell $*" >> "$MOCK_CALLS"\ncase "$*" in *"sandbox create"*) exit 97;; esac\necho "{}"\n',
        'gh': ('echo "gh $*" >> "$MOCK_CALLS"\n'
               'case "$*" in *issues/1/labels*) if [ "${MOCK_AUTHORIZED:-0}" = 1 ]; then printf "agent/allowed\\nagent/scope/test\\nagent/reviewable-code\\n"; fi;; '
               '*pulls/5*) echo abc1234;; esac\n'),
    }.items():
        stub = path/'bin'/name
        stub.write_text('#!/bin/sh\n'+body)
        stub.chmod(0o755)
    (path/'agent.md').write_text('---\nname: skip-check\n---\nMust never run.\n')
    wrapper = path/'pre-wrapper.sh'
    wrapper.write_text('#!/bin/sh\nbash "'+str(root/'scripts/pre-lifecycle.sh')+'"\nrc=$?\n'
                       'if [ "$rc" != 0 ]; then exit "$rc"; fi\n'
                       'if [ "${MOCK_AUTHORIZED:-0}" = 1 ]; then exit 89; fi\nexit 0\n')
    wrapper.chmod(0o755)
    harness = dict(agent='agent.md',role='triage',slug='skip-check',image='unused:never',
                   policy=str(root/'policies/base.yaml'),model='claude-sonnet-5',
                   pre_script=str(wrapper),
                   env={'runner':dict(PHASE='code-review',GH_TOKEN='fixture',
                       ISSUE_URL='https://github.com/test/repo/issues/1',LABEL_SCOPE='agent/scope/test',PR_NUMBER='5')})
    (path/'harness/skip-check.yaml').write_text(json.dumps(harness))
    (path/'config.yaml').write_text(json.dumps(dict(version='1',agents=[dict(name='skip-check',source='harness/skip-check.yaml')])))
    env = dict(os.environ,PATH=str(path/'bin')+':/usr/bin:/bin',MOCK_CALLS=str(path/'calls'),
               HOME=str(path),NO_COLOR='1',GH_TOKEN='fixture')
    result = subprocess.run([fullsend,'run','skip-check','--fullsend-dir',str(path),
                             '--target-repo',str(path),'--output-dir',str(path/'runs')],
                            env=env,capture_output=True,text=True,timeout=30)
    output = result.stdout+result.stderr
    assert result.returncode == 0, output
    assert 'Run skipped by pre-script: agent/allowed is not present' in output, output
    assert 'sandbox create' not in (path/'calls').read_text(), output
    print('PASS: native Fullsend honors skipped=true and reason; no sandbox or agent started')

    # Now allow the real precondition through, then abort in the test wrapper
    # before sandbox creation. The native CLI still supplies the real run root.
    env['MOCK_AUTHORIZED'] = '1'
    permitted = subprocess.run([fullsend,'run','skip-check','--fullsend-dir',str(path),
                                '--target-repo',str(path),'--output-dir',str(path/'runs')],
                               env=env,capture_output=True,text=True,timeout=30)
    assert permitted.returncode != 0, 'test wrapper must abort before sandbox creation'
    states = list((path/'runs').rglob('.lifecycle-state.json'))
    assert len(states) == 1, permitted.stdout+permitted.stderr
    state = json.loads(states[0].read_text())
    assert state['head'] == 'abc1234' and state['pr'] == '5', state
    assert state['phase'] == 'code-review' and state['issue'] == 1, state
    assert states[0].stat().st_mode & 0o777 == 0o600
    print('PASS: native Fullsend places private pre-script state in its host run directory')
PY
