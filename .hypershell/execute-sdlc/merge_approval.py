"""Manual merge eligibility. These checks run on the host, never in an agent."""
from funnel_dispatch import DispatchError, command, json_command

LABEL = 'agent/review-code-approved'


def ready(state):
    return (state and state.get('state') == 'open' and state.get('draft') is False
            and state.get('mergeable') is True and state.get('checks') == 'green')


def authorized(github, item):
    labels = set(github.labels(item))
    return (item['state'] == 'open' and {'agent/allowed', github.scope, LABEL} <= labels
            and not labels.intersection({'agent/blocked', 'agent/reviewable-code',
                                         'agent/review-code-rejected', 'agent/code-conflicts'}))


def pending(github, item, pr, state):
    if not authorized(github, item) or LABEL in github.labels(pr) or not ready(state):
        return None
    return dict(key=f'{item["number"]}:merge-approval', item=str(item['number']),
                title=item.get('title', ''), url=item['html_url'], repo=github.repo,
                scope=github.scope, agent='code-review', pr_number=str(pr['number']),
                pr_url=pr['html_url'], pr_head_sha=state['head_sha'],
                pr_base_sha=state['base_sha'], status='awaiting_approval')


def approve(github, request):
    number, pr_number = str(request.get('item', '')), str(request.get('pr_number', ''))
    if (not number.isdecimal() or int(number) <= 0 or not pr_number.isdecimal() or int(pr_number) <= 0
            or request.get('repo') != github.repo or request.get('scope') != github.scope):
        raise DispatchError('Invalid merge approval target')
    item = github.current(number)
    pr = github.review_pr(item, github.pages('pulls', state='all'))
    if str(pr['number']) != pr_number or not authorized(github, item):
        raise DispatchError('Issue authorization or linked PR changed; refresh before approving')
    state = github.code_state(item, pr, inspect=True)
    def unchanged(current):
        return (ready(current) and current.get('head_sha') == request.get('pr_head_sha')
                and current.get('base_sha') == request.get('pr_base_sha'))
    if not request.get('pr_head_sha') or not request.get('pr_base_sha') or not unchanged(state):
        raise DispatchError('PR commits or readiness changed; refresh before approving')
    # Re-read immediately before writing; the displayed snapshot grants no authority.
    if not authorized(github, github.current(number)) or not unchanged(github.code_state(item, pr, inspect=True)):
        raise DispatchError('Approval eligibility changed; PR label left unchanged')
    path = f'repos/{github.repo}/issues/{pr_number}/labels'
    json_command(['gh', 'api', '-X', 'POST', path, '-f', 'labels[]='+LABEL])
    try:
        live = github.current(number)
        current_pr = github.review_pr(live, github.pages('pulls', state='all'))
        if (str(current_pr['number']) != pr_number or not authorized(github, live)
                or not unchanged(github.code_state(live, current_pr, inspect=True))):
            raise DispatchError('PR changed during approval')
        labels = json_command(['gh', 'api', path, '--paginate', '--slurp'])
        if not any(label['name'] == LABEL for page in labels for label in page):
            raise DispatchError('PR label verification failed')
    except Exception:
        # Fail closed if the final observation fails; normal maintenance also
        # revokes eligibility when CI or conflicts change after this action.
        command(['gh', 'api', '-X', 'DELETE', path+'/agent%2Freview-code-approved'])
        raise
