#!/usr/bin/env python3
"""Private host-side handoff between independent native Fullsend hook processes."""
import argparse
import json
import os
from pathlib import Path
import re
import stat
import tempfile


def load(path, phase, repo, issue):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o077:
        raise ValueError('state must be an owned private regular file')
    data = json.loads(path.read_text())
    if (data.get('version'), data.get('phase'), data.get('repo'), data.get('issue')) != (1, phase, repo, issue):
        raise ValueError('state belongs to a different phase or execution record')
    for key in ('pr', 'head'):
        value = data.get(key)
        if not isinstance(value, str) or (value and not re.fullmatch(r'[1-9][0-9]*' if key == 'pr' else r'[0-9a-fA-F]{7,64}', value)):
            raise ValueError('invalid trusted PR identity')
    return data


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('write', 'read'))
    parser.add_argument('path', type=Path)
    parser.add_argument('phase')
    parser.add_argument('repo')
    parser.add_argument('issue', type=int)
    parser.add_argument('pr', nargs='?', default='')
    parser.add_argument('head', nargs='?', default='')
    args = parser.parse_args()
    if args.action == 'write':
        data = dict(version=1, phase=args.phase, repo=args.repo, issue=args.issue,
                    pr=args.pr, head=args.head)
        fd, temporary = tempfile.mkstemp(prefix='.lifecycle-state-', dir=args.path.parent)
        try:
            with os.fdopen(fd, 'w') as stream:
                json.dump(data, stream)
            load(Path(temporary), args.phase, args.repo, args.issue)
            os.replace(temporary, args.path)
        finally:
            Path(temporary).unlink(missing_ok=True)
    else:
        data = load(args.path, args.phase, args.repo, args.issue)
        print('LIFECYCLE_PR_NUMBER=' + data['pr'])
        print('LIFECYCLE_PR_HEAD_SHA=' + data['head'])


if __name__ == '__main__':
    main()
