#!/usr/bin/env python3
"""Refresh code eligibility from live PR state, usable by any trusted scheduler."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import quote


def run(*args):
    return subprocess.check_output(args, text=True, timeout=120)


def api(path, *args):
    return json.loads(run('gh', 'api', path, *args))


def observe(repo, pr):
    library = str(Path(__file__).resolve().parent/'lib/github.sh')
    return json.loads(run('bash', '-c', '. "$1"; gh_pr_code_state "$2" "$3"',
                          'code-state', library, repo, str(pr)))


def refresh(repo, number, pr_number, scope, inspect=False):
    item = api(f'repos/{repo}/issues/{number}')
    pr = api(f'repos/{repo}/pulls/{pr_number}')
    url = f'https://github.com/{repo}/issues/{number}'
    pr_url = f'https://github.com/{repo}/pull/{pr_number}'
    closes = rf'\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?):?\s+(?:#{number}|{re.escape(repo)}#{number}|{re.escape(url)})(?![\w/])'
    explicit = re.search(re.escape(pr_url)+r'(?![\w/])', item.get('body') or '')
    if not explicit and not re.search(closes, pr.get('body') or '', re.I):
        raise ValueError('PR is not explicitly linked to the execution record')
    state = observe(repo, pr_number)
    state['changed'] = False
    labels = {label['name'] for label in item['labels']}
    dry_run = any(os.environ.get(key, '').lower() in ('1', 'true', 'yes')
                  for key in ('DRY_RUN', 'AGENT_DRY_RUN'))
    if (inspect or dry_run or item['state'] != 'open' or state['state'] != 'open'
            or not {'agent/allowed', scope} <= labels or 'agent/blocked' in labels):
        return state
    conflicted = state['mergeable'] is False and state['mergeable_state'] == 'dirty'
    # Conflicts invalidate both an existing approval and an in-progress review.
    # A separate phase label avoids counting this as a model review rejection.
    reason = 'conflicts' if conflicted else 'waiting'
    needs_recovery = conflicted or ('agent/review-code-approved' in labels and
                                    (state['checks'] != 'green' or state['draft']
                                     or state['mergeable'] is not True))
    if not labels.intersection({'agent/review-code-approved', 'agent/reviewable-code',
                                'agent/review-code-rejected', 'agent/code-conflicts',
                                'agent/review-spec-approved'}):
        return state
    if not needs_recovery:
        return state
    # Re-read authorization and head/base before writes; an unknown or changing
    # snapshot must never trigger edits based on an earlier observation.
    current = api(f'repos/{repo}/issues/{number}')
    live = api(f'repos/{repo}/pulls/{pr_number}')
    current_labels = {label['name'] for label in current['labels']}
    if (current['state'] != 'open' or live['state'] != 'open'
            or not {'agent/allowed', scope} <= current_labels or 'agent/blocked' in current_labels
            or live['head']['sha'] != state['head_sha'] or live['base']['sha'] != state['base_sha']):
        raise ValueError('Code state changed during refresh; retry discovery')
    if conflicted and not (live.get('mergeable') is False and live.get('mergeable_state') == 'dirty'):
        raise ValueError('Conflict state changed during refresh; retry discovery')
    # Revoke PR queue eligibility first; subsequent interrupted writes converge.
    def remove(number, label):
        existing = api(f'repos/{repo}/issues/{number}/labels', '--paginate', '--slurp')
        if existing and isinstance(existing[0], list):
            existing = [entry for page in existing for entry in page]
        if any(entry['name'] == label for entry in existing):
            run('gh', 'api', '-X', 'DELETE', f'repos/{repo}/issues/{number}/labels/{quote(label, safe="")}')
    remove(pr_number, 'agent/review-code-approved')
    library = str(Path(__file__).resolve().parent/'lib/lifecycle-contract.sh')
    transition = dict(line.split('=', 1) for line in run(
        'bash', '-c', '. "$1"; lifecycle_transition code-recovery "$2"',
        'code-state', library, reason).splitlines())
    target = transition['add']
    if target not in current_labels:
        library = str(Path(__file__).resolve().parent/'lib/github.sh')
        run('bash', '-c', '. "$1"; gh_add_labels "$2" "$3" "$4"',
            'code-state', library, repo, str(number), target)
    for label in transition['remove'].split():
        remove(number, label)
    state['changed'] = True
    return state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--issue-url', required=True)
    parser.add_argument('--pr', type=int, required=True)
    parser.add_argument('--scope', required=True)
    parser.add_argument('--inspect', action='store_true')
    args = parser.parse_args()
    match = re.fullmatch(r'https://github\.com/([\w.-]+/[\w.-]+)/issues/([1-9][0-9]*)', args.issue_url)
    if not match or args.pr <= 0 or not args.scope.startswith('agent/scope/'):
        parser.error('Use an exact GitHub issue URL, positive PR number, and explicit scope label')
    print(json.dumps(refresh(match[1], int(match[2]), args.pr, args.scope, args.inspect)))


if __name__ == '__main__':
    main()
