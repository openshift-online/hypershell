import test from 'node:test';
import assert from 'node:assert/strict';
import { requestOptions } from '../.fullsend/scripts/jira-request.mjs';

const api = {version:1, mode:'api-token', api_base_url:'https://redhat.atlassian.net', authorization:'Basic test'};
const oauth = {version:1, mode:'acli', api_base_url:'https://api.atlassian.com/ex/jira/cloud-123', authorization:'Bearer test',expires_at:'2099-01-01T00:00:00Z'};

test('Basic and OAuth use their own API base and authentication header', () => {
  for (const credentials of [api, oauth]) {
    const {url,options}=requestOptions(credentials,'GET','/rest/api/3/myself');
    assert.equal(url,credentials.api_base_url+'/rest/api/3/myself');
    assert.equal(options.headers.Authorization,credentials.authorization);
    assert.equal(options.redirect,'error');
  }
});

test('requests cannot redirect credentials to other hosts or escape the tenant', () => {
  for (const path of ['https://evil.test/x','//evil.test/x','/rest/api/3/../../../../other','/rest/api/3/%2e%2e/%2fescape','/rest/api/3/issue#secret','/rest/api/3/issue\\evil']) {
    assert.throws(()=>requestOptions(oauth,'GET',path));
  }
  assert.throws(()=>requestOptions({...oauth,api_base_url:'https://evil.test'},'GET','/rest/api/3/myself'));
});

test('expired OAuth fails before issuing a request', () => {
  assert.throws(()=>requestOptions({...oauth,expires_at:'2000-01-01T00:00:00Z'},'POST','/rest/api/3/issue'),/expired/);
  assert.throws(()=>requestOptions({...oauth,expires_at:'not-a-date'},'GET','/rest/api/3/myself'),/expired/);
});

test('JQL queries and JSON write requests stay supported', () => {
  const query='/rest/api/3/search/jql?jql='+encodeURIComponent('labels = "agent/scope/test"');
  assert.equal(requestOptions(api,'GET',query).url,api.api_base_url+query);
  assert.equal(requestOptions(oauth,'POST','/rest/api/3/issue/TEST-1/remotelink').options.method,'POST');
});
