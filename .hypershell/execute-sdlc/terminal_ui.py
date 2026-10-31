"""Small dependency-free terminal view; redirected output stays plain text."""
import os
import re
import shutil
import sys
import time
import textwrap
import json
from pathlib import Path


def clean(value):
    text = re.sub(r'\x1b\[[0-?]*[ -/]*[@-~]', '', str(value))
    return ''.join(c for c in text if c == '\n' or ord(c) >= 32 and ord(c) != 127)


class TerminalUI:
    def __init__(self, stream=None):
        self._stream = stream
        self.lines = 0
        self.previous = None
        self.plain = False

    @property
    def stream(self):
        return self._stream or sys.stderr

    @property
    def interactive(self):
        return not self.plain and self.stream.isatty() and os.environ.get('TERM') != 'dumb'

    def size(self):
        try:
            return os.get_terminal_size(self.stream.fileno())
        except (OSError, AttributeError):
            return shutil.get_terminal_size((100, 24))

    def dashboard(self, *, scope, mode, message, rows, capacity, gateway, footer, detail=''):
        width = max(12, min(100, self.size().columns - 3))
        now = time.time()
        counts = {state: sum(row['status'] == state for row in rows)
                  for state in ('running', 'succeeded', 'failed', 'uncertain')}
        active = counts['running']
        slots = min(capacity, 16)
        meter = '#' * min(active, slots) + '.' * max(0, slots-active)
        lines = ['HYPERSHELL / FULLSEND',
                 f'{scope}  |  {time.strftime("%H:%M:%S")}', '=' * width,
                 mode]
        lines.extend(textwrap.wrap(clean(message), width)[:2])
        lines += ['', f'Workers [{meter}] {active}/{capacity}   Done {counts["succeeded"]}   Failed {counts["failed"]}   Uncertain {counts["uncertain"]}',
                  f'Gateway  {gateway}', '', 'WORK / STATUS', '-' * width]
        order = {'running': 0, 'uncertain': 1, 'failed': 2, 'succeeded': 3}
        ordered = sorted(rows, key=lambda row: (order.get(row['status'], 4), -row['started']))
        # Reserve room for the next action and exit hint even on short terminals.
        limit = max(1, min(6, (self.size().lines-len(lines)-6)//2))
        for row in ordered[:limit]:
            scope_job = row['item'].startswith('agent/scope/')
            item = 'Scope' if scope_job else '#' + row['item']
            payload = json.loads(row.get('payload') or '{}')
            title = payload.get('title') or ('Discover and link Jira / GitHub work' if scope_job else 'Title not yet loaded')
            lines.append(item + '  ' + middle_ellipsis(title, max(1, width-len(item)-2)))
            name = row['agent'].removeprefix('funnel-')
            state = row['status']
            if state == 'running':
                status = run_activity(row.get('log')) + ' | ' + duration(now-row['started']) + ' elapsed'
            elif state == 'failed':
                status = 'Failed | Retry in ' + duration(row['next_at']-now) if row['next_at'] > now else 'Failed | Ready to retry'
            elif state == 'uncertain':
                status = 'Unknown outcome | Inspect log'
            else:
                status = 'Completed'
            prefix = '  ' + name + ' | '
            lines.append(prefix + status)
        if not rows:
            lines.append('No work recorded yet.')
        if len(rows) > limit:
            lines.append(f'+ {len(rows)-limit} more recorded jobs; use --status to inspect all')
        lines += ['', detail or 'Updates automatically. Failed work retries with backoff.', '-' * width, footer]
        self.draw(lines)

    def color(self, text, code):
        if self.interactive and 'NO_COLOR' not in os.environ:
            return f'\033[{code}m{text}\033[0m'
        return text

    def clear(self):
        if self.lines:
            self.stream.write(f'\033[{self.lines}A\r\033[J')
            self.stream.flush()
        self.lines, self.previous = 0, None

    def event(self, message):
        self.clear()
        message = clean(message)
        if self.interactive:
            stamp = self.color(time.strftime('%H:%M:%S'), '2')
            message = stamp + '  ' + message.replace('\n', '\n          ')
        print(message, file=self.stream, flush=True)

    def heading(self, title, description=''):
        self.clear()
        print('\n' + self.color(clean(title), '1;36'), file=self.stream)
        if description:
            print(clean(description), file=self.stream)
        self.stream.flush()

    def options(self, entries):
        self.clear()
        for key, title, description in entries:
            print(f'  {self.color(key, "1;36")}  {clean(title)}', file=self.stream)
            print(f'     {self.color(clean(description), "2")}', file=self.stream)
        print(file=self.stream, flush=True)

    def draw(self, lines):
        if not self.interactive:
            return
        size = self.size()
        width = max(1, size.columns - 1)
        visible = [clean(line).replace('\n', ' ') for line in lines]
        visible = [line if len(line) <= width else (line[:max(0, width-3)]+'...')[:width] for line in visible]
        height = max(1, size.lines - 2)
        if len(visible) > height:
            visible = visible[:height-1] + ['(resize terminal for more details)'[:width]]
        if visible == self.previous:
            return
        self.clear()
        for index, line in enumerate(visible):
            style = '0'
            if index == 0 or line in ('DISPATCHING', 'DISCOVERING', 'WATCHING', 'WORK / STATUS'):
                style = '1;36'
            elif ' | Failed | ' in line or ' | Unknown outcome | ' in line:
                style = '1;31'
            elif line in ('MONITORING', 'PAUSED', 'NEEDS ATTENTION'):
                style = '1;33'
            elif line.endswith(' | Completed'):
                style = '32'
            elif line.startswith('  ') and ' | ' in line:
                style = '36'
            elif line.startswith(('#', 'Scope  ')):
                style = '1'
            elif line.startswith(('---', '===', 'Ctrl-C', '+ ')):
                style = '2'
            print(self.color(line, style) if style != '0' else line, file=self.stream)
        self.lines, self.previous = len(visible), visible
        self.stream.flush()


def middle_ellipsis(text, width):
    text = clean(text).replace('\n', ' ')
    if len(text) <= width:
        return text
    if width <= 3:
        return '.' * max(0, width)
    left = (width - 3 + 1)//2
    right = width - 3 - left
    return text[:left] + '...' + (text[-right:] if right else '')


def run_activity(log):
    """Show only recognized lifecycle markers, never arbitrary log/secret text."""
    if not log:
        return 'Running (no activity reported)'
    try:
        with Path(log).open('rb') as stream:
            stream.seek(0, 2)
            stream.seek(max(0, stream.tell()-32768))
            tail = clean(stream.read().decode('utf-8', errors='replace'))
    except OSError:
        return 'Running (log unavailable)'
    markers = [('Cleaning up sandbox', 'Cleaning up'),
               ('Collecting OpenShell logs', 'Collecting logs'),
               ('Post-loop validation', 'Validating results'),
               ('Extracting target repo', 'Extracting changes'),
               ('Extracting transcripts', 'Extracting transcripts'),
               ('Extracting output files', 'Extracting results'),
               ('Agent running (', 'Agent running'), ('Running agent', 'Agent running'),
               ('Checking GitHub API', 'Checking GitHub access'),
               ('pre-agent security scan', 'Security scan'),
               ('Copying project code', 'Copying project'),
               ('Bootstrapping sandbox', 'Bootstrapping sandbox'),
               ('Sandbox bootstrapped', 'Sandbox bootstrapped'),
               ('Creating sandbox', 'Creating sandbox'),
               ('Sandbox created', 'Sandbox created'),
               ('Ensuring provider', 'Configuring providers'),
               ('Importing profile', 'Configuring profiles'),
               ('Checking gateway', 'Checking gateway'),
               ('Loading harness', 'Loading harness')]
    for line in reversed(tail.splitlines()):
        if line.lstrip().startswith(('⚙', '💬')):
            return 'Agent working'
        for marker, label in markers:
            if marker == 'Running agent' and 'Running agent:' in line:
                continue  # Fullsend's startup banner is not agent execution.
            if marker in line:
                return label
    return 'Running (no activity reported)'


def duration(seconds):
    seconds = max(0, int(seconds))
    if seconds < 60:
        return f'{seconds}s'
    minutes, seconds = divmod(seconds, 60)
    if minutes < 60:
        return f'{minutes}m {seconds:02d}s'
    hours, minutes = divmod(minutes, 60)
    return f'{hours}h {minutes:02d}m'


ui = TerminalUI()
