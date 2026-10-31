"""Interactive hsctl bootstrap and non-secret, worktree-local gateway state."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import select
import shutil
import subprocess
import sys
import tempfile
import uuid
from urllib.parse import urlsplit
from terminal_ui import ui

RELEASE_REPO = 'openshift-online/hypershell'
DEFAULT_API = 'https://api.hyp0.infra.hypershell.app'


class GatewaySetupError(Exception):
    pass


def say(message):
    ui.event(message)


def ask(title, *, description='', choices=None, default=''):
    """Structured prompts for the TUI; readable prompts for standalone use."""
    if os.environ.get('FUNNEL_EMBEDDED_UI') == '1':
        prompt = dict(title=title, description=description, default=default,
                      choices=[dict(value=v, title=t, description=d)
                               for v, t, d in (choices or [])])
        print('\x1e' + json.dumps(prompt) + '\x1f', end='', file=sys.stderr, flush=True)
        return input()
    if choices:
        ui.heading(title, description)
        ui.options(choices)
    return input(title + (f' [{default}]' if default else '') + ' ')


def confirm(title, description=''):
    return ask(title, description=description, choices=[
        ('n', 'Cancel', 'Return without making this change.'),
        ('y', 'Continue', description or 'Proceed with this action.')
    ]).strip().lower() in ('y', 'yes')


def capture(argv):
    try:
        result = subprocess.run(argv, capture_output=True, text=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise GatewaySetupError(f'{argv[0]} could not complete: {exc}') from exc
    if result.returncode:
        raise GatewaySetupError(result.stderr.strip() or f'{argv[0]} exited {result.returncode}')
    return result.stdout


def json_call(argv):
    try:
        return json.loads(capture(argv))
    except ValueError as exc:
        raise GatewaySetupError(f'{argv[0]} returned invalid JSON') from exc


def release_asset(system=None, machine=None):
    system = (system or platform.system()).lower()
    machine = (machine or platform.machine()).lower()
    arch = {'x86_64': 'amd64', 'amd64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(machine)
    if system not in ('linux', 'darwin') or not arch:
        raise GatewaySetupError(f'No supported local dispatcher hsctl download for {system}/{machine}')
    return f'hsctl-{system}-{arch}'


def verify_checksum(binary, manifest):
    matches = [line.split()[0] for line in manifest.read_text().splitlines()
               if len(line.split()) == 2 and line.split()[1].lstrip('*') == binary.name]
    if len(matches) != 1 or hashlib.sha256(binary.read_bytes()).hexdigest() != matches[0]:
        raise GatewaySetupError('hsctl release checksum verification failed; nothing was installed')


def connection_args(gateway):
    try:
        oidc = gateway['oidc']
        oidc = json.loads(oidc) if isinstance(oidc, str) else oidc
        issuer, client, audience = (oidc[key] for key in ('issuer', 'client_id', 'audience'))
        dns = (gateway.get('external_dns') or '').strip()
        endpoint = dns or (gateway.get('route_address') or '').strip()
        if endpoint.startswith('grpcs://'):
            endpoint = 'https://' + endpoint[len('grpcs://'):]
        elif '://' not in endpoint and endpoint:
            endpoint = 'https://' + endpoint
        if not all((issuer, client, audience, endpoint)) or urlsplit(endpoint).scheme != 'https':
            raise ValueError('pending or insecure connection metadata')
        if client.startswith('hs-sa-'):
            raise ValueError('service-account client cannot perform interactive login')
        return ['openshell', 'gateway', 'add', '--name', gateway['name'],
                '--oidc-issuer', issuer, '--oidc-client-id', client,
                '--oidc-audience', audience, endpoint]
    except (ValueError, TypeError, KeyError) as exc:
        raise GatewaySetupError('Gateway has no usable interactive OIDC connection metadata yet') from exc


class GatewayManager:
    def __init__(self, args):
        self.path = args.gateway_state
        self.login_timeout = args.gateway_login_timeout
        self.hsctl = shutil.which('hsctl')
        self.state = None
        self.state_error = None
        if self.path.exists():
            try:
                value = json.loads(self.path.read_text())
                if value.get('version') != 1 or not all(isinstance(value.get(k), str) and value[k] for k in ('id', 'name', 'api_url')):
                    raise ValueError('invalid state')
                if 'local_name' in value and not isinstance(value['local_name'], str):
                    raise ValueError('invalid local gateway name')
                self.state = value
            except (OSError, ValueError, TypeError, AttributeError):
                self.state_error = 'Cannot read saved gateway state; reconnect or create a gateway to replace it.'
        self.api_url = args.api_url or (self.state or {}).get('api_url') or DEFAULT_API
        parsed = urlsplit(self.api_url)
        if parsed.scheme not in ('http', 'https') or not parsed.netloc or parsed.username or parsed.password:
            raise GatewaySetupError('Invalid HyperShell API URL')
        # A command-line selection wins; otherwise this worktree's saved gateway
        # wins over the shell/global default that may have caused the failure.
        self.name = args.gateway or os.environ.get('OPENSHELL_GATEWAY')
        if self.state and not args.gateway:
            self.name = self.state.get('local_name') or self.state['name']
            os.environ['OPENSHELL_GATEWAY'] = self.name
            os.environ.pop('OPENSHELL_GATEWAY_ENDPOINT', None)
            say(f'Reusing saved gateway {self.name} ({self.state["id"]}) from {self.path}')

    @staticmethod
    def interactive():
        return sys.stdin.isatty() and sys.stderr.isatty()

    def ensure_hsctl(self):
        if self.hsctl:
            return
        target = Path.home()/'.local/bin/hsctl'
        if target.is_file() and os.access(target, os.X_OK):
            self.hsctl = str(target)
            return
        asset = release_asset()
        release = json_call(['gh', 'api', f'repos/{RELEASE_REPO}/releases/latest'])
        names = {item['name'] for item in release['assets']}
        if not {asset, 'SHA256SUMS'} <= names:
            raise GatewaySetupError(f'Latest release has no {asset} and SHA256SUMS pair')
        tag = release['tag_name']
        say(f'hsctl is missing. Release: https://github.com/{RELEASE_REPO}/releases/tag/{tag}')
        if not confirm('Install hsctl?', f'Download {asset}, verify SHA256SUMS, and install to {target}.'):
            raise GatewaySetupError('hsctl installation declined; gateway setup remains paused')
        target.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='.hsctl-download-', dir=target.parent) as directory:
            capture(['gh', 'release', 'download', tag, '--repo', RELEASE_REPO, '--pattern', asset,
                     '--pattern', 'SHA256SUMS', '--dir', directory])
            binary = Path(directory)/asset
            verify_checksum(binary, Path(directory)/'SHA256SUMS')
            binary.chmod(0o755)
            # Avoid replacing a binary that appeared while the prompt was open.
            if target.exists():
                raise GatewaySetupError(f'{target} now exists; inspect it before retrying')
            os.replace(binary, target)
        self.hsctl = str(target)
        say(f'Installed hsctl to {target}')

    def ensure_api(self):
        say('Checking hsctl and API configuration...')
        self.ensure_hsctl()
        config = capture([self.hsctl, 'config'])
        match = re.search(r'^API URL:\s*(\S+)', config, re.M)
        configured = match.group(1).rstrip('/') if match else ''
        if configured != self.api_url.rstrip('/'):
            say(f'Gateway setup needs hsctl login to {self.api_url} (current: {configured or "not configured"}).')
            if not confirm('Log in to this API now?', self.api_url):
                raise GatewaySetupError('HyperShell API login declined')
            self.api_login()
        try:
            say(f'Checking API sign-in at {self.api_url} (up to 60s)...')
            # whoami can report cached identity even after the API session expires.
            capture([self.hsctl, 'list', 'gateways', '--output', 'json'])
        except GatewaySetupError:
            if not confirm('hsctl authentication failed. Log in again?', self.api_url):
                raise GatewaySetupError('HyperShell API authentication is unavailable')
            self.api_login()

    def api_login(self):
        try:
            result = subprocess.run([self.hsctl, 'login', '--url', self.api_url], timeout=self.login_timeout)
        except subprocess.TimeoutExpired:
            raise GatewaySetupError('hsctl browser login timed out; retry setup when ready') from None
        if result.returncode:
            raise GatewaySetupError('hsctl login failed')

    def save(self, gateway, local_name=None):
        state = {'version': 1, 'api_url': self.api_url, 'id': gateway['id'], 'name': gateway['name']}
        if local_name:
            state['local_name'] = local_name
        elif self.state and self.state['id'] == gateway['id'] and self.state.get('local_name'):
            state['local_name'] = self.state['local_name']
        self.path.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(mode='w', prefix='.gateway-', dir=self.path.parent, delete=False) as output:
            temporary = Path(output.name)
            json.dump(state, output)
            output.write('\n')
        try:
            os.replace(temporary, self.path)
        finally:
            temporary.unlink(missing_ok=True)
        self.state, self.state_error, self.name = state, None, state.get('local_name') or state['name']
        os.environ['OPENSHELL_GATEWAY'] = self.name
        os.environ.pop('OPENSHELL_GATEWAY_ENDPOINT', None)

    def choose(self, items, label):
        if not items:
            raise GatewaySetupError(f'No {label} are available to this account')
        value = ask(f'Select a {label}', description=f'HyperShell API: {self.api_url}',
                    choices=[(str(index), item.get('name', item['id']),
                              ', '.join(item.get('capabilities') or []) or item.get('phase') or item['id'])
                             for index, item in enumerate(items, 1)]).strip()
        if not value.isdigit() or not 1 <= int(value) <= len(items):
            raise GatewaySetupError('Selection cancelled')
        return items[int(value)-1]

    def existing(self):
        say('Loading gateway connection details (up to 60s per request)...')
        if self.state and self.name in (self.state['name'], self.state.get('local_name')):
            # A failed lookup is not proof of deletion: preserve state and let
            # the user explicitly choose whether to create a replacement.
            if self.state['id'] == 'pending-create':
                return dict(self.state)
            return json_call([self.hsctl, 'get', 'gateway', self.state['id']])
        if not self.name:
            registered = json_call(['openshell', 'gateway', 'list', '--output', 'json'])
            active = [g for g in registered if g.get('active')]
            if len(active) == 1:
                self.name = active[0]['name']
        gateways = json_call([self.hsctl, 'list', 'gateways', '--output', 'json'])['items']
        matches = [g for g in gateways if g['name'] == self.name]
        if len(matches) == 1:
            return matches[0]
        return self.choose(gateways, 'gateway')

    def create(self):
        say('Loading available clusters (up to 60s)...')
        clusters = json_call([self.hsctl, 'list', 'managedClusters', '--output', 'json'])['items']
        cluster = self.choose(clusters, 'cluster')
        default = 'fullsend-' + uuid.uuid4().hex[:8]
        name = ask('New gateway name', description='Lowercase letters, digits and hyphens. Leave blank to use the suggested name.', default=default).strip() or default
        if not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,62}', name):
            raise GatewaySetupError('Use a lowercase gateway name with letters, digits and hyphens (maximum 63 characters)')
        say(f'Create gateway {name} on {cluster.get("name", cluster["id"])} using {self.api_url}. Existing gateways will be kept.')
        if not confirm('Create this gateway?', f'{name} on {cluster.get("name", cluster["id"])} at {self.api_url}. Existing gateways will be kept.'):
            raise GatewaySetupError('Gateway creation cancelled')
        # Persist the name intent BEFORE the request. If its response is lost,
        # recover by name instead of issuing another create on an automatic retry.
        self.save({'id': 'pending-create', 'name': name})
        try:
            gateway = json_call([self.hsctl, 'create', 'gateway', '--name', name, '--cluster-id', cluster['id']])
        except GatewaySetupError:
            raise GatewaySetupError(f'Gateway creation outcome is unknown. Saved name {name}; reconnect to check it before creating another.') from None
        if not isinstance(gateway, dict) or not gateway.get('id') or gateway.get('name') != name:
            raise GatewaySetupError(f'Gateway create returned an unexpected response. Saved name {name}; reconnect to resolve its ID.')
        self.save(gateway)
        return gateway

    def recover(self, *, allow_replace=True):
        if not self.interactive():
            say('Gateway setup needs a terminal. Re-run interactively to reconnect with hsctl or create a replacement gateway.')
            return 'wait', None
        if self.state_error:
            say(self.state_error)
        if not allow_replace:
            say('Workers are still running. Gateway replacement will be offered after they finish; current workers keep their gateway.')
            return 'wait', None
        choices = [
            ('1', 'Reconnect and sign in', 'Refresh this gateway connection, then sign in with your browser.'),
            ('2', 'Create a new gateway', 'Choose a cluster and name. Your existing gateway will be kept.'),
            ('3', 'Keep waiting', 'Return to the work list and retry the connection automatically.')]
        try:
            if os.environ.get('FUNNEL_EMBEDDED_UI') == '1':
                choice = ask('Connect your gateway', description='New work is paused. Choose how to reconnect.',
                             choices=choices).strip().lower()
            else:
                ui.heading('Gateway needs attention', 'New work is paused. Choose how to reconnect:')
                ui.options(choices)
                prompt = 'Choice [1/2/3, default 3 in 30s]: '
                if sys.stdin.isatty():
                    print(prompt, end='', file=sys.stderr, flush=True)
                    readable, _, _ = select.select([sys.stdin], [], [], 30)
                    if not readable:
                        say('\nNo recovery action selected; continuing gateway checks.')
                        return 'wait', None
                    prompt = ''
                choice = input(prompt).strip().lower()
        except EOFError:
            return 'wait', None
        # Keep the old shortcuts usable, but never send `l` through stale local
        # OIDC metadata. All sign-ins refresh the connection through the API.
        choice = {'1': 'r', 'l': 'r', '2': 'n'}.get(choice, choice)
        if choice not in ('r', 'n'):
            return 'wait', None
        try:
            self.ensure_api()
            gateway = self.create() if choice == 'n' else self.existing()
            self.save(gateway)
            return 'connect', gateway
        except (GatewaySetupError, OSError, EOFError) as exc:
            say(f'Gateway setup failed: {exc}. New launches remain paused; no existing gateway was deleted.')
            self.last_recovery_error = str(exc)
            return 'wait', None

    def registration(self):
        self.ensure_hsctl()
        if self.state['id'] == 'pending-create':
            gateways = json_call([self.hsctl, 'list', 'gateways', '--output', 'json'])['items']
            matches = [g for g in gateways if g['name'] == self.state['name']]
            if len(matches) != 1:
                raise GatewaySetupError('Pending gateway creation has not resolved to exactly one gateway; inspect the API before creating another')
            self.save(matches[0])
        gateway = json_call([self.hsctl, 'get', 'gateway', self.state['id']])
        if gateway.get('phase', '').lower() not in ('running', 'ready'):
            raise GatewaySetupError(f'Gateway {gateway["name"]} is {gateway.get("phase", "pending")}')
        argv = connection_args(gateway)
        # `gateway add` refuses existing names. Keep other tools' registrations
        # intact, including service-account registrations that cannot do browser
        # login. A metadata-derived alias is stable across restarts and changes
        # when the API advertises new OIDC settings.
        digest = hashlib.sha256(json.dumps(argv[5:]).encode()).hexdigest()[:8]
        alias = f'funnel-{gateway["id"]}-{digest}'
        registrations = json_call(['openshell', 'gateway', 'list', '--output', 'json'])
        matches = [g for g in registrations if g['name'] == alias]
        if matches and (len(matches) != 1 or matches[0]['endpoint'] != argv[-1]):
            raise GatewaySetupError(f'Local gateway alias {alias} is already used by another endpoint')
        self.save(gateway, local_name=alias)
        os.environ.pop('OPENSHELL_OIDC_CLIENT_SECRET', None)
        if matches:
            return ['openshell', 'gateway', 'login', alias]
        argv[argv.index('--name')+1] = alias
        return argv
