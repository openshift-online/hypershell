"""Mechanical discovery of authorized records that need canonical linking."""
import hashlib
import json
import re
import time

from reconcile_probe import ProbeError, canonical_key


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()


def labels(item):
    return sorted(label['name'] if isinstance(label, dict) else label for label in item['labels'])


def description_text(value):
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        text = value.get('text', '') + ''.join(description_text(c) for c in value.get('content', []))
        return text + ('\n' if value.get('type') in ('paragraph', 'heading') else '')
    return ''


def pending_items(scope, github, jira):
    github = [i for i in github if {scope, 'agent/allowed'} <= set(labels(i))]
    pairs = {}
    for item in github:
        try:
            key = canonical_key(item.get('body'))
        except ProbeError:
            key = None
        if key and 'agent/execution-record' in labels(item):
            pairs.setdefault(key, []).append(item)
    jira = [i for i in jira if i['key'] in pairs or {scope, 'agent/allowed'} <= set(i['fields'].get('labels', []))]
    healthy, pending = set(), []
    for item in jira:
        matches = pairs.get(item['key'], [])
        backlinks = re.findall(r'^https://github\.com/[\w.-]+/[\w.-]+/issues/[0-9]+$',
                               description_text(item['fields'].get('description')), re.MULTILINE)
        if len(matches) == 1 and backlinks == [matches[0]['html_url']]:
            healthy.add(matches[0]['number'])
        else:
            fields = dict(item['fields'])
            fields['labels'] = sorted(fields.get('labels', []))
            pending.append(['jira', item['key'], fields])
    for item in github:
        if item['number'] not in healthy:
            pending.append(['github', item['number'], item.get('title'), item.get('body'), item['state'],
                            [l for l in labels(item) if l in (scope, 'agent/allowed', 'agent/execution-record')]])
    return sorted(pending, key=lambda value: (value[0], str(value[1])))


class IntakeProbe:
    def __init__(self, db, scope, read):
        self.db, self.scope, self.read = db, scope, read
        db.execute('''CREATE TABLE IF NOT EXISTS intake_observations (
            scope TEXT PRIMARY KEY, checked_at REAL NOT NULL DEFAULT 0, digest TEXT,
            pending INTEGER NOT NULL DEFAULT 0, retry_at REAL NOT NULL DEFAULT 0, error TEXT)''')
        db.commit()

    def observe(self, github, interval, now=None):
        now = time.time() if now is None else now
        row = self.db.execute('SELECT * FROM intake_observations WHERE scope=?', (self.scope,)).fetchone()
        if row:
            if row['retry_at'] > now:
                raise ProbeError(row['error'])
            if not row['error'] and now - row['checked_at'] < interval:
                return row['digest'] if row['pending'] else None
        try:
            pending = pending_items(self.scope, github, self.read(github))
        except (ProbeError, KeyError, TypeError, ValueError):
            with self.db:
                self.db.execute('''INSERT INTO intake_observations(scope,retry_at,error) VALUES(?,?,?)
                    ON CONFLICT(scope) DO UPDATE SET retry_at=excluded.retry_at,error=excluded.error''',
                    (self.scope, now+60, 'Intake discovery failed; retrying after 60s'))
            raise ProbeError('Intake discovery failed; check Jira/GitHub access and canonical links') from None
        fingerprint = digest(pending)
        with self.db:
            self.db.execute('''INSERT INTO intake_observations(scope,checked_at,digest,pending) VALUES(?,?,?,?)
                ON CONFLICT(scope) DO UPDATE SET checked_at=excluded.checked_at,digest=excluded.digest,
                pending=excluded.pending,retry_at=0,error=NULL''', (self.scope, now, fingerprint, bool(pending)))
        return fingerprint if pending else None
