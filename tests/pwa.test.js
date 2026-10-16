import test from 'node:test';
import assert from 'node:assert/strict';
import { createPwaService } from '../frontend/src/services/pwa.js';

function setup({ oldKey = [1, 2, 3], existing = true, permission = 'granted', registerError = false } = {}) {
  const calls = [], values = new Map([['pushEnabled', existing]]);
  let sub, failDelete = false, htmlResponse = false;
  const makeSub = key => ({
    endpoint: `https://push.example/${key.join('-')}`,
    options: { applicationServerKey: Uint8Array.from(key).buffer },
    async unsubscribe() { calls.push('unsubscribe'); sub = null; return true; },
    toJSON() { return { endpoint: this.endpoint, keys: { auth: 'a', p256dh: 'b' } }; },
  });
  sub = existing ? makeSub(oldKey) : null;
  const reg = { pushManager: {
    async getSubscription() { return sub; },
    async subscribe(options) { calls.push('subscribe'); sub = makeSub([...options.applicationServerKey]); return sub; },
  } };
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { serviceWorker: {
    async register() { if (registerError) throw new Error('registration failed'); return reg; },
    ready: Promise.resolve(reg),
  } } });
  globalThis.window = { PushManager: class {}, Notification: {}, addEventListener() {}, removeEventListener() {} };
  globalThis.document = { visibilityState: 'visible', addEventListener() {}, removeEventListener() {} };
  globalThis.Notification = { permission, async requestPermission() { return permission; } };
  globalThis.fetch = async (url, options) => {
    calls.push(options.method);
    return { ok: !(failDelete && options.method === 'DELETE'), headers: new Headers({ 'content-type': htmlResponse ? 'text/html' : 'application/json' }), async json() { return { ok: true }; } };
  };
  const service = createPwaService({
    store: { set: (key, value) => values.set(key, value), get: key => values.get(key) },
    apiJson: async () => ({ publicKey: 'AQID' }),
  });
  return { service, calls, values, setFailDelete: () => { failDelete = true; }, setHTML: () => { htmlResponse = true; } };
}

test('existing matching subscription is synchronized without creating another', async () => {
  const x = setup();
  assert.equal(await x.service.refreshPushState(), true);
  assert.deepEqual(x.calls, ['POST']);
});
test('changed VAPID key removes and replaces the old subscription', async () => {
  const x = setup({ oldKey: [4, 5, 6] });
  assert.equal(await x.service.refreshPushState(), true);
  assert.deepEqual(x.calls, ['DELETE', 'unsubscribe', 'subscribe', 'POST']);
});
test('refresh never enables an opted-out device', async () => {
  const x = setup({ existing: false });
  assert.equal(await x.service.refreshPushState(), false);
  assert.deepEqual(x.calls, []);
});
test('failed DELETE does not unsubscribe locally or report disabled', async () => {
  const x = setup(); x.setFailDelete();
  await assert.rejects(x.service.disablePush(), /synchronize/);
  assert.equal(x.values.get('pushEnabled'), true);
  assert.deepEqual(x.calls, ['DELETE']);
});
test('disable removes server and browser subscription', async () => {
  const x = setup();
  await x.service.disablePush();
  assert.deepEqual(x.calls, ['DELETE', 'unsubscribe']);
  assert.equal(x.values.get('pushEnabled'), false);
});
test('rapid double toggle cannot create two subscriptions', async () => {
  const x = setup({ existing: false });
  await Promise.all([x.service.togglePush(), x.service.togglePush()]);
  assert.deepEqual(x.calls, ['subscribe', 'POST']);
});
test('HTML login response is not accepted as successful registration', async () => {
  const x = setup({ existing: false }); x.setHTML();
  await assert.rejects(x.service.enablePush(), /synchronize/);
  assert.equal(x.values.get('pushEnabled'), false);
});
test('registration failure rejects enable rather than waiting forever', async () => {
  const x = setup({ existing: false, registerError: true });
  await assert.rejects(x.service.enablePush(), /registration failed/);
});
test('denied notification permission does not synchronize', async () => {
  const x = setup({ permission: 'denied' });
  assert.equal(await x.service.refreshPushState(), false);
  assert.deepEqual(x.calls, []);
});
