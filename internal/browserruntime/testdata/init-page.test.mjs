import assert from 'node:assert/strict';
import { createHmac } from 'node:crypto';

const base = 'http://odoo:8069';
// Odoo 5a4e4c21 http.Request.csrf_token: SHA-1 hex + 'o' + integer expiry.
const expiry = 1822489200;
const token = createHmac('sha1', 'fixture-secret').update('A'.repeat(42) + expiry).digest('hex') + 'o' + expiry;
const login = base + '/web/login';
const session = base + '/web/session/get_session_info';
process.env.ODOOCTL_BROWSER_PUBLIC = 'false';
process.env.ODOOCTL_BROWSER_URL = 'https://agent-controlled.example';
process.env.ODOOCTL_BROWSER_LOGIN = 'test-login';
process.env.ODOOCTL_BROWSER_PASSWORD = 'test-password';

async function run(responses, failure = false) {
  const requests = [];
  const disposed = [];
  const navigations = [];
  let closed = false;
  let cookie = '';
  const request = async (method, url, options) => {
    assert.equal(new URL(url).origin, base, 'external authentication request');
    assert.equal(options.maxRedirects, 0, 'automatic redirects must be disabled');
    assert.ok(Number.isInteger(options.timeout) && options.timeout > 0 && options.timeout <= 10000);
    const expected = responses[requests.length];
    assert.ok(expected, 'unexpected request');
    assert.equal(method, expected.method);
    assert.equal(url, expected.url);
    if (method === 'post' && url === login) {
      assert.equal(cookie, 'OID_SESSION');
      assert.deepEqual(options.form, { login: 'test-login', password: 'test-password', csrf_token: token });
    }
    if (url === session) {
      assert.equal(cookie, 'OID_SESSION_AUTHENTICATED');
      assert.deepEqual(options.data, { jsonrpc: '2.0', method: 'call', params: {}, id: 1 });
      assert.equal(options.form, undefined);
    }
    requests.push({ method, url });
    if (expected.error)
      throw new Error(expected.error);
    if (expected.cookie)
      cookie = expected.cookie;
    expected.after?.();
    return {
      status: () => expected.status,
      headers: () => expected.headers || {},
      body: async () => Buffer.from(expected.html || ''),
      json: async () => {
        if (expected.jsonError)
          throw new Error(expected.jsonError);
        return expected.json || { result: { uid: 2, private: 'withheld-session-data' } };
      },
      dispose: async () => disposed.push(url),
    };
  };
  const context = {
    request: { get: (url, options) => request('get', url, options), post: (url, options) => request('post', url, options) },
    routeWebSocket: async (pattern, handler) => {
      assert.equal(pattern, '**/*');
      handler({ close: () => { closed = true; } });
    },
  };
  const page = {
    context: () => context,
    locator: () => { throw new Error('credentials must never enter browser DOM'); },
    goto: async (url, options) => {
      assert.ok(options.timeout > 0 && options.timeout <= 30000);
      assert.equal(cookie, 'OID_SESSION_AUTHENTICATED');
      assert.equal(requests.at(-1).url, session, 'navigation must follow session verification');
      if (responses.at(-1).navigationError)
        throw new Error(responses.at(-1).navigationError);
      navigations.push(url);
      if (responses.at(-1).navigationStatus !== undefined)
        return { status: () => responses.at(-1).navigationStatus };
    },
  };
  if (failure) {
    await assert.rejects(() => init({ page }), error => {
      assert.match(error.message, /^Odoo browser login failed \(stage=(login_fetch|csrf|login_submit|session_verify|page_navigation), status=(none|[45][0-9]{2})\)$/);
      if (typeof failure === 'string')
        assert.equal(error.message, `Odoo browser login failed (${failure})`);
      return true;
    });
    assert.deepEqual(navigations, responses.at(-1)?.navigationStatus !== undefined ? [base + '/web'] : []);
  } else {
    assert.equal(await init({ page }), undefined, 'no response data returned');
    assert.deepEqual(navigations, process.env.ODOOCTL_BROWSER_PUBLIC === 'true' ? [] : [base + '/web']);
  }
  assert.equal(closed, true);
  assert.equal(requests.length, responses.length);
  assert.equal(disposed.length, responses.filter(response => !response.error).length, 'authentication responses must be discarded');
}

