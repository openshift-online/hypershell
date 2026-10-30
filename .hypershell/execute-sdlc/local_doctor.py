#!/usr/bin/env python3
"""Check host setup without launching agents, opening browsers, or writing issues."""
import json
import os
from pathlib import Path
import shutil
import sys

from funnel_dispatch import DispatchError, prepare_inference
from local_harness import yaml_module
from jira_auth import JiraAuthError, read_keyring, selected_auth


def main():
    print('FULLSEND / Setup check')
    print(os.environ.get('FULLSEND_CHECK_SCOPE') or os.environ.get('FULLSEND_SCOPE', 'Scope not configured'))
    print()
    tools = ('go', 'git', 'python3', 'gh', 'fullsend', 'openshell', 'jq')
    missing = [name for name in tools if not shutil.which(name)]
    failures = len(missing)
    if missing:
        print('Tools    Needs setup: install ' + ', '.join(missing))
    else:
        print('Tools    Ready: Go, Git, Python, GitHub CLI, Fullsend, OpenShell, jq')
    try:
        fullsend_dir = Path(os.environ.get('FULLSEND_DIR', '.fullsend'))
        if not (fullsend_dir/'harness').is_dir():
            raise DispatchError('Fullsend harness directory is missing; run from the repository worktree or set FULLSEND_DIR')
        prepare_inference(fullsend_dir)
        path = os.environ.get('GOOGLE_APPLICATION_CREDENTIALS')
        if path:
            try:
                value = json.loads(Path(path).read_text())
            except (OSError, ValueError):
                raise DispatchError('ADC file is not readable JSON; run gcloud auth application-default login') from None
            if not isinstance(value, dict) or not value.get('type'):
                raise DispatchError('ADC file has no credential type; use a Google ADC file')
        print('Vertex   Ready: project, region, and credential file configured')
    except DispatchError as exc:
        print(f'Vertex   Needs setup: {exc}')
        failures += 1
    if os.environ.get('FULLSEND_LIFECYCLE', 'jira') == 'github-only':
        print('Jira     Disabled: GitHub-only lifecycle selected')
    else:
        try:
            yaml_module()
            auth = selected_auth(os.environ.get('JIRA_AUTH_MODE', 'auto'))
            if auth['mode'] == 'acli':
                if not shutil.which('acli'):
                    raise JiraAuthError('acli is missing; install ACLI and run acli jira auth login --web')
                read_keyring(auth)  # Verify compatibility without displaying any token.
            print(f'Jira     Ready: {auth["mode"]} at {auth["site_url"]}')
        except (JiraAuthError, ValueError) as exc:
            print(f'Jira     Needs setup: {exc}')
            failures += 1
    if shutil.which('hsctl'):
        print('Gateway  hsctl installed; connect from the dashboard with s')
    else:
        print('Gateway  hsctl not installed; dashboard setup can offer a download')
    print('\nNot tested: Vertex model access, API permissions, and gateway connectivity.')
    if failures:
        print('\nSetup needs attention. Fix the items above, then run make fullsend-doctor again.')
        print('Setup guide: docs/development/fullsend.md')
    else:
        print('\nLocal setup passed. Next: make fullsend')
    return 1 if failures else 0


if __name__ == '__main__':
    sys.exit(main())
