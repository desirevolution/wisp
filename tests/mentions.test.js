import test from 'node:test';
import assert from 'node:assert/strict';
import { linkify } from '../frontend/src/lib/utils.js';

test('resolved mention names are escaped as text, while URLs still become links', () => {
  const output = linkify('Hi @<img src=x onerror=alert(1)> & @Arthur https://example.org');
  assert.ok(output.includes('@&lt;img src=x onerror=alert(1)&gt;'));
  assert.ok(output.includes('&amp; @Arthur'));
  assert.ok(!output.includes('<img'));
  assert.ok(output.includes('href="https://example.org"'));
});