const get = (html, url = login) => ({ method: 'get', url, status: 200, html, cookie: 'OID_SESSION' });
const post = (status = 303, location = '/web') => ({ method: 'post', url: login, status, headers: location ? { location } : {}, cookie: 'OID_SESSION_AUTHENTICATED' });
const verify = (extra = {}) => ({ method: 'post', url: session, status: 200, ...extra });
const html = `<input type="hidden" name="csrf_token" value="${token}">`;

for (const variant of [
  html,
  `<INPUT VALUE='${token}' NAME='csrf_token' TYPE='HIDDEN'>`,
  `<input data-example="ignored > name='not-csrf'" value = '${token}' type=hidden name=csrf_token />`,
  `<input name="csrf_token" type='hidden' value=${token}>`,
])
  await run([get(variant), post(), verify()]);

await run([
  { method: 'get', url: login, status: 302, headers: { location: '/web/login?db=fixture' } },
  get(html, login + '?db=fixture'), post(200, ''), verify(),
]);
// Five local GET redirects are permitted, but never an unbounded chain.
await run([...Array.from({ length: 5 }, () => ({ method: 'get', url: login, status: 302, headers: { location: login } })), get(html), post(), verify()]);
await run(Array.from({ length: 6 }, () => ({ method: 'get', url: login, status: 302, headers: { location: login } })), true);

for (const status of [301, 302, 303, 307, 308]) {
  await run([get(html), post(status, 'https://approved-external.example/collect')], true);
  await run([get(html), post(status, '/web') , verify()]);
  await run([{ method: 'get', url: login, status, headers: { location: 'https://approved-external.example/login' } }], true);
}
for (const location of ['//approved-external.example/collect', 'http://odoo:8069@approved-external.example/', 'http://odoo:8068/web', 'https://odoo:8069/web', 'http://user:password@odoo:8069/web'])
  await run([get(html), post(307, location)], true);

for (const invalid of [
  '',
  `<input type="hidden" data-name="csrf_token" value="${token}">`,
  `<input type="text" name="csrf_token" value="${token}">`,
  `<input type="hidden" name="csrf_token" value="">`,
  `<input type="hidden" name="csrf_token" value="unsafe&amp;token">`,
  `<input type="hidden" name="csrf_token" value="${'a'.repeat(513)}">`,
  `<input type="hidden" name="csrf_token" value="${token}" value="duplicate">`,
  html + html,
  'x'.repeat(1048577) + html,
])
  await run([get(invalid)], true);
await run([{ ...get(html), headers: { 'content-length': '1048577' } }], true);
await run([{ ...get(html), status: 403 }], 'stage=login_fetch, status=403');
await run([get('')], 'stage=csrf, status=none');
await run([get(html), post(401, '')], 'stage=login_submit, status=401');
await run([get(html), post(), verify({ status: 503 })], 'stage=session_verify, status=503');
await run([get(html), post(), verify({ json: { result: { uid: 0 } } })], 'stage=session_verify, status=none');
for (const status of [399, 600, '403', 403.5, NaN])
  await run([{ ...get(html), status }], 'stage=login_fetch, status=none');
await run([{ ...get(html), error: `secret ${token} test-password private-body` }], 'stage=login_fetch, status=none');
await run([get(html), post(), verify({ jsonError: 'private-session-body test-password' })], 'stage=session_verify, status=none');
await run([get(html), post(), verify({ navigationError: 'private-url test-password' })], 'stage=page_navigation, status=none');
await run([get(html), post(), verify({ navigationStatus: 502 })], 'stage=page_navigation, status=502');
await run([get(html), post(), verify({ json: { jsonrpc: '2.0', error: { code: 100, data: 'private-traceback' } } })], 'stage=session_verify, status=none');
// Exhaust the shared budget without sleeping; no credential POST may start.
const now = Date.now;
let clock = 0;
Date.now = () => clock;
try {
  await run([{ ...get(html), after: () => { clock = 30000; } }], 'stage=login_submit, status=none');
} finally {
  Date.now = now;
}
for (const extra of [{ status: 307, headers: { location: 'https://approved-external.example/' } }, { json: { result: { uid: 0 } } }, { json: { result: { uid: '7' } } }])
  await run([get(html), post(), verify(extra)], true);

process.env.ODOOCTL_BROWSER_PUBLIC = 'true';
await run([]);
