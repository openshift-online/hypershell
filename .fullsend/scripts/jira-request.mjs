// Shared sandbox Jira transport. Credentials never appear in arguments or output.
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

export function requestOptions(credentials, method, path) {
  if (!['GET', 'POST', 'PUT', 'DELETE'].includes(method)) throw new Error('Unsupported Jira method');
  if (credentials.version !== 1) throw new Error('Unsupported Jira credential snapshot');
  const base = credentials.api_base_url;
  const basic = credentials.mode === 'api-token' && /^https:\/\/[a-zA-Z0-9-]+\.atlassian\.net$/.test(base);
  const oauth = credentials.mode === 'acli' && /^https:\/\/api\.atlassian\.com\/ex\/jira\/[a-zA-Z0-9-]+$/.test(base);
  if (!basic && !oauth) throw new Error('Invalid Jira API base URL');
  if (typeof credentials.authorization !== 'string' || /[\r\n]/.test(credentials.authorization) ||
      !credentials.authorization.startsWith(basic ? 'Basic ' : 'Bearer ')) throw new Error('Invalid Jira authorization scheme');
  if (!/^\/rest\/api\/3\//.test(path) || /[\\#\r\n]/.test(path)) throw new Error('Use a relative /rest/api/3/ path');
  const target = new URL(base + path);
  // URL normalization must not escape the selected tenant or API prefix.
  const prefix = new URL(base).pathname.replace(/\/$/, '') + '/rest/api/3/';
  if (!target.pathname.startsWith(prefix) || /%2e|%2f|%5c/i.test(target.pathname)) throw new Error('Invalid Jira API path');
  if (oauth && (!Number.isFinite(Date.parse(credentials.expires_at)) || Date.parse(credentials.expires_at) <= Date.now())) {
    throw new Error('Jira OAuth snapshot expired; stop this attempt so the host can refresh through acli on retry');
  }
  return { url: target.toString(), options: {
    method, redirect: 'error', signal: AbortSignal.timeout(60000),
    headers: { Authorization: credentials.authorization, Accept: 'application/json', 'Content-Type': 'application/json' },
  }};
}

export async function loadCredentials(env = process.env) {
  if (env.JIRA_CREDENTIALS_FILE) {
    return JSON.parse(await readFile(env.JIRA_CREDENTIALS_FILE, 'utf8'));
  }
  if (!env.JIRA_EMAIL || !env.JIRA_API_TOKEN || !env.JIRA_BASE_URL) {
    throw new Error('Invalid Jira credential configuration');
  }
  return {version: 1, mode: 'api-token', api_base_url: env.JIRA_BASE_URL,
    authorization: 'Basic ' + Buffer.from(env.JIRA_EMAIL + ':' + env.JIRA_API_TOKEN).toString('base64')};
}

async function main() {
  const [method, path, bodyFile] = process.argv.slice(2);
  const credentials = await loadCredentials();
  const {url, options} = requestOptions(credentials, method, path);
  if (bodyFile) {
    if (method === 'GET') throw new Error('GET requests cannot have a body');
    options.body = await readFile(bodyFile === '-' ? '/dev/stdin' : bodyFile, 'utf8');
    JSON.parse(options.body);
  }
  const response = await fetch(url, options);
  if (!response.ok) {
    const hint = response.status === 401 ? ' Authentication expired or was revoked; retry with refreshed host credentials.' :
      response.status === 403 ? ' Check the account permissions and OAuth scopes; API-token mode is also supported.' : '';
    throw new Error(`Jira returned HTTP ${response.status}.${hint}`);
  }
  process.stdout.write(await response.text());
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => {
    // Do not log response bodies, headers, credentials, or raw network errors.
    const known = /^(Unsupported Jira|Invalid Jira|Use a relative|GET requests|Jira (OAuth|returned))/;
    console.error(known.test(error.message) ? error.message : 'Jira request failed; check the request JSON and network access');
    process.exitCode = 1;
  });
}
