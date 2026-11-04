"""Declarative local preferences. Never source this file as shell code."""
import fcntl
import json
import os
from pathlib import Path
import re
import stat
import tempfile
from urllib.parse import urlsplit

# key, label, effect, kind, choices, help
SPECS = [
 ('concurrency','Worker limit','Immediate','number',[], 'Lowering the limit lets existing workers finish.'),
 ('poll_interval','GitHub discovery (seconds)','Immediate','number',[], ''),
 ('lifecycle_interval','Intake checks (seconds)','Immediate','number',[], ''),
 ('reconcile_interval','Jira drift checks (seconds)','Immediate','number',[], ''),
 ('auth_check_interval','Gateway checks (seconds)','Immediate','number',[], ''),
 ('auth_retry','Gateway retry (seconds)','Immediate','number',[], ''),
 ('model','Model override','Next sandbox','text',[], 'Empty uses each harness model.'),
 ('backoff','Initial failure backoff (seconds)','Next sandbox','number',[], 'Existing retry deadlines are preserved.'),
 ('backoff_max','Maximum failure backoff (seconds)','Next sandbox','number',[], ''),
 ('ANTHROPIC_VERTEX_PROJECT_ID','Vertex project','Next sandbox','text',[], ''),
 ('CLOUD_ML_REGION','Vertex region','Next sandbox','text',[], ''),
 ('GOOGLE_APPLICATION_CREDENTIALS','Google credentials path','Next sandbox','file',[], 'Credential contents stay outside this file.'),
 ('gateway_login_timeout','Sign-in timeout (seconds)','Next sign-in','number',[], ''),
 ('gateway_ready_timeout','Gateway ready timeout (seconds)','Next sign-in','number',[], ''),
 ('scope','Scope','Restart required','scope',[], 'The scope label must be registered before launch.'),
 ('repo','GitHub repository','Restart required','repo',[], ''),
 ('lifecycle','Lifecycle','Restart required','choice',['jira','github-only'], ''),
 ('jira_auth','Jira authentication mode','Restart required','choice',['auto','acli','api-token'], ''),
 ('gateway','Gateway selection','Restart required','text',[], 'Use Sign in for connection recovery.'),
 ('api_url','HyperShell API URL','Restart required','url',[], ''),
 ('fullsend_dir','Harness directory','Restart required','directory',[], ''),
 ('target_repo','Target checkout','Restart required','directory',[], ''),
 ('state_dir','State root','Restart required','path',[], 'The effective directory includes the repository identifier.'),
 ('log_dir','Log directory override','Restart required','path',[], 'Empty uses the state directory.'),
 ('gateway_state','Gateway state file override','Restart required','path',[], ''),
]
ENV_KEYS = {'ANTHROPIC_VERTEX_PROJECT_ID','CLOUD_ML_REGION','GOOGLE_APPLICATION_CREDENTIALS'}


def config_path():
    return Path(os.path.abspath(os.path.expanduser(os.environ.get('FULLSEND_CONFIG_FILE') or
        str(Path(os.environ.get('XDG_CONFIG_HOME',str(Path.home()/'.config')))/'hypershell/fullsend.json'))))


def validate(key, value):
    spec = next((s for s in SPECS if s[0] == key), None)
    if spec is None:
        raise ValueError('Unknown preference key')
    if not isinstance(value, (str,int)) or isinstance(value,bool):
        raise ValueError('Setting must be text or an integer')
    text = str(value)
    if any(c in text for c in ('\n','\r','\x00')) or len(text)>4096:
        raise ValueError('Setting must be a single line of at most 4096 characters')
    kind = spec[3]
    if kind == 'number':
        if not re.fullmatch(r'[0-9]+',text) or not 1 <= int(text) <= 2147483647:
            raise ValueError('Enter a positive integer (at most 2147483647)')
        return int(text)
    if kind == 'choice' and text not in spec[4]:
        raise ValueError('Choose one of: '+', '.join(spec[4]))
    if kind == 'scope' and not re.fullmatch(r'agent/scope/[^\s]+',text):
        raise ValueError('Scope must identify an agent/scope/ label')
    if kind == 'repo' and not re.fullmatch(r'[\w.-]+/[\w.-]+',text):
        raise ValueError('Repository must be owner/name')
    if kind == 'url' and text:
        parsed=urlsplit(text)
        if not re.fullmatch(r'https?://[^\s]+',text) or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError('Enter an HTTP(S) API base URL without credentials, query or fragment')
    if (kind == 'directory' or key == 'state_dir') and not text:
        raise ValueError('A directory path is required')
    if kind in ('directory','file','path') and text:
        text = str(Path(text).expanduser().absolute())
        if kind == 'directory' and not Path(text).is_dir():
            raise ValueError('Directory does not exist')
        if kind == 'file' and not Path(text).is_file():
            raise ValueError('Credential file does not exist')
    return text


