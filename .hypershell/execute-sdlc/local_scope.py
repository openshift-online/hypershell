#!/usr/bin/env python3
"""Require an existing GitHub scope label before starting local workers."""
import json
import os
import subprocess
import sys
from urllib.parse import quote


def main():
    scope = sys.argv[1]
    repo = os.environ.get('REPOSITORY', 'openshift-online/hypershell')
    endpoint = f'repos/{repo}/labels/{quote(scope, safe="")}'
    try:
        result = subprocess.run(['gh', 'api', endpoint], capture_output=True,
                                text=True, timeout=30, check=True)
        if json.loads(result.stdout).get('name') != scope:
            raise ValueError('GitHub returned a different label')
    except (OSError, subprocess.SubprocessError, ValueError, AttributeError):
        print(f'error: cannot verify registered scope {scope!r} in {repo}.\n'
              'Sign in with gh auth login and confirm repository access.\n'
              'The exact scope label must already exist in the repository; ask your\n'
              'maintainer to register it or choose an existing scope with SCOPE=...\n'
              'No dispatcher or workers were started.', file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main())
