'use strict';

const assert = require('node:assert/strict');
const { patchSnapshots } = require(process.argv[2]);

// Exact generated producer fragment verified against the published pinned
// coreBundle.js and upstream ariaSnapshot.ts at e8149b8257d32dcf8f72573ecc43e72439da7080.
const producer = `function toAriaNode(element) {
  const result = { role: "textbox", name: "control", children: [], box: { x: 1, y: 2 } };
  if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
    if (element.type !== "checkbox" && element.type !== "radio" && element.type !== "file")
      result.children = [element.value];
  }
  return result;
}`;
const moduleMarker = '// packages/playwright-core/src/generated/injectedScriptSource.ts';
const escaped = producer.replace(/\n/g, '\\n');
const bundle = `${moduleMarker}\nvar source5 = '${escaped}';\n// packages/nextModule.ts\n`;
const patched = patchSnapshots(bundle);
assert.ok(patched.includes('void 0; /* odooctl: omit control values */'));
assert.ok(!patched.includes('result.children = [element.value]'));
assert.equal(patched.replace('void 0; /* odooctl: omit control values */', 'result.children = [element.value];'), bundle);

class HTMLInputElement {
  constructor(type, value) { this.type = type; this.value = value; }
}
class HTMLTextAreaElement {
  constructor(value) { this.type = 'textarea'; this.value = value; }
}
const original = new Function('HTMLInputElement', 'HTMLTextAreaElement', producer + '; return toAriaNode;')(HTMLInputElement, HTMLTextAreaElement);
const source = patched.slice(patched.indexOf("var source5 = '") + 15, patched.indexOf("';\n// packages/nextModule.ts")).replace(/\\n/g, '\n');
const safe = new Function('HTMLInputElement', 'HTMLTextAreaElement', source + '; return toAriaNode;')(HTMLInputElement, HTMLTextAreaElement);
for (const element of [new HTMLInputElement('password', 'secret'), new HTMLInputElement('text', 'secret'), new HTMLTextAreaElement('secret')]) {
  assert.deepEqual(original(element).children, ['secret']);
  const snapshot = safe(element);
  assert.deepEqual(snapshot.children, []);
  assert.equal(snapshot.name, 'control');
  assert.deepEqual(snapshot.box, { x: 1, y: 2 });
  assert.equal(element.value, 'secret', 'application DOM value must be unchanged');
}
for (const type of ['checkbox', 'radio', 'file'])
  assert.deepEqual(safe(new HTMLInputElement(type, 'unchanged')), original(new HTMLInputElement(type, 'unchanged')));

for (const invalid of [
  bundle.replace('result.children = [element.value];', 'result.children.push(element.value);'),
  bundle + 'result.children = [element.value];',
  bundle + moduleMarker,
  bundle.replace(moduleMarker, '// unexpected injected module'),
  bundle.replace('// packages/nextModule.ts', '// unexpected boundary'),
  bundle.replace(escaped, escaped + escaped),
  `${moduleMarker}\n// packages/nextModule.ts\n${escaped}`,
  patched,
])
  assert.throws(() => patchSnapshots(invalid), { message: 'Pinned snapshot producer mismatch' });

// Optional read-only registry verification; default unit tests stay offline.
if (process.env.ODOOCTL_VERIFY_PINNED_PLAYWRIGHT === 'true') {
  (async () => {
    const response = await fetch('https://unpkg.com/playwright-core@1.64.0-alpha-1790635538000/lib/coreBundle.js', { signal: AbortSignal.timeout(15000) });
    assert.equal(response.status, 200);
    const source = await response.text();
    assert.equal(require('node:crypto').createHash('sha256').update(source).digest('hex'), 'cd6730be1bbcff00771a0fc1390f200306d28428ce68e305420ca623cb39cdeb');
    const patched = patchSnapshots(source);
    assert.ok(!patched.includes('result.children = [element.value];'));
    assert.equal(patched.split('void 0; /* odooctl: omit control values */').length, 2);
  })().catch(error => { console.error(error); process.exitCode = 1; });
}
