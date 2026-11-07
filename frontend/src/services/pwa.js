/** Browser/PWA lifecycle and per-device Web Push subscriptions. */
export function createPwaService({ store, apiJson, onStateChange = () => {}, onUpdateChange = () => {}, onError = () => {} }) {
  let registrationPromise;
  let workerRegistration;
  let installing;
  let dismissed;
  let applying = false;
  let applyTimer;
  let checkTimer;
  let lastCheck = 0;
  let operation = Promise.resolve();
  let toggling = null;
  const set = (key, value) => { store.set(key, value); onStateChange(); return value; };
  const supported = () => 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
  const serial = task => {
    const next = operation.then(task);
    operation = next.catch(() => {});
    return next;
  };

  function registerWorker() {
    if (!registrationPromise) {
      registrationPromise = navigator.serviceWorker.register('/sw.js', { updateViaCache: 'none' })
        .then(reg => {
          workerRegistration = reg;
          reg.addEventListener?.('updatefound', watchInstalling);
          watchInstalling();
          return reg;
        })
        .catch(error => { registrationPromise = null; throw error; });
    }
    return registrationPromise;
  }

  async function registration() {
    let timer;
    try {
      return await Promise.race([
        registerWorker().then(() => navigator.serviceWorker.ready),
        new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Service worker is not ready. Please reload and try again.')), 10000); }),
      ]);
    } finally { clearTimeout(timer); }
  }

  function announceUpdate() {
    const waiting = workerRegistration?.waiting;
    onUpdateChange(Boolean(waiting && navigator.serviceWorker.controller && waiting !== dismissed), applying);
  }

  function watchInstalling() {
    installing?.removeEventListener('statechange', announceUpdate);
    installing = workerRegistration?.installing;
    installing?.addEventListener('statechange', announceUpdate);
    announceUpdate();
  }

  async function checkForUpdate() {
    if (document.visibilityState !== 'visible' || navigator.onLine === false || !('serviceWorker' in navigator)) return;
    if (Date.now() - lastCheck < 60_000) return;
    lastCheck = Date.now();
    try {
      const reg = await registerWorker();
      await reg.update();
      announceUpdate();
    } catch (error) {
      console.debug('PWA update check unavailable', error);
    }
  }

  function dismissUpdate() {
    dismissed = workerRegistration?.waiting;
    announceUpdate();
  }

  function applyUpdate() {
    const waiting = workerRegistration?.waiting;
    if (!waiting || applying) return;
    applying = true;
    announceUpdate();
    applyTimer = setTimeout(() => {
      applying = false;
      announceUpdate();
      onError('Could not apply the app update. Please try again.');
    }, 10_000);
    waiting.postMessage({ type: 'SKIP_WAITING' });
  }

  function onControllerChange() {
    if (applying) {
      clearTimeout(applyTimer);
      applying = false;
      window.location.reload();
    } else {
      announceUpdate();
    }
  }

  async function updateSubscription(method, data) {
    const response = await fetch('/api/push/subscribe', {
      method, headers: { 'content-type': 'application/json' },
      body: JSON.stringify(data), signal: AbortSignal.timeout(10000),
    });
    // An authentication redirect can return a successful HTML login page.
    if (!response.ok || !(response.headers.get('content-type') || '').includes('application/json')) {
      throw new Error('Could not synchronize push notifications with Wisp. Check your connection and login.');
    }
    const result = await response.json();
    if (result.ok !== true) throw new Error('Wisp did not confirm the push subscription.');
  }

  function applicationServerKey(publicKey) {
    const padded = publicKey.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(publicKey.length / 4) * 4, '=');
    return Uint8Array.from(atob(padded), c => c.charCodeAt(0));
  }
  function sameKey(sub, key) {
    const current = sub.options?.applicationServerKey;
    if (!current) return false;
    const bytes = new Uint8Array(current);
    return bytes.length === key.length && bytes.every((value, index) => value === key[index]);
  }

  async function synchronize(create) {
    const reg = await registration();
    let sub = await reg.pushManager.getSubscription();
    if (!sub && !create) return set('pushEnabled', false);
    const { publicKey } = await apiJson('/api/push/public-key', { signal: AbortSignal.timeout(10000) });
    const key = applicationServerKey(publicKey);
    if (sub && !sameKey(sub, key)) {
      await updateSubscription('DELETE', { endpoint: sub.endpoint });
      if (!await sub.unsubscribe()) throw new Error('Could not replace the old push subscription. Please try again.');
      sub = null;
    }
    // Only create automatically when replacing an existing subscription, or
    // following an explicit Enable. Never enable push on an opted-out device.
    sub ||= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
    await updateSubscription('POST', sub);
    return set('pushEnabled', true);
  }

  function refreshPushState() {
    return serial(async () => {
      if (!supported() || Notification.permission !== 'granted') return set('pushEnabled', false);
      try { return await synchronize(false); }
      catch (error) {
        console.warn('Push subscription synchronization failed', error);
        return set('pushEnabled', false);
      }
    });
  }

  function enablePush() {
    if (!supported()) { onError('Web Push is not supported in this browser.'); return Promise.resolve(); }
    // Ask within the user gesture, before waiting for registration/network work.
    const permission = Notification.requestPermission();
    return serial(async () => {
      if (await permission !== 'granted') { onError('Notification permission was not granted.'); return; }
      await synchronize(true);
    });
  }

  function disablePush() {
    return serial(async () => {
      if (!('serviceWorker' in navigator)) return;
      const reg = await registration();
      const sub = await reg.pushManager.getSubscription();
      if (sub) {
        await updateSubscription('DELETE', { endpoint: sub.endpoint });
        if (!await sub.unsubscribe()) throw new Error('Could not disable push in this browser. Please try again.');
      }
      set('pushEnabled', false);
    });
  }

  function togglePush() {
    if (toggling) return toggling;
    toggling = (store.get('pushEnabled') ? disablePush() : enablePush()).finally(() => { toggling = null; });
    return toggling;
  }

  function onResume() {
    if (document.visibilityState === 'visible') {
      void refreshPushState();
      void checkForUpdate();
    }
  }

  async function installApp() {
    const prompt = store.get('installPrompt');
    if (!prompt) return;
    set('installPrompt', null);
    await prompt.prompt();
    await prompt.userChoice;
  }

  function onBeforeInstallPrompt(event) {
    event.preventDefault();
    set('installPrompt', event);
  }

  function onAppInstalled() {
    store.update({ installPrompt: null, isStandalone: true });
    onStateChange();
  }

  function start() {
    window.addEventListener('beforeinstallprompt', onBeforeInstallPrompt);
    window.addEventListener('appinstalled', onAppInstalled);
    window.addEventListener('online', onResume);
    document.addEventListener('visibilitychange', onResume);
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.addEventListener('controllerchange', onControllerChange);
      window.addEventListener('focus', checkForUpdate);
      checkTimer = setInterval(checkForUpdate, 30 * 60_000);
      void checkForUpdate();
    }
  }

  function stop() {
    navigator.serviceWorker?.removeEventListener('controllerchange', onControllerChange);
    workerRegistration?.removeEventListener('updatefound', watchInstalling);
    installing?.removeEventListener('statechange', announceUpdate);
    window.removeEventListener('focus', checkForUpdate);
    clearInterval(checkTimer);
    clearTimeout(applyTimer);
    window.removeEventListener('beforeinstallprompt', onBeforeInstallPrompt);
    window.removeEventListener('appinstalled', onAppInstalled);
    window.removeEventListener('online', onResume);
    document.removeEventListener('visibilitychange', onResume);
  }

  return { start, stop, refreshPushState, enablePush, disablePush, togglePush, installApp, dismissUpdate, applyUpdate };
}
