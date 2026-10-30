"""Host-side Jira credential selection and per-worker OAuth snapshots.

ACLI 1.3 stores profile metadata in YAML and gzip-compressed OAuth JSON in the
OS keyring. That format is not a public export API: reject unfamiliar formats
rather than copying the entire keyring or refresh token into a sandbox.
"""
import base64
from datetime import datetime, timezone
import fcntl
import gzip
import json
import os
from pathlib import Path
import re
import subprocess
import sys


class JiraAuthError(Exception):
    pass


def private_command(argv):
    """Never put credential-command output into an error or log."""
    try:
        result = subprocess.run(argv, capture_output=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired):
        raise JiraAuthError(f'{argv[0]} could not run; check your local Jira authentication setup') from None
    if result.returncode:
        raise JiraAuthError(f'{argv[0]} failed; run acli jira auth login --web or choose --jira-auth api-token')
    return result.stdout


def site_url(value):
    value = value.rstrip('/')
    if not re.fullmatch(r'https://[A-Za-z0-9-]+\.atlassian\.net', value):
        raise JiraAuthError('JIRA_BASE_URL must be an HTTPS Jira Cloud tenant URL (*.atlassian.net)')
    return value


def acli_profile():
    try:
        import yaml
    except ImportError:
        raise JiraAuthError('acli auth mode needs PyYAML on the host; install python3-yaml or use --jira-auth api-token') from None
    config_dir = Path(os.environ.get('ACLI_CONFIG_DIR') or
                      str(Path(os.environ.get('XDG_CONFIG_HOME', str(Path.home()/'.config')))/'acli'))
    try:
        config = yaml.safe_load((config_dir/'jira_config.yaml').read_text())
        profiles = [p for p in config['profiles']
                    if f'{p["cloud_id"]}:{p["account_id"]}' == config['current_profile']]
        if len(profiles) != 1:
            raise ValueError('ambiguous or missing active profile')
        profile = profiles[0]
        if profile['auth_type'] not in ('oauth', 'oauth_global'):
            raise JiraAuthError('The active acli Jira profile is not OAuth; use acli jira auth login --web or --jira-auth api-token')
        profile = {key: profile[key] for key in ('site', 'cloud_id', 'account_id')}
        if not re.fullmatch(r'[A-Za-z0-9-]+', profile['cloud_id']):
            raise ValueError('invalid cloud id')
        profile['site_url'] = site_url('https://' + profile['site'].removeprefix('https://').rstrip('/'))
        return profile
    except (OSError, ValueError, KeyError, TypeError, AttributeError, yaml.YAMLError):
        raise JiraAuthError('Cannot read the active acli Jira profile; run acli jira auth login --web') from None


def selected_auth(mode):
    if mode not in ('auto', 'acli', 'api-token'):
        raise JiraAuthError('JIRA_AUTH_MODE must be auto, acli, or api-token')
    explicit = bool(os.environ.get('JIRA_TOKEN') and os.environ.get('JIRA_USER_EMAIL'))
    if mode == 'auto':
        mode = 'api-token' if explicit else 'acli'
    if mode == 'api-token':
        missing = [key for key in ('JIRA_USER_EMAIL', 'JIRA_TOKEN') if not os.environ.get(key)]
        if missing:
            raise JiraAuthError('API-token authentication requires ' + ', '.join(missing) +
                                ' in your exported environment or --env-file; alternatively use --jira-auth acli')
        return {'mode': mode, 'site_url': site_url(os.environ.get('JIRA_BASE_URL') or 'https://redhat.atlassian.net')}
    profile = acli_profile()
    if os.environ.get('JIRA_BASE_URL') and site_url(os.environ['JIRA_BASE_URL']) != profile['site_url']:
        raise JiraAuthError('JIRA_BASE_URL differs from the active acli Jira site; switch acli accounts or correct the override')
    return dict(profile, mode=mode)


def read_keyring(profile):
    username = f'oauth:{profile["cloud_id"]}:{profile["account_id"]}'
    if sys.platform == 'darwin':
        data = private_command(['security', 'find-generic-password', '-s', 'acli', '-a', username, '-w']).strip()
    elif sys.platform.startswith('linux'):
        data = private_command(['secret-tool', 'lookup', 'service', 'acli', 'username', username])
    else:
        raise JiraAuthError('acli credential snapshots support Linux and macOS; use --jira-auth api-token on this host')
    try:
        if data.startswith(b'go-keyring-base64:'):
            data = base64.b64decode(data.split(b':', 1)[1], validate=True)
        if data.startswith(b'\x1f\x8b'):
            data = gzip.decompress(data)
        token = json.loads(data)
        access_token = token['access_token']
        expiry = datetime.fromisoformat(token['expiry'].replace('Z', '+00:00'))
        if not isinstance(access_token, str) or not access_token or '\n' in access_token or '\r' in access_token:
            raise ValueError('invalid access token')
        if token.get('token_type', '').lower() != 'bearer':
            raise ValueError('unsupported token type')
        if (expiry - datetime.now(timezone.utc)).total_seconds() < 60:
            raise JiraAuthError('acli access token is expired or expires within 60 seconds; retry after it expires so acli can refresh it')
        # Deliberately do not return refresh_token or other keyring fields.
        return {'authorization': 'Bearer ' + access_token, 'expires_at': expiry.isoformat()}
    except (ValueError, KeyError, TypeError, AttributeError, OSError, EOFError):
        raise JiraAuthError('Unsupported acli keyring format; reauthenticate or use --jira-auth api-token') from None


def snapshot(selection, scope, lock_path):
    if selection['mode'] == 'api-token':
        encoded = base64.b64encode((os.environ['JIRA_USER_EMAIL'] + ':' + os.environ['JIRA_TOKEN']).encode()).decode()
        return dict(version=1, mode='api-token', site_url=selection['site_url'],
                    api_base_url=selection['site_url'], authorization='Basic ' + encoded)
    # Serialize our own acli refreshes across workers and repositories. Do not
    # refresh or mutate OAuth credentials ourselves; let the installed CLI do it.
    lock_path = Path(lock_path)
    lock_path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    with lock_path.open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        current = selected_auth('acli')
        if any(current[key] != selection[key] for key in ('cloud_id', 'account_id', 'site_url')):
            raise JiraAuthError('The active acli account changed; restart the dispatcher to select it explicitly')
        # This read-only scoped request exercises token refresh without exporting
        # issue content or relying on a cached local "authenticated" status.
        jql = f'labels = {json.dumps(scope)} AND labels = "agent/allowed"'
        private_command(['acli', 'jira', 'workitem', 'search', '--jql', jql, '--count'])
        token = read_keyring(current)
    return dict(version=1, mode='acli', site_url=current['site_url'],
                api_base_url=f'https://api.atlassian.com/ex/jira/{current["cloud_id"]}', **token)
