import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
const source = readFileSync(new URL('../frontend/public/sw.js', import.meta.url), 'utf8');
function setup(windows = []) {
  const handlers = {}, notifications = [], opened = [], synced = [];
  const self = {
    location: { origin: 'https://wisp.example' },
    addEventListener: (type, handler) => { handlers[type] = handler; },
    registration: { showNotification: async (title, options) => { notifications.push({ title, ...options }); } },
    clients: { matchAll: async () => windows, openWindow: async url => { opened.push(url); } },
  };
  vm.runInNewContext(source, { self, URL, URLSearchParams, AbortSignal, fetch: async (url, options) => {
    synced.push([options.method, JSON.parse(options.body)]);
    return { ok: true, headers: new Headers({ 'content-type': 'application/json' }), json: async () => ({ ok: true }) };
  } });
  async function emit(type, values) {
    let done;
    handlers[type]({ ...values, waitUntil(promise) { done = promise; } });
    await done;
  }
  return { emit, notifications, opened, synced };
}
test('retries share a tag, separate messages and chats do not', async () => {
  const x = setup();
  for (const [chatId, messageId] of [['a', '1'], ['a', '1'], ['a', '2'], ['b', '1']]) {
    await x.emit('push', { data: { json: () => ({ title: 'Chat', body: 'Text', chatId, messageId }) } });
  }
  assert.equal(x.notifications[0].tag, x.notifications[1].tag);
  assert.notEqual(x.notifications[0].tag, x.notifications[2].tag);
  assert.notEqual(x.notifications[0].tag, x.notifications[3].tag);
  assert.equal(x.notifications[0].renotify, false);
});
test('click opens encoded chat/message deep link in a new window', async () => {
  const x = setup();
  await x.emit('notificationclick', { notification: { close() {}, data: { chatId: '123@g.us', messageId: 'A+B&?' } } });
  const url = new URL(x.opened[0]);
  assert.equal(url.searchParams.get('chat'), '123@g.us');
  assert.equal(url.searchParams.get('message'), 'A+B&?');
});
test('null navigation falls back to a new window', async () => {
  const x = setup([{ url: 'https://wisp.example/', navigate: async () => null }]);
  await x.emit('notificationclick', { notification: { close() {}, data: { chatId: 'a' } } });
  assert.equal(x.opened.length, 1);
});
test('focused app window is reused instead of unrelated path', async () => {
  let focused = false;
  const x = setup([
    { url: 'https://wisp.example/other', navigate() { throw new Error('wrong app'); } },
    { url: 'https://wisp.example/', focused: true, navigate: async () => ({ focus: async () => { focused = true; } }) },
  ]);
  await x.emit('notificationclick', { notification: { close() {}, data: {} } });
  assert.equal(focused, true);
  assert.deepEqual(x.opened, []);
});
test('subscription change registers replacement before deleting old endpoint', async () => {
  const x = setup();
  await x.emit('pushsubscriptionchange', { oldSubscription: { endpoint: 'old' }, newSubscription: { endpoint: 'new' } });
  assert.deepEqual(x.synced, [['POST', { endpoint: 'new' }], ['DELETE', { endpoint: 'old' }]]);
});
test('test notifications have unique tags and open the app without a fake chat', async () => {
  const x = setup();
  for (const testId of ['test-one', 'test-two']) {
    await x.emit('push', { data: { json: () => ({ title: 'Test', body: 'Test', testId }) } });
  }
  assert.notEqual(x.notifications[0].tag, x.notifications[1].tag);
  assert.equal(x.notifications[0].data.testId, 'test-one');
  await x.emit('notificationclick', { notification: { close() {}, data: x.notifications[0].data } });
  assert.equal(new URL(x.opened[0]).search, '');
});
