#!/usr/bin/env python3
"""Narrow migration adapter: existing forge rules and host credential integrations.

Scheduling, locks, worker supervision, timers, and terminal rendering live in Go.
Only JSON is written to stdout except during interactive gateway recovery.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from funnel_dispatch import Coordinator, Store, parse_args, prepare_inference
from gateway_setup import GatewayManager, GatewaySetupError
from jira_auth import selected_auth, snapshot


class RecoveryDiagnostics:
    """Record Python diagnostics without turning the interactive terminal into a pipe."""
    def __init__(self, terminal, path):
        self.terminal = terminal
        self.log = os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), 'w')

    def write(self, text):
        self.log.write(text)
        self.log.flush()
        return self.terminal.write(text)

    def flush(self):
        self.log.flush()
        self.terminal.flush()

    def __getattr__(self, name):
        return getattr(self.terminal, name)


def main():
    action = sys.argv[1]
    sys.argv[0] = 'funnel-dispatch.sh'
    if action == 'snapshot':
        request = json.load(sys.stdin)
        lock = Path(os.environ.get('XDG_STATE_HOME', str(Path.home()/'.local/state')))/'hypershell/funnel/acli-auth.lock'
        print(json.dumps(snapshot(request['selection'], request['scope'], lock)))
        return
    args = parse_args(sys.argv[2:])
    if args.gateway:
        os.environ['OPENSHELL_GATEWAY'] = args.gateway
        os.environ.pop('OPENSHELL_GATEWAY_ENDPOINT', None)
    if action == 'config':
        if not args.dry_run and not args.status and not args.retry:
            prepare_inference(args.fullsend_dir)
            if args.lifecycle == 'jira':
                args.jira_selection = selected_auth(args.jira_auth)
                os.environ['JIRA_BASE_URL'] = args.jira_selection['site_url']
            manager = GatewayManager(args)
            args.gateway_name = (manager.state or {}).get('name') or manager.name or 'Selected gateway'
        env_keys = ('GOOGLE_APPLICATION_CREDENTIALS', 'CLOUD_ML_REGION', 'JIRA_BASE_URL',
                    'OPENSHELL_GATEWAY', 'OPENSHELL_GATEWAY_ENDPOINT')
        result = vars(args)
        result['environment'] = {key: os.environ.get(key) for key in env_keys}
        print(json.dumps(result, default=str))
        return
    if action == 'gateway':
        args.log_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        sys.stderr = RecoveryDiagnostics(sys.stderr, args.log_dir/'gateway-signin.log')
        manager = GatewayManager(args)
        action, _ = manager.recover()
        if action != 'connect':
            raise GatewaySetupError(getattr(manager, 'last_recovery_error', 'Sign-in cancelled; the connection has not been restored'))
        print('Preparing the gateway connection (up to 60s per API request)...', file=sys.stderr, flush=True)
        deadline = time.monotonic() + args.gateway_ready_timeout
        while True:
            try:
                command = manager.registration()
                break
            except GatewaySetupError as exc:
                if time.monotonic() >= deadline:
                    raise
                print(f'{exc}; checking again in {args.auth_retry}s', file=sys.stderr)
                time.sleep(args.auth_retry)
        print(f'Opening gateway sign-in; complete it in your browser (timeout: {args.gateway_login_timeout}s).', file=sys.stderr, flush=True)
        subprocess.run(command, timeout=args.gateway_login_timeout, check=True)
        print('Verifying gateway access...', file=sys.stderr, flush=True)
        subprocess.run(['openshell', 'whoami', '--output', 'json'],
                       stdout=subprocess.DEVNULL, timeout=30, check=True)
        return
    store = Store(args.state_dir/'state.sqlite3')
    # Reuse domain rules without the Python coordinator loop or gateway side effects.
    args.dry_run = True
    coordinator = Coordinator(args, store, -1)
    try:
        if action == 'discover':
            coordinator.discover()
            print(json.dumps({'jobs': coordinator.candidates, 'errors': coordinator.errors}))
        elif action == 'authorize':
            print(json.dumps({'allowed': coordinator.authorized(json.load(sys.stdin))}))
        else:
            raise ValueError(f'Unknown adapter action: {action}')
    finally:
        store.db.close()


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(130)
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
