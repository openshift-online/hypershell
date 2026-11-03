"""Read-only Jira observations for local scheduling; stores hashes, never credentials."""
import hashlib
import json
import re
import time
from datetime import datetime
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, build_opener, HTTPRedirectHandler

from jira_auth import JiraAuthError, snapshot


class ProbeError(Exception):
    pass


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def canonical_key(body):
    keys = set(re.findall(r'^\s*([A-Z][A-Z0-9_]*-[0-9]+)\s*$', body or '', re.MULTILINE))
    if len(keys) > 1:
        raise ProbeError('Ambiguous canonical Jira key; inspect the execution record')
    return next(iter(keys), None)


class JiraReader:
    def __init__(self, selection, scope, state_dir):
        self.selection, self.scope = selection, scope
        self.lock = Path(state_dir).parent/'acli-auth.lock'
        self.credentials = None
        self.failure = None
        self.failed_at = 0

    def __call__(self, key):
        if self.failure:
            if time.time() - self.failed_at < 60:
                raise ProbeError(self.failure)
            self.credentials, self.failure = None, None
        try:
            if self.credentials is None:
                self.credentials = snapshot(self.selection, self.scope, self.lock)
            credentials = self.credentials
            base = credentials['api_base_url']
            if not re.fullmatch(r'https://(?:[A-Za-z0-9-]+\.atlassian\.net|api\.atlassian\.com/ex/jira/[A-Za-z0-9-]+)', base):
                raise ProbeError('Invalid Jira API base for drift check')
            opener = build_opener(NoRedirect())

            def get(suffix):
                request = Request(base + '/rest/api/3/issue/' + key + suffix,
                                  headers={'Authorization': credentials['authorization'], 'Accept': 'application/json'})
                with opener.open(request, timeout=30) as response:
                    return json.load(response)

            issue = get('?fields=summary,description,priority,labels,parent,status,assignee,issuetype,updated')
            links = get('/remotelink')
            fields = issue['fields']
            updated = fields.pop('updated', None)
            # Label and remote-link ordering is not a content change.
            fields['labels'] = sorted(fields.get('labels') or [])
            links = sorted(links, key=lambda link: json.dumps(link, sort_keys=True))
            digest = hashlib.sha256(json.dumps([fields, links], sort_keys=True).encode()).hexdigest()
            return digest, updated
        except (JiraAuthError, HTTPError, URLError, OSError, ValueError, KeyError, TypeError):
            # Do not expose headers, response bodies, or credential-command output.
            self.failed_at = time.time()
            self.failure = 'Jira drift check failed; check authentication, permissions and network access'
            raise ProbeError(self.failure) from None


class ReconcileProbe:
    def __init__(self, db, read):
        self.db, self.read = db, read
        db.execute('''CREATE TABLE IF NOT EXISTS reconcile_observations (
            key TEXT PRIMARY KEY, jira_key TEXT NOT NULL, digest TEXT,
            generation INTEGER NOT NULL DEFAULT 0, checked_at REAL NOT NULL DEFAULT 0,
            retry_at REAL NOT NULL DEFAULT 0, error TEXT)''')
        db.commit()

    def generation(self, job, body, prior, interval, now=None, baseline=False):
        now = time.time() if now is None else now
        key = canonical_key(body)
        if key is None:
            return 0  # No canonical Jira counterpart; GitHub inputs still trigger sync.
        row = self.db.execute('SELECT * FROM reconcile_observations WHERE key=?', (job,)).fetchone()
        if row and row['jira_key'] == key and not baseline:
            if row['retry_at'] > now:
                raise ProbeError(row['error'])
            if not row['error'] and now - row['checked_at'] < interval:
                return row['generation']
        try:
            digest, updated = self.read(key)
        except ProbeError:
            with self.db:
                self.db.execute('''INSERT INTO reconcile_observations(key,jira_key,retry_at,error) VALUES(?,?,?,?)
                    ON CONFLICT(key) DO UPDATE SET retry_at=excluded.retry_at,error=excluded.error''',
                    (job, key, now+60, 'Jira drift check failed; retrying after 60s'))
            raise
        generation = row['generation'] if row else 0
        if not baseline:
            if row and (row['jira_key'] != key or row['digest'] != digest):
                generation += 1
            elif not row and prior and prior['status'] == 'succeeded' and updated:
                # Upgrade existing successful records without a blanket replay.
                # A Jira edit after their last sync still needs reconciliation.
                try:
                    if datetime.fromisoformat(updated.replace('Z', '+00:00')).timestamp() > prior['finished']:
                        generation += 1
                except (ValueError, TypeError):
                    raise ProbeError('Invalid Jira update timestamp; cannot establish reconciliation baseline') from None
        with self.db:
            self.db.execute('''INSERT INTO reconcile_observations(key,jira_key,digest,generation,checked_at) VALUES(?,?,?,?,?)
                ON CONFLICT(key) DO UPDATE SET jira_key=excluded.jira_key,digest=excluded.digest,
                generation=excluded.generation,checked_at=excluded.checked_at,retry_at=0,error=NULL''',
                (job, key, digest, generation, now))
        return generation