def read(path=None):
    path = path or config_path()
    try:
        fd = os.open(path,os.O_RDONLY|os.O_NOFOLLOW)
    except FileNotFoundError:
        return {}
    with os.fdopen(fd) as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
            raise ValueError('Settings must be a regular file owned by this user')
        data=json.loads(source.read(1048577))
    if not isinstance(data,dict) or data.get('version') != 1 or not isinstance(data.get('values'),dict):
        raise ValueError('Settings must contain version: 1 and a values object')
    return {key:validate(key,value) for key,value in data['values'].items()}


def save(key,value):
    if key == 'GOOGLE_APPLICATION_CREDENTIALS' and not value:
        from funnel_dispatch import default_adc_path
        value=str(default_adc_path())
    value=validate(key,value)
    path=config_path()
    path.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
    lock_fd=os.open(str(path)+'.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    with os.fdopen(lock_fd,'w') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX)
        values=read(path)
        values[key]=value
        name=None
        try:
            with tempfile.NamedTemporaryFile(mode='w',dir=path.parent,prefix='.fullsend-',delete=False) as out:
                name=out.name
                json.dump({'version':1,'values':values},out,indent=2)
                out.write('\n');out.flush();os.fsync(out.fileno())
            os.replace(name,path)
            directory=os.open(path.parent,os.O_RDONLY)
            try:os.fsync(directory)
            finally:os.close(directory)
        finally:
            if name and os.path.exists(name):os.unlink(name)
    return value


def defaults(parser):
    values=read()
    parser.set_defaults(**{key:value for key,value in values.items() if key not in ENV_KEYS and key!='scope'})
    return values


def apply(args, values):
    runtime=json.loads(os.environ.get('FULLSEND_RUNTIME_SETTINGS') or '{}')
    if not isinstance(runtime,dict):raise ValueError('Invalid runtime settings')
    for key,value in runtime.items():
        value=validate(key,value)
        if key in ENV_KEYS:values[key]=value
        else:
            if key in ('state_dir','log_dir','gateway_state'):value=Path(value) if value else None
            setattr(args,key,value)
    for key in ENV_KEYS:
        if key in values:
            if values[key]:os.environ[key]=str(values[key])
            else:os.environ.pop(key,None)
    active=vars(args)
    args.settings_inputs={s[0]:str(os.environ.get(s[0],'')) if s[0] in ENV_KEYS else
                          (str(active[s[0]]) if active.get(s[0]) is not None else '') for s in SPECS}


def display(value):
    if value is None:return 'Not set'
    if isinstance(value,bool):return 'Enabled' if value else 'Disabled'
    if isinstance(value,(list,tuple)):return ', '.join(str(v) for v in value) or 'None'
    return str(value)


def view(args):
    saved=read()
    rows=[dict(key=k,label=label,value=str(args.settings_inputs[k]),effect=effect,kind=kind,choices=choices,
               help=help,saved=str(saved[k]) if k in saved else None) for k,label,effect,kind,choices,help in SPECS]
    for key in ('watch','plain','dry_run','status','retry','retry_uncertain','state_dir','log_dir','env_file'):
        rows.append(dict(key='effective_'+key,label='Effective '+key.replace('_',' '),value=display(vars(args).get(key,'')),
                         effect='Read only',kind='readonly',choices=[],help='Set through launch options.'))
    defined={s[0] for s in SPECS}|{'settings_inputs','jira_selection','watch','plain','dry_run','status','retry','retry_uncertain','env_file'}
    for key,value in vars(args).items():
        if key not in defined:
            rows.append(dict(key='effective_'+key,label=key.replace('_',' ').capitalize(),value=('Hidden' if any(word in key.lower() for word in ('token','secret','password','authorization','credential')) or isinstance(value,dict) else display(value)),
                             effect='Read only',kind='readonly',choices=[],help='Resolved launch configuration.'))
    for key in ('mode','site_url','cloud_id','account_id'):
        value=vars(args).get('jira_selection',{}).get(key)
        if value is not None:
            rows.append(dict(key='jira_active_'+key,label='Active Jira '+key.replace('_',' '),value=str(value),effect='Read only',kind='readonly',choices=[],help='Selected by Jira authentication; restart after changing accounts.'))
    visible_environment = {
        'JIRA_BASE_URL':os.environ.get('JIRA_BASE_URL','Not exported'),
        'JIRA_USER_EMAIL':os.environ.get('JIRA_USER_EMAIL','Not exported'),
        'CLOUDSDK_CONFIG':os.environ.get('CLOUDSDK_CONFIG','Not exported'),
        'ACLI_CONFIG_DIR':os.environ.get('ACLI_CONFIG_DIR','Not exported'),
        'GH_TOKEN':'Exported (hidden)' if 'GH_TOKEN' in os.environ else 'Not exported',
        'JIRA_TOKEN':'Exported (hidden)' if 'JIRA_TOKEN' in os.environ else 'Not exported',
    }
    for key,value in visible_environment.items():
        rows.append(dict(key=key,label=key,value=value,effect='Read only',kind='readonly',choices=[],help='Manage through your env file or authentication tools.'))
    return rows


if __name__=='__main__':
    # The Make launcher can use a saved scope without sourcing shell text.
    try:print(read().get('scope',''))
    except (ValueError,OSError) as exc:raise SystemExit('Cannot read settings '+str(config_path())+': '+str(exc))
