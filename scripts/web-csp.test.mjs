import { createHash } from 'node:crypto';
import assert from 'node:assert/strict';
import test from 'node:test';
import { pageScriptHashes } from './web-csp.mjs';

test('CSP hashes preserve script bytes, deduplicate and normalize HTML newlines', () => {
  const script = 'window.value = "中文 &amp;";\nwindow.ready = true;';
  const expected = `sha256-${createHash('sha256').update(script).digest('base64')}`;
  const html = `<script src="/chunk.js"></script><script data-value="a>b">${script}</script><SCRIPT>${script.replaceAll('\n', '\r\n')}</SCRIPT>`;
  assert.deepEqual(pageScriptHashes(html), [expected]);
  assert.notDeepEqual(pageScriptHashes(`<script>${script} </script>`), [expected]);
});
