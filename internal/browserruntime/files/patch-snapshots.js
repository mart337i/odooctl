'use strict';

const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');

// Verified published bundle for playwright-core 1.64.0-alpha-1790635538000,
// gitHead e8149b8257d32dcf8f72573ecc43e72439da7080. Patch the common injected
// ARIA producer, not application DOM or individual MCP tool responses.
const version = '1.64.0-alpha-1790635538000';
const digest = 'cd6730be1bbcff00771a0fc1390f200306d28428ce68e305420ca623cb39cdeb';
const moduleMarker = '// packages/playwright-core/src/generated/injectedScriptSource.ts';
const marker = '  if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {\\n    if (element.type !== "checkbox" && element.type !== "radio" && element.type !== "file")\\n      result.children = [element.value];\\n  }';

function patchSnapshots(source) {
  const start = source.indexOf(moduleMarker);
  const end = source.indexOf('\n// packages/', start + moduleMarker.length);
  const at = source.indexOf(marker);
  if (start < 0 || source.split(moduleMarker).length !== 2 || end < 0 || at < start || at >= end || source.split(marker).length !== 2 || source.split('result.children = [element.value];').length !== 2)
    throw new Error('Pinned snapshot producer mismatch');
  return source.replace(marker, marker.replace('result.children = [element.value];', 'void 0; /* odooctl: omit control values */'));
}

module.exports = { patchSnapshots };

if (require.main === module) {
  try {
    const root = path.join(__dirname, 'node_modules', 'playwright-core');
    if (JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8')).version !== version)
      throw new Error('version mismatch');
    const file = path.join(root, 'lib', 'coreBundle.js');
    const source = fs.readFileSync(file, 'utf8');
    if (crypto.createHash('sha256').update(source).digest('hex') !== digest)
      throw new Error('bundle mismatch');
    fs.writeFileSync(file, patchSnapshots(source));
  } catch {
    process.stderr.write('Pinned Playwright snapshot patch validation failed\n');
    process.exitCode = 1;
  }
}
