ARG BASE=node:22-bookworm-slim
# @playwright/mcp 0.0.83 upstream gitHead f183dad4a52965583e3cc1d59b88cdc279e2e57d.
FROM ${BASE}
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/browsers HOME=/tmp
WORKDIR /opt/runtime
COPY patch-snapshots.js /opt/runtime/
RUN npm install --omit=dev --save-exact @playwright/mcp@0.0.83 playwright@1.64.0-alpha-1790635538000 playwright-core@1.64.0-alpha-1790635538000 \
    && node /opt/runtime/patch-snapshots.js \
    && ./node_modules/.bin/playwright install --with-deps chromium \
    && node -e "require('fs').symlinkSync(require('playwright').chromium.executablePath(), '/opt/chromium')" \
    && chmod -R a+rX /opt/runtime /opt/browsers
COPY init-page.ts init-script.js config.json /opt/runtime/
RUN chmod 0444 /opt/runtime/init-page.ts /opt/runtime/init-script.js /opt/runtime/config.json /opt/runtime/patch-snapshots.js
WORKDIR /artifacts
ENTRYPOINT ["node", "/opt/runtime/node_modules/@playwright/mcp/cli.js"]
