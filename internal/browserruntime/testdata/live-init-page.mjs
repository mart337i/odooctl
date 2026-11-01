import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';

const require = createRequire('/opt/runtime/package.json');
const { chromium } = require('playwright');
const config = JSON.parse(readFileSync('/opt/runtime/config.json', 'utf8'));
const browser = await chromium.launch({
  ...config.browser.launchOptions,
  executablePath: '/opt/chromium', headless: true,
  args: [...config.browser.launchOptions.args, '--no-sandbox'],
  proxy: { server: 'http://proxy:3128', bypass: '<-loopback>' },
});
try {
  const context = await browser.newContext({ ...config.browser.contextOptions, serviceWorkers: 'block' });
  const page = await context.newPage();
  for (const method of ['get', 'post']) {
    const original = context.request[method].bind(context.request);
    context.request[method] = async (url, options) => {
      const stage = new URL(url).pathname === '/web/login' ? 'login' : 'session';
      try {
        const response = await original(url, options);
        const cookies = await context.cookies();
        console.log(JSON.stringify({ method, stage, status: response.status(), hasSessionCookie: cookies.some(cookie => cookie.name === 'session_id') }));
        const body = response.body.bind(response);
        response.body = async () => {
          const data = await body();
          console.log(JSON.stringify({ stage, htmlBytes: data.length, csrfNamePresent: data.includes('csrf_token') }));
          return data;
        };
        const json = response.json.bind(response);
        response.json = async () => {
          const data = await json();
          console.log(JSON.stringify({ stage, uidPositive: Number.isInteger(data?.result?.uid) && data.result.uid > 0 }));
          return data;
        };
        return response;
      } catch (error) {
        const code = ['ENOTFOUND', 'ECONNREFUSED', 'ETIMEDOUT', 'ENETUNREACH'].find(code => String(error?.message).includes(code)) || 'withheld';
        console.log(JSON.stringify({ method, stage, errorCode: code }));
        throw error;
      }
    };
  }
  await init({ page });
  console.log(JSON.stringify({ authenticated: true }));
} catch {
  console.log(JSON.stringify({ authenticated: false }));
  process.exitCode = 1;
} finally {
  await browser.close();
}
