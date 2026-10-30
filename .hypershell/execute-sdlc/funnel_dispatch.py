#!/usr/bin/env python3
"""Local Fullsend coordinator. SQLite stores outcomes; flock owns execution."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import time
import uuid

from local_harness import prepare as prepare_local_harness
from jira_auth import JiraAuthError, selected_auth, snapshot
from gateway_setup import GatewayManager, GatewaySetupError
from terminal_ui import ui


class DispatchError(Exception):
    pass


def report(message):
    ui.event(message)


def prepare_inference(fullsend_dir):
    harnesses = Path(fullsend_dir)/'harness'
    if not any('CLAUDE_CODE_USE_VERTEX:' in path.read_text()
               for path in harnesses.glob('*.yaml')):
        return
    configured = os.environ.get('GOOGLE_APPLICATION_CREDENTIALS')
    config_dir = Path(os.environ.get('CLOUDSDK_CONFIG', str(Path.home()/'.config/gcloud'))).expanduser()
    credentials = Path(configured).expanduser() if configured else config_dir/'application_default_credentials.json'
    if not credentials.is_file() or not os.access(credentials, os.R_OK):
        raise DispatchError('Vertex inference requires a readable credentials file. '
                            'Set GOOGLE_APPLICATION_CREDENTIALS in your exported environment or '
                            '--env-file (for example --env-file ~/hypinfer.env). '
                            'To create local ADC credentials, run gcloud auth application-default login.')
    os.environ['GOOGLE_APPLICATION_CREDENTIALS'] = str(credentials.resolve())
    if not configured:
        report(f'Inference: using local ADC credentials at {credentials}')
    if not os.environ.get('CLOUD_ML_REGION'):
        os.environ['CLOUD_ML_REGION'] = os.environ.get('ANTHROPIC_VERTEX_REGION', '')
    missing = [name for name in ('ANTHROPIC_VERTEX_PROJECT_ID', 'CLOUD_ML_REGION') if not os.environ.get(name)]
    if missing:
        raise DispatchError('Vertex inference requires ' + ', '.join(missing) +
                            ' in your exported environment or --env-file (for example --env-file ~/hypinfer.env).')


def command(args, *, data=None, timeout=60):
    try:
        result = subprocess.run(args, input=data, text=True, capture_output=True, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise DispatchError(f'{args[0]} failed: {exc}') from exc
    if result.returncode:
        raise DispatchError(f'{" ".join(args[:3])}: {result.stderr.strip() or result.stdout.strip()}')
    return result.stdout


def json_command(args, **kwargs):
    try:
        return json.loads(command(args, **kwargs))
    except ValueError as exc:
        raise DispatchError(f'{args[0]} returned invalid JSON') from exc


def fingerprint(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()


class Store:
    def __init__(self, path):
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.db = sqlite3.connect(self.path, timeout=30)
        self.db.row_factory = sqlite3.Row
        self.db.executescript('''
            CREATE TABLE IF NOT EXISTS jobs (
                key TEXT PRIMARY KEY, scope TEXT NOT NULL, agent TEXT NOT NULL,
                item TEXT NOT NULL, fingerprint TEXT NOT NULL, payload TEXT NOT NULL,
                status TEXT NOT NULL, failures INTEGER NOT NULL DEFAULT 0,
                next_at REAL NOT NULL DEFAULT 0, started REAL NOT NULL DEFAULT 0,
                finished REAL, run_id TEXT, log TEXT, error TEXT
            );
            CREATE TABLE IF NOT EXISTS runs (
                id TEXT PRIMARY KEY, job_key TEXT NOT NULL, started REAL NOT NULL,
                finished REAL, exit_code INTEGER, log TEXT NOT NULL
            );
        ''')

    def get(self, key):
        return self.db.execute('SELECT * FROM jobs WHERE key=?', (key,)).fetchone()

    def eligible(self, job, now):
        if self.db.execute("SELECT 1 FROM jobs WHERE item=? AND status='uncertain' LIMIT 1", (job['item'],)).fetchone():
            return False
        row = self.get(job['key'])
        if row is None:
            return True
        if row['status'] in ('running', 'uncertain') or row['next_at'] > now:
            return False
        return row['status'] != 'succeeded' or row['fingerprint'] != job['fingerprint']

    def start(self, job, log, now):
        run_id = uuid.uuid4().hex
        old = self.get(job['key'])
        failures = old['failures'] if old else 0
        with self.db:
            self.db.execute('''INSERT INTO jobs
                (key,scope,agent,item,fingerprint,payload,status,failures,started,run_id,log)
                VALUES (?,?,?,?,?,?,'running',?,?,?,?)
                ON CONFLICT(key) DO UPDATE SET scope=excluded.scope, fingerprint=excluded.fingerprint,
                payload=excluded.payload,status='running',started=excluded.started,
                run_id=excluded.run_id,log=excluded.log,error=NULL,finished=NULL,next_at=0''',
                (job['key'], job['scope'], job['agent'], job['item'], job['fingerprint'],
                 json.dumps(job), failures, now, run_id, str(log)))
            self.db.execute('INSERT INTO runs(id,job_key,started,log) VALUES(?,?,?,?)',
                            (run_id, job['key'], now, str(log)))
        return run_id

    def finish(self, key, run_id, rc, base, cap, now=None):
        now = time.time() if now is None else now
        row = self.get(key)
        if not row or row['run_id'] != run_id:
            raise DispatchError(f'Run ownership changed for {key}')
        failures = row['failures'] + 1 if rc else 0
        delay = min(cap, base * 2 ** min(failures - 1, 20)) if rc else 0
        with self.db:
            self.db.execute('''UPDATE jobs SET status=?,failures=?,next_at=?,finished=?,error=?
                WHERE key=? AND run_id=?''',
                ('failed' if rc else 'succeeded', failures, now + delay, now,
                 f'exit {rc}; see {row["log"]}' if rc else None, key, run_id))
            self.db.execute('UPDATE runs SET finished=?,exit_code=? WHERE id=?', (now, rc, run_id))

    def run_lock(self, run_id):
        directory = self.path.parent/'workers'
        directory.mkdir(exist_ok=True, mode=0o700)
        return (directory/(fingerprint(run_id)+'.lock')).open('a')

    def running(self, run_id):
        # The supervisor and its Fullsend child inherit this per-run lock.
        # No PID reuse assumptions and no coordinator lock held by workers.
        with self.run_lock(run_id) as lock:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                return True
        return False

    def recover(self):
        unknown = []
        for row in self.db.execute("SELECT key,run_id FROM jobs WHERE status='running'").fetchall():
            if not self.running(row['run_id']):
                with self.db:
                    changed = self.db.execute("UPDATE jobs SET status='uncertain',error='Worker disappeared; inspect the remote run before retrying' WHERE key=? AND status='running' AND run_id=?", (row['key'], row['run_id'])).rowcount
                if changed:
                    unknown.append(row['key'])
        return unknown

    def retry_uncertain(self, key):
        with self.db:
            changed = self.db.execute("UPDATE jobs SET status='failed',next_at=0 WHERE key=? AND status='uncertain'", (key,)).rowcount
        if not changed:
            raise DispatchError(f'{key} is not an uncertain job')


class GitHub:
    def __init__(self, repo, scope, fullsend_dir):
        self.repo, self.scope, self.fullsend_dir = repo, scope, fullsend_dir

    def pages(self, resource, **fields):
        args = ['gh', 'api', '--method', 'GET', '--paginate', '--slurp',
                f'repos/{self.repo}/{resource}', '-f', 'per_page=100']
        for key, value in fields.items():
            args += ['-f', f'{key}={value}']
        pages = json_command(args)
        if not isinstance(pages, list) or any(not isinstance(page, list) for page in pages):
            raise DispatchError(f'Unexpected pagination response for {resource}')
        return [item for page in pages for item in page]

    def discover(self):
        items = self.pages('issues', state='all', labels=f'{self.scope},agent/allowed')
        return [item for item in items if 'pull_request' not in item and
                {self.scope, 'agent/allowed'} <= set(self.labels(item))]

    @staticmethod
    def labels(item):
        return sorted(label['name'] if isinstance(label, dict) else label for label in item['labels'])

    def classify(self, item):
        labels = self.labels(item)
        if item['state'] != 'open':
            return []
        agents = set()
        for label in labels:
            if not label.startswith('agent/'):
                continue
            event = dict(repo=self.repo,
                         entity=dict(kind='work_item', id=item['number'], url=item['html_url']),
                         source=dict(system='github', raw_type='issues', raw_action='labeled'),
                         transition=dict(kind='label_changed', label=dict(name=label, action='added')),
                         actor=dict(id='local-dispatch', kind='human', role='write', is_entity_author=False),
                         state=dict(labels=labels))
            result = json_command(['fullsend', 'dispatch', '--config-dir', self.fullsend_dir,
                                   '--input-driver', 'json', '--output-driver', 'json'], data=json.dumps(event))
            # Lifecycle work is scheduled separately with local Jira credentials.
            # Hosted lifecycle harnesses must not also run as issue phases.
            agents.update(entry['agent'] for entry in (result or [])
                          if entry['agent'] not in ('ingest', 'reconcile'))
        return sorted(agents)

    def linked_prs(self, item, prs):
        # Explicit URLs and closing references only. A descriptive Jira key or
        # loose number search is not evidence that a PR implements this issue.
        number = item['number']
        references = rf'(?:#{number}|{re.escape(self.repo)}#{number}|{re.escape(item["html_url"])})'
        closes = re.compile(r'\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+' + references + r'(?![\w/])', re.I)
        body = item.get('body') or ''
        return [pr for pr in prs if
                re.search(re.escape(pr['html_url']) + r'(?![\w/])', body) or
                closes.search(pr.get('body') or '')]

    def review_pr(self, item, prs):
        matches = self.linked_prs(item, prs)
        if len(matches) != 1 or matches[0]['state'] != 'open':
            ids = ', '.join('#' + str(pr['number']) for pr in matches) or 'none'
            raise DispatchError(f'#{item["number"]}: need exactly one linked, open PR; found {ids}. '
                                'Add an explicit PR URL or a Fixes #N reference; resolve ambiguous links.')
        return matches[0]

    def phase_job(self, scope, agent, item, prs=None):
        """Use the same live inputs for discovery and pre-launch validation."""
        implementation = agent in ('code-implementation', 'spec-implementation')
        pr, rejection = None, None
        if implementation or agent.endswith('-review'):
            if prs is None:
                prs = self.pages('pulls', state='all')
            label = f'agent/review-{agent.split("-")[0]}-rejected'
            rejected = implementation and label in self.labels(item)
            if not implementation or rejected or self.linked_prs(item, prs):
                pr = self.review_pr(item, prs)
            if rejected:
                events = self.pages(f'issues/{item["number"]}/events')
                occurrences = [event['id'] for event in events
                               if event.get('event') == 'labeled'
                               and (event.get('label') or {}).get('name') == label
                               and isinstance(event.get('id'), int)]
                if not occurrences:
                    raise DispatchError(f'#{item["number"]}: cannot resolve the current rejection event; retry discovery')
                rejection = max(occurrences)
        return make_job(scope, agent, item, pr, rejection)

    def current(self, number):
        return json_command(['gh', 'api', f'repos/{self.repo}/issues/{number}'])


class GatewayAuth:
    def __init__(self, retry, manager=None, can_replace=lambda: True, login_timeout=120, ready_timeout=600, diagnostics=None):
        self.retry, self.next_at, self.attempt = retry, 0, 0
        self.login = None
        self.status = 'not checked'
        self.manager, self.can_replace = manager, can_replace
        self.login_timeout, self.ready_timeout = login_timeout, ready_timeout
        self.login_started, self.setup_started = 0, 0
        self.connecting = False
        self.diagnostics = diagnostics

    def start_login(self, argv, now):
        report(f'Opening gateway sign-in. Complete it in your browser (timeout: {self.login_timeout}s).')
        self.login = subprocess.Popen(argv)
        self.login_started = now
        self.status = 'paused: waiting for gateway login'

    def recovery(self, now):
        action, gateway = self.manager.recover(allow_replace=self.can_replace())
        if action == 'connect':
            self.connecting, self.setup_started, self.next_at = True, now, 0
            self.status = f'paused: provisioning/connecting {gateway["name"]}'

    def ready(self, now):
        if self.login:
            rc = self.login.poll()
            if rc is None:
                if now - self.login_started < self.login_timeout:
                    return False
                self.login.terminate()
                try:
                    self.login.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    self.login.kill()
                    self.login.wait()
                self.login = None
                report('Gateway browser login timed out. A rejected redirect_uri can leave the CLI waiting; reconnect with hsctl or create a replacement.')
                self.next_at = now + self.retry
                if self.manager:
                    self.recovery(now)
                return False
            self.login = None
            if rc:
                report(f'Gateway login failed (exit {rc}); new launches remain paused. Retrying in {self.retry}s.')
                self.next_at = now + self.retry
                return False
            self.next_at = 0
        if now < self.next_at:
            return False
        if self.connecting:
            try:
                argv = self.manager.registration()
                self.start_login(argv, now)
                self.connecting = False
            except GatewaySetupError as exc:
                report(f'Gateway not ready to connect: {exc}; retrying in {self.retry}s')
                self.next_at = now + self.retry
                if now - self.setup_started >= self.ready_timeout:
                    self.connecting = False
                    self.recovery(now)
            return False
        try:
            command(['openshell', 'whoami', '--output', 'json'], timeout=30)
        except DispatchError as exc:
            self.attempt += 1
            self.status = f'paused: gateway check failed (attempt {self.attempt})'
            if ui.interactive and self.diagnostics:
                try:
                    with self.diagnostics.open('a') as output:
                        output.write(f'{time.ctime()}\n{exc}\n\n')
                    report(f'Gateway unavailable (attempt {self.attempt}). New work is paused.\nDetails: {self.diagnostics}')
                except OSError as error:
                    report(f'{self.status}\n{exc}\nCould not write gateway diagnostics: {error}')
            else:
                report(f'{self.status}\n{exc}\nRetrying in {self.retry}s; running jobs remain tracked.')
            self.next_at = now + self.retry
            if self.manager:
                self.recovery(now)
                return False
            if re.search(r'unauthenticated|requires authentication|credentials are missing|no OIDC token stored|expired|invalid_grant|invalid_client|openshell gateway login', str(exc), re.I):
                if sys.stdin.isatty() and sys.stderr.isatty():
                    report('Opening OpenShell gateway login; complete sign-in in your browser.')
                    self.start_login(['openshell', 'gateway', 'login'], now)
                else:
                    report('No interactive terminal. Run openshell gateway login in another terminal '
                           f'(gateway: {os.environ.get("OPENSHELL_GATEWAY", "active selection")}); checking will continue.')
            return False
        if self.attempt:
            report('Gateway authentication restored; resuming dispatch.')
        self.attempt = 0
        self.status = 'ready'
        return True

    def close(self):
        if self.login and self.login.poll() is None:
            self.login.terminate()
            try:
                self.login.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.login.kill()
                self.login.wait(timeout=5)


def make_job(scope, agent, item, pr=None, rejection=None):
    context = dict(labels=GitHub.labels(item), body=item.get('body'), title=item.get('title'),
                   state=item['state'], pr=pr and [pr['number'], pr.get('head', {}).get('sha')])
    if rejection is not None:
        context['rejection_event'] = rejection
    # Preserve existing storage identities when reusing the shared agent names.
    storage_agent = 'funnel-reconcile' if agent == 'reconcile' else agent
    return dict(key=f'{item["number"]}:{storage_agent}', scope=scope, agent=agent,
                item=str(item['number']), title=item.get('title', ''), url=item['html_url'], fingerprint=fingerprint(context),
                labels=GitHub.labels(item),
                pr_number=str(pr['number']) if pr else '', pr_url=pr['html_url'] if pr else '')


def worker(db_path, key, run_id, lock_fd, base, cap):
    store = Store(db_path)
    row = store.get(key)
    job = json.loads(row['payload'])
    job['agent'] = job['agent'].removeprefix('funnel-') if job['agent'] in ('funnel-ingest', 'funnel-reconcile') else job['agent']
    env = dict(os.environ, FULLSEND_WORK_ITEM_URL=job['url'], ISSUE_NUMBER=job['item'],
               REPOSITORY=job['repo'], REPO_FULL_NAME=job['repo'], LABEL_SCOPE=job['scope'],
               DRY_RUN=os.environ.get('AGENT_DRY_RUN', 'false'), PR_NUMBER=job['pr_number'], PR_URL=job['pr_url'],
               PREVIOUS_PHASE=job.get('previous_phase', ''))
    credentials = None
    runtime_dir = None
    try:
        if job['agent'] in ('ingest', 'reconcile'):
            # Keep Jira secrets out of job payloads, command lines and logs.
            # The sandbox receives only this run's private credential file.
            auth_lock = Path(os.environ.get('XDG_STATE_HOME', str(Path.home()/'.local/state')))/'hypershell/funnel/acli-auth.lock'
            auth = snapshot(job['jira_auth'], job['scope'], auth_lock)
            credentials = tempfile.NamedTemporaryFile(mode='w', prefix='local-dispatch-jira-', suffix='.json')
            json.dump(auth, credentials)
            credentials.flush()
            env['LOCAL_DISPATCH_JIRA_CREDENTIALS_FILE'] = credentials.name
        # The CLI also inherits the lock: killing this supervisor cannot permit
        # another coordinator to duplicate a still-running Fullsend invocation.
        command = list(job['command'])
        if job['agent'] in ('ingest', 'reconcile'):
            command[2] = job['agent']
            index = command.index('--fullsend-dir') + 1
            runtime_dir = prepare_local_harness(command[index], job['agent'])
            command[index] = runtime_dir
        rc = subprocess.call(command, env=env, pass_fds=(lock_fd,))
    except (OSError, ValueError, JiraAuthError) as exc:
        print(exc, flush=True)
        rc = 127
    finally:
        if runtime_dir:
            shutil.rmtree(runtime_dir)
        if credentials:
            credentials.close()
    store.finish(key, run_id, rc, base, cap)
    return rc


class Coordinator:
    def __init__(self, args, store, lock_fd):
        self.args, self.store, self.lock_fd = args, store, lock_fd
        self.github = GitHub(args.repo, args.scope, args.fullsend_dir)
        self.active = {row['key']: None for row in store.db.execute("SELECT key FROM jobs WHERE status='running'")}
        self.candidates, self.errors = [], []
        if self.active:
            report(f'Resumed monitoring {len(self.active)} running worker(s). Free slots can accept new work.')
        manager = None if args.dry_run else GatewayManager(args)
        self.auth = GatewayAuth(args.auth_retry, manager, lambda: not self.active,
                                args.gateway_login_timeout, args.gateway_ready_timeout,
                                args.log_dir/'gateway-auth.log')
        self.last_display = None
        self.discovering = False
        self.next_discovery = 0
        self.next_auth_check = 0

    def maintain_auth(self, now):
        if self.args.dry_run:
            return
        if now < self.next_auth_check and not self.auth.login:
            return
        ready = self.auth.ready(now)
        interval = self.args.auth_check_interval if ready else self.args.auth_retry
        self.next_auth_check = time.time() + interval

    def discover(self):
        ingest = self.intake_job()
        if self.args.lifecycle == 'jira':
            row = self.store.get(ingest['key'])
            if not row or row['status'] != 'succeeded':
                self.candidates, self.errors = [ingest], []
                return
        items = self.github.discover()
        candidates, errors = ([ingest] if self.args.lifecycle == 'jira' else []), []
        prs = None
        for item in items:
            # Backfill display titles for runs created before titles were saved.
            for row in self.store.db.execute('SELECT key,payload FROM jobs WHERE item=?', (str(item['number']),)).fetchall():
                payload = json.loads(row['payload'])
                if payload.get('title') != item.get('title', ''):
                    payload['title'] = item.get('title', '')
                    with self.store.db:
                        self.store.db.execute('UPDATE jobs SET payload=? WHERE key=?', (json.dumps(payload), row['key']))
            try:
                if self.args.lifecycle == 'jira':
                    if 'agent/execution-record' not in self.github.labels(item):
                        # New GitHub-origin records must pass intake first.
                        ingest['fingerprint'] = fingerprint([ingest['fingerprint'], item['number']])
                        errors.append(f'#{item["number"]}: awaiting canonical execution-record link from intake; phase work is paused')
                        continue
                    if prs is None:
                        prs = self.github.pages('pulls', state='all')
                    reconcile = self.reconcile_job(item, prs)
                    candidates.append(reconcile)
                    row = self.store.get(reconcile['key'])
                    if not row or row['status'] != 'succeeded' or row['fingerprint'] != reconcile['fingerprint']:
                        continue
                for agent in self.github.classify(item):
                    if (agent.endswith('-review') or agent in ('code-implementation', 'spec-implementation')) and prs is None:
                        prs = self.github.pages('pulls', state='all')
                    candidates.append(self.github.phase_job(self.args.scope, agent, item, prs))
            except DispatchError as exc:
                errors.append(str(exc))
        self.candidates, self.errors = candidates, errors
        for error in errors:
            report(error)

    def intake_job(self):
        return dict(key=f'{self.args.scope}:funnel-ingest', scope=self.args.scope,
                    item=self.args.scope, agent='ingest', url='', pr_number='', pr_url='',
                    fingerprint=fingerprint(int(time.time() // self.args.lifecycle_interval)))

    def reconcile_job(self, item, prs):
        job = make_job(self.args.scope, 'reconcile', item)
        matches = self.github.linked_prs(item, prs)
        if len(matches) > 1:
            raise DispatchError(f'#{item["number"]}: multiple linked PRs; reconciliation and work are paused')
        last = self.store.db.execute('''SELECT run_id,payload FROM jobs WHERE item=?
            AND agent NOT IN ('ingest','reconcile','funnel-ingest','funnel-reconcile') AND finished IS NOT NULL
            ORDER BY finished DESC LIMIT 1''', (str(item['number']),)).fetchone()
        if last:
            previous = json.loads(last['payload']).get('labels', [])
            job['previous_phase'] = ','.join(label for label in previous if label.startswith('agent/')
                                             and not label.startswith('agent/scope/')
                                             and label not in ('agent/allowed', 'agent/execution-record'))
        inputs = [job['fingerprint'],
                  [(p['number'], p.get('merged_at'), p['state'], p.get('head', {}).get('sha')) for p in matches],
                  last['run_id'] if last else None]
        job['source_fingerprint'] = fingerprint(inputs)
        job['fingerprint'] = job['source_fingerprint']
        prior = self.store.get(job['key'])
        if prior:
            payload = json.loads(prior['payload'])
            same_inputs = payload.get('source_fingerprint') == job['source_fingerprint']
            if 'source_fingerprint' not in payload:
                # Recognize the previous clock-bucket format without replaying
                # every successful or still-running sync during this upgrade.
                same_inputs = prior['fingerprint'] == fingerprint(
                    inputs + [int(prior['started'] // self.args.lifecycle_interval)])
            if same_inputs:
                job['fingerprint'] = prior['fingerprint']
                if (prior['status'] == 'succeeded' and prior['finished'] is not None
                        and time.time() - prior['finished'] >= self.args.reconcile_interval):
                    # Stable through discovery/restarts until this check starts;
                    # the successful attempt resets the elapsed-time schedule.
                    job['fingerprint'] = fingerprint([job['source_fingerprint'], prior['run_id']])
        return job

    def authorized(self, job):
        if job['agent'] in ('funnel-ingest', 'funnel-reconcile'):
            job = dict(job, agent=job['agent'].removeprefix('funnel-'))
        if job['agent'] == 'ingest':
            return True  # The intake harness enforces the gate on both origins.
        item = self.github.current(job['item'])
        if not {self.args.scope, 'agent/allowed'} <= set(self.github.labels(item)):
            report(f'{job["key"]}: authorization or scope removed; skipping')
            return False
        if job['agent'] != 'reconcile' and item['state'] != 'open':
            return False
        if self.github.labels(item) != job['labels']:
            report(f'{job["key"]}: labels changed; rediscovering before launch')
            return False
        if job['agent'] not in ('ingest', 'reconcile'):
            if self.github.phase_job(self.args.scope, job['agent'], item)['fingerprint'] != job['fingerprint']:
                report(f'{job["key"]}: inputs changed; rediscovering before launch')
                return False
        return True

    def reap(self):
        completed = False
        for key, proc in list(self.active.items()):
            row = self.store.get(key)
            if proc is not None:
                proc.poll()  # Reap our own children when they exit.
            if self.store.running(row['run_id']):
                continue
            if row['status'] == 'running':
                with self.store.db:
                    self.store.db.execute("UPDATE jobs SET status='uncertain',error='Worker exited without recording an outcome' WHERE key=?", (key,))
                report(f'{key}: outcome unknown; inspect {row["log"]} before --retry-uncertain {key}')
            elif row['status'] == 'failed':
                report(f'{key}: FAILED ({row["error"]}); retry after {time.ctime(row["next_at"])}')
            else:
                report(f'{key}: completed; log: {row["log"]}')
            del self.active[key]
            completed = True
        return completed

    def launch(self, job):
        job = dict(job, repo=self.args.repo)
        if job['agent'] in ('ingest', 'reconcile'):
            job['jira_auth'] = self.args.jira_selection
        job['command'] = ['fullsend', 'run', job['agent'], '--fullsend-dir', self.args.fullsend_dir,
                          '--target-repo', self.args.target_repo]
        if self.args.model:
            job['command'] += ['--model', self.args.model]
        # Env files have already been sourced by the entry point. Passing them
        # again could override the coordinator's explicit gateway selection.
        safe_item = re.sub(r'[^A-Za-z0-9_.-]', '_', job['item'])
        log = self.args.log_dir / f'{job["agent"]}-{safe_item}-{time.time_ns()}.log'
        run_id = self.store.start(job, log, time.time())
        try:
            with self.store.run_lock(run_id) as run_lock, log.open('a') as output:
                fcntl.flock(run_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                proc = subprocess.Popen([sys.executable, str(Path(__file__).resolve()), '--worker',
                                         str(self.store.path), job['key'], run_id, str(run_lock.fileno()),
                                         str(self.args.backoff), str(self.args.backoff_max)],
                                        stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT,
                                        start_new_session=True, pass_fds=(run_lock.fileno(),))
        except OSError:
            self.store.finish(job['key'], run_id, 127, self.args.backoff, self.args.backoff_max)
            raise
        self.active[job['key']] = proc
        report(f'Started {job["key"]}; log: {log}')

    def display(self):
        counts = dict(self.store.db.execute('SELECT status,count(*) FROM jobs WHERE scope=? GROUP BY status', (self.args.scope,)))
        if ui.interactive:
            # Let the child own terminal output while it prints browser URLs.
            if self.auth.login:
                return
            now = time.time()
            manager = self.auth.manager
            gateway = ((manager.state or {}).get('name') or manager.name or 'current OpenShell selection') if manager else 'current selection'
            rows = [dict(row) for row in self.store.db.execute(
                "SELECT * FROM jobs WHERE scope=? OR status='running'", (self.args.scope,))]
            ready = sum(self.store.eligible(job, now) for job in self.candidates)
            if self.discovering:
                mode, message = 'DISCOVERING', 'Reading the backlog and checking which stages are eligible.'
            elif self.auth.status.startswith('paused'):
                mode, message = 'PAUSED', self.auth.status.removeprefix('paused: ')
                if self.auth.next_at > now:
                    message += f'. Next connection check in {int(self.auth.next_at-now)+1}s.'
            elif counts.get('uncertain'):
                mode, message = 'NEEDS ATTENTION', 'An earlier run has an unknown outcome. Inspect its log before retrying.'
            elif self.active:
                mode, message = 'DISPATCHING', f'{len(self.active)} worker(s) running. {ready} candidate(s) ready for a free slot.'
            else:
                delay = max(0, int(self.next_discovery-now))
                mode, message = 'WATCHING', f'{ready} candidate(s) ready. Next backlog check in {delay}s.'
            ui.dashboard(scope=self.args.scope, mode=mode, message=message, rows=rows,
                         capacity=self.args.concurrency, gateway=gateway,
                         detail=self.errors[0] if self.errors else f'Logs: {self.args.log_dir}',
                         footer='Ctrl-C detaches + prints logs. Restart resumes monitoring.')
            return
        summary = f'gateway auth: {self.auth.status} | active {len(self.active)}/{self.args.concurrency} | jobs {counts} | candidates {len(self.candidates)}'
        if summary != self.last_display:
            print(summary, flush=True)
            self.last_display = summary

    def show_exit_logs(self):
        show_exit_logs(self.store, self.args)

    def run(self):
        next_poll = 0
        attempted = set()
        def attempt_key(job):
            return (job['key'], job['fingerprint']) if job['agent'] == 'reconcile' else job['key']
        try:
            while True:
                completed = self.reap()
                if completed and (self.args.watch or self.args.lifecycle == 'jira'):
                    next_poll = 0
                now = time.time()
                self.maintain_auth(now)
                if now >= next_poll and (self.args.watch or next_poll == 0):
                    self.discovering = True
                    self.display()
                    try:
                        self.discover()
                    except DispatchError as exc:
                        self.errors = [str(exc)]
                        self.candidates = []  # Never schedule a stale snapshot after discovery fails.
                        report(f'Discovery failed: {exc}; retrying in {self.args.poll_interval}s')
                    finally:
                        self.discovering = False
                    next_poll = time.time() + self.args.poll_interval
                    self.next_discovery = next_poll
                    if self.args.dry_run:
                        for job in self.candidates:
                            print(f'WOULD run {job["key"]} {job["url"]}')
                        return int(bool(self.errors))
                # Do not run two phases of the same issue concurrently. Oldest
                # attempt first prevents a persistently eligible item starving others.
                ordered = sorted(self.candidates, key=lambda j: (self.store.get(j['key']) or {'started': 0})['started'])
                for job in ordered:
                    if len(self.active) >= self.args.concurrency:
                        break
                    if not self.args.watch and attempt_key(job) in attempted:
                        continue
                    if any(key.split(':', 1)[0] == job['item'] for key in self.active):
                        continue
                    if not self.store.eligible(job, time.time()):
                        continue
                    if not self.auth.ready(time.time()):
                        break
                    try:
                        if not self.authorized(job):
                            next_poll = 0
                            continue
                    except DispatchError as exc:
                        report(f'Cannot verify {job["key"]}: {exc}; retrying at the next poll')
                        self.errors.append(str(exc))
                        attempted.add(attempt_key(job))
                        self.candidates = []
                        break
                    self.launch(job)
                    attempted.add(attempt_key(job))
                self.display()
                pending = any(attempt_key(j) not in attempted and self.store.eligible(j, time.time()) for j in self.candidates)
                if not self.args.watch and not self.active and not pending and next_poll != 0:
                    failures = self.store.db.execute("SELECT count(*) FROM jobs WHERE scope=? AND status IN ('failed','uncertain')", (self.args.scope,)).fetchone()[0]
                    return int(bool(failures or self.errors))
                time.sleep(0.5)
        finally:
            ui.clear()
            try:
                self.auth.close()
                if self.active:
                    report('Dispatcher detached. Workers continue in the background. Restart to resume monitoring and scheduling.')
            finally:
                self.show_exit_logs()


def show_exit_logs(store, args):
    """Leave full copyable paths in scrollback, outside the clipped panel."""
    failed = list(store.db.execute('''SELECT r.job_key,r.log,r.exit_code
        FROM runs r JOIN jobs j ON j.key=r.job_key
        WHERE j.scope=? AND r.exit_code IS NOT NULL AND r.exit_code != 0
        ORDER BY r.started''', (args.scope,)))
    pending = list(store.db.execute('''SELECT key,log,status FROM jobs
        WHERE scope=? AND status IN ('running','uncertain') AND log IS NOT NULL
        ORDER BY started''', (args.scope,)))
    ui.clear()
    if failed:
        print('\nFailed run logs:', file=sys.stderr)
        for row in failed:
            print(f'{row["job_key"]} (exit {row["exit_code"]})\n{row["log"]}', file=sys.stderr)
    if pending:
        print('\nRunning or unresolved run logs:', file=sys.stderr)
        for row in pending:
            print(f'{row["key"]} ({row["status"]})\n{row["log"]}', file=sys.stderr)
    sys.stderr.flush()


def positive(value):
    try:
        parsed = int(value)
        if parsed > 0:
            return parsed
    except ValueError:
        pass
    raise argparse.ArgumentTypeError('must be a positive integer')


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('scope')
    parser.add_argument('--watch', action='store_true')
    parser.add_argument('--plain', action='store_true', help='disable the live terminal dashboard and color')
    parser.add_argument('--dry-run', action='store_true')
    parser.add_argument('--env-file', action='append', default=[],
                        help='optional exports-only file; overrides inherited environment variables')
    parser.add_argument('--gateway')
    parser.add_argument('--api-url', default=os.environ.get('HYPERSHELL_API_URL'),
                        help='HyperShell API for gateway setup (saved API or https://api.hyp0.infra.hypershell.app)')
    parser.add_argument('--gateway-state', type=Path, help='local gateway state (default: TARGET_REPO/.hypershell/.funnel-gateway.json)')
    parser.add_argument('--gateway-login-timeout', type=positive, default='120', help='seconds before offering recovery from a stuck browser login')
    parser.add_argument('--gateway-ready-timeout', type=positive, default='600', help='seconds to wait for a created gateway before offering recovery again')
    parser.add_argument('--model', default=os.environ.get('FULLSEND_MODEL'))
    parser.add_argument('--concurrency', type=positive, default=os.environ.get('CONCURRENCY', '3'))
    parser.add_argument('--poll-interval', type=positive, default=os.environ.get('POLL_INTERVAL', '30'))
    parser.add_argument('--auth-retry', type=positive, default=os.environ.get('GATEWAY_AUTH_RETRY_SECONDS', '30'))
    parser.add_argument('--auth-check-interval', type=positive,
                        default=os.environ.get('GATEWAY_AUTH_CHECK_SECONDS', '60'),
                        help='check/refresh gateway credentials periodically, including while workers run (default: 60s)')
    parser.add_argument('--backoff', type=positive, default=os.environ.get('FAILURE_BACKOFF_SECONDS', '60'))
    parser.add_argument('--backoff-max', type=positive, default=os.environ.get('FAILURE_BACKOFF_MAX_SECONDS', '3600'))
    parser.add_argument('--lifecycle', choices=('jira', 'github-only'), default='jira',
                        help='jira runs intake and reconciliation; github-only explicitly omits tracker sync')
    parser.add_argument('--jira-auth', choices=('auto', 'acli', 'api-token'),
                        default=os.environ.get('JIRA_AUTH_MODE', 'auto'),
                        help='auto uses API-token credentials only when both email and token are set, otherwise the active acli OAuth login')
    parser.add_argument('--lifecycle-interval', type=positive, default='900',
                        help='seconds between checks for newly scoped issues (default: 900)')
    parser.add_argument('--reconcile-interval', type=positive, default='21600',
                        help='seconds after a successful sync before checking unchanged work for Jira drift (default: 21600, six hours)')
    parser.add_argument('--repo', default=os.environ.get('REPOSITORY', 'openshift-online/hypershell'))
    parser.add_argument('--fullsend-dir', default=os.environ.get('FULLSEND_DIR', '.fullsend'))
    parser.add_argument('--target-repo', default=os.environ.get('TARGET_REPO', os.getcwd()))
    parser.add_argument('--state-dir', type=Path, default=Path(os.environ.get('XDG_STATE_HOME', str(Path.home()/'.local/state')))/'hypershell/funnel')
    parser.add_argument('--log-dir', type=Path, default=os.environ.get('LOGDIR'))
    parser.add_argument('--status', action='store_true', help='show persisted jobs without starting work')
    parser.add_argument('--retry', metavar='ISSUE:AGENT', help='request another attempt for a failed job without waiting for backoff')
    parser.add_argument('--retry-uncertain', metavar='ITEM:AGENT', help='retry only after verifying that the previous remote run has stopped')
    args = parser.parse_args(argv)
    if args.retry and (args.status or args.dry_run or args.retry_uncertain):
        parser.error('--retry cannot be combined with --status, --dry-run, or --retry-uncertain')
    if not args.scope.startswith('agent/scope/'):
        parser.error('scope must start with agent/scope/')
    if args.backoff_max < args.backoff:
        parser.error('--backoff-max must be at least --backoff')
    args.state_dir = args.state_dir.expanduser().resolve()/fingerprint(args.repo)[:16]
    args.log_dir = (args.log_dir or args.state_dir/'logs').expanduser().resolve()
    args.fullsend_dir = str(Path(args.fullsend_dir).resolve())
    args.target_repo = str(Path(args.target_repo).resolve())
    args.gateway_state = (args.gateway_state or Path(args.target_repo)/'.hypershell/.funnel-gateway.json').expanduser().resolve()
    return args


def main():
    if len(sys.argv) > 1 and sys.argv[1] == '--worker':
        return worker(*sys.argv[2:5], *map(int, sys.argv[5:8]))
    args = parse_args()
    ui.plain = args.plain
    if args.gateway:
        os.environ['OPENSHELL_GATEWAY'] = args.gateway
        os.environ.pop('OPENSHELL_GATEWAY_ENDPOINT', None)
    store = Store(args.state_dir/'state.sqlite3')
    if args.status:
        for row in store.db.execute('SELECT key,status,failures,next_at,log,error FROM jobs ORDER BY key'):
            print(json.dumps(dict(row)))
        return 0
    dependencies = ['gh', 'fullsend'] + ([] if args.dry_run else ['openshell'])
    if args.lifecycle == 'jira' and not args.dry_run:
        dependencies.append('jq')
    for dependency in dependencies:
        if not shutil.which(dependency):
            raise DispatchError(f'missing dependency: {dependency}')
    if not args.dry_run:
        prepare_inference(args.fullsend_dir)
    if args.lifecycle == 'jira' and not args.dry_run:
        args.jira_selection = selected_auth(args.jira_auth)
        os.environ['JIRA_BASE_URL'] = args.jira_selection['site_url']
        report(f'Jira authentication: {args.jira_selection["mode"]} ({args.jira_selection["site_url"]})')
    elif args.lifecycle == 'github-only':
        report('GitHub-only mode: Jira intake and reconciliation are disabled.')
    args.log_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    # Keep the original lock as a compatibility gate for older processes.
    # New workers inherit only per-run locks, so restarting can adopt them.
    with (args.state_dir/'dispatcher.lock').open('a') as lock:
        waiting = False
        last_snapshot = None
        try:
            while True:
                try:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    break
                except BlockingIOError:
                    if not waiting:
                        report('Monitoring existing work. Another dispatcher or older workers hold the scheduling lock.\nScheduling resumes automatically when they release it; no work will be duplicated.')
                        waiting = True
                    rows = [dict(row) for row in store.db.execute('SELECT * FROM jobs ORDER BY started DESC')]
                    if ui.interactive:
                        ui.dashboard(scope=args.scope, mode='MONITORING',
                                     message='Scheduling will resume when the existing owner releases its lock.',
                                     rows=rows, capacity=args.concurrency, gateway='Existing worker connections',
                                     footer='Ctrl-C detaches and prints log paths. Workers keep running.')
                    else:
                        snapshot = [(row['key'], row['status']) for row in rows]
                        if snapshot != last_snapshot:
                            report('Existing work: ' + (', '.join(f'{key}: {status}' for key, status in snapshot) or 'no recorded workers'))
                            last_snapshot = snapshot
                    time.sleep(0.5)
        except BaseException:
            show_exit_logs(store, args)
            raise
        finally:
            ui.clear()
        if waiting:
            report('Scheduling lock acquired. Resuming dispatch.')
        for key in store.recover():
            report(f'{key}: outcome uncertain. Inspect the remote run, then use --retry-uncertain {key} if safe.')
        if args.retry_uncertain:
            store.retry_uncertain(args.retry_uncertain)
        if not os.environ.get('GH_TOKEN'):
            os.environ['GH_TOKEN'] = command(['gh', 'auth', 'token']).strip()
        os.environ.setdefault('CLOUD_ML_REGION', os.environ.get('ANTHROPIC_VERTEX_REGION', ''))
        return Coordinator(args, store, lock.fileno()).run()


if __name__ == '__main__':
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(130))
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(130)
    except (DispatchError, JiraAuthError, GatewaySetupError, OSError, sqlite3.Error) as error:
        report(f'error: {error}')
        sys.exit(1)
