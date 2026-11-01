export default async ({ page }) => {
  const context = page.context();
  await context.routeWebSocket('**/*', socket => socket.close());
  if (process.env.ODOOCTL_BROWSER_PUBLIC === 'true')
    return;
  let stage = 'login_fetch';
  let errorStatus = 'none';
  const deadline = Date.now() + 30000;
  const timeout = (limit = 10000) => {
    const remaining = deadline - Date.now();
    if (remaining <= 0)
      throw new Error('deadline exceeded');
    return Math.min(limit, remaining);
  };
  const recordStatus = status => {
    errorStatus = Number.isInteger(status) && status >= 400 && status <= 599 ? String(status) : 'none';
  };
  try {
    const base = 'http://odoo:8069';
    const loginURL = base + '/web/login';
    const sameOrigin = (location, from) => {
      const target = new URL(location, from);
      if (target.origin !== base || target.username || target.password)
        throw new Error('invalid redirect');
      return target.href;
    };
    let url = loginURL;
    let csrf;
    for (let hops = 0; hops <= 5; hops++) {
      const response = await context.request.get(url, { maxRedirects: 0, timeout: timeout() });
      try {
        const status = response.status();
        recordStatus(status);
        const headers = response.headers();
        if ([301, 302, 303, 307, 308].includes(status)) {
          if (!headers.location || hops === 5)
            throw new Error('invalid redirect');
          url = sameOrigin(headers.location, url);
          continue;
        }
        if (status !== 200 || Number(headers['content-length']) > 1048576)
          throw new Error('invalid login page');
        const html = await response.body();
        if (html.length > 1048576)
          throw new Error('invalid login page');
        stage = 'csrf';
        // Parse only input attributes locally; never execute or expose login HTML.
        const inputs = html.toString('utf8').match(/<input\b(?:[^"'<>]|"[^"]*"|'[^']*')*>/gi) || [];
        for (const input of inputs) {
          const fields = {};
          const attrs = /\s([^\s"'<>\/=]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/g;
          let attr;
          while ((attr = attrs.exec(input))) {
            const name = attr[1].toLowerCase();
            if (!['name', 'value', 'type'].includes(name))
              continue;
            if (Object.hasOwn(fields, name))
              throw new Error('ambiguous login page');
            fields[name] = attr[2] ?? attr[3] ?? attr[4];
          }
          if (fields.name !== 'csrf_token' || fields.type?.toLowerCase() !== 'hidden')
            continue;
          if (csrf !== undefined || !/^[A-Za-z0-9_-]{1,512}$/.test(fields.value || ''))
            throw new Error('invalid csrf token');
          csrf = fields.value;
        }
        break;
      } finally {
        await response.dispose();
      }
    }
    if (!csrf)
      throw new Error('missing csrf token');
    // The shared context cookie jar authenticates subsequent browser GETs.
    // In particular, never let a 307/308 replay this form to another origin.
    stage = 'login_submit';
    errorStatus = 'none';
    const loginResponse = await context.request.post(loginURL, {
      form: {
        login: process.env.ODOOCTL_BROWSER_LOGIN || '',
        password: process.env.ODOOCTL_BROWSER_PASSWORD || '',
        csrf_token: csrf,
      },
      maxRedirects: 0,
      timeout: timeout(),
    });
    try {
      const status = loginResponse.status();
      recordStatus(status);
      const location = loginResponse.headers().location;
      if (location)
        sameOrigin(location, loginURL);
      if (status !== 200 && ![301, 302, 303, 307, 308].includes(status))
        throw new Error('invalid login response');
    } finally {
      await loginResponse.dispose();
    }
    stage = 'session_verify';
    errorStatus = 'none';
    const response = await context.request.post(base + '/web/session/get_session_info', {
      data: { jsonrpc: '2.0', method: 'call', params: {}, id: 1 },
      maxRedirects: 0,
      timeout: timeout(),
    });
    try {
      const status = response.status();
      recordStatus(status);
      if (status !== 200 || response.headers().location)
        throw new Error('invalid session response');
      const body = await response.json();
      if (!Number.isInteger(body?.result?.uid) || body.result.uid <= 0)
        throw new Error('unauthenticated');
    } finally {
      await response.dispose();
    }
    stage = 'page_navigation';
    errorStatus = 'none';
    const navigation = await page.goto(base + '/web', { timeout: timeout(30000) });
    if (navigation) {
      recordStatus(navigation.status());
      if (errorStatus !== 'none')
        throw new Error('invalid navigation response');
    }
  } catch {
    // Only local stage constants and bounded HTTP status values may escape.
    throw new Error(`Odoo browser login failed (stage=${stage}, status=${errorStatus})`);
  }
};
