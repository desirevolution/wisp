const CACHE = 'wisp-dev';
const SHELL = [
  '/', '/manifest.webmanifest', '/icons/icon.svg', '/icons/icon-192.png',
  '/icons/icon-512.png', '/icons/icon-maskable-512.png',
  /* BUILD_ASSETS */
];

self.addEventListener('install', event => {
  event.waitUntil((async () => {
    const requests = SHELL.map(url => new Request(url, { cache: 'reload', redirect: 'error' }));
    const cache = await caches.open(CACHE);
    await cache.addAll(requests);
  })());
});

self.addEventListener('message', event => {
  if (event.data?.type === 'SKIP_WAITING') event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    const keys = await caches.keys();
    await Promise.all(keys
      .filter(key => key !== CACHE && (key.startsWith('wisp-build-') || key === 'wisp-dev' || /^wisp-\d+\.\d+\.\d+$/.test(key)))
      .map(key => caches.delete(key)));
    await self.clients.claim();
  })());
});

self.addEventListener('fetch', event => {
  const request = event.request;
  const url = new URL(request.url);
  if (request.method !== 'GET' || url.origin !== self.location.origin) return;
  if (request.mode === 'navigate' && !url.pathname.startsWith('/api/')) {
    // Preserve server authentication and deep links during ordinary navigation.
    event.respondWith(fetch(request).catch(async () => {
      const cache = await caches.open(CACHE);
      return await cache.match('/') || Response.error();
    }));
  } else if (SHELL.includes(url.pathname) && url.pathname !== '/') {
    event.respondWith((async () => {
      const cache = await caches.open(CACHE);
      return await cache.match(request) || fetch(request);
    })());
  }
});

// Stable per-message tags make provider retries replace the same notification.
self.addEventListener('push', event => {
  let data = { title: 'Wisp', body: 'New message' };
  try { data = { ...data, ...event.data.json() }; } catch {}
  const tag = data.testId ? JSON.stringify(['push-test',data.testId]) : data.messageId ? JSON.stringify([data.chatId || '', data.messageId]) : undefined;
  event.waitUntil(self.registration.showNotification(data.title, {
    body: data.body, icon: '/icons/icon-192.png', badge: '/icons/icon-192.png',
    tag, renotify: false, data: { chatId: data.chatId, messageId: data.messageId, ...(data.testId?{testId:data.testId}:{}) },
  }));
});

self.addEventListener('notificationclick', event => {
  event.notification.close();
  const data = event.notification.data || {};
  const target = new URL('/', self.location.origin);
  if (data.chatId) target.searchParams.set('chat', data.chatId);
  if (data.messageId) target.searchParams.set('message', data.messageId);
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    // Restrict reuse to this app and prefer the already focused window.
    const candidates = windows.filter(client => {
      const url = new URL(client.url);
      return url.origin === target.origin && url.pathname === '/';
    }).sort((a, b) => Number(b.focused) - Number(a.focused));
    for (const client of candidates) {
      try {
        const navigated = await client.navigate(target.href);
        if (navigated) { await navigated.focus(); return; }
      } catch { /* Closed window or failed navigation: try another client. */ }
    }
    await self.clients.openWindow(target.href);
  })());
});

self.addEventListener('pushsubscriptionchange', event => {
  event.waitUntil((async () => {
    // Browsers differ in support. The foreground app also reconciles on resume.
    const sub = event.newSubscription || (event.oldSubscription &&
      await self.registration.pushManager.subscribe(event.oldSubscription.options));
    if (!sub) return;
    async function sync(method, body) {
      const response = await fetch('/api/push/subscribe', {
        method, headers: { 'content-type': 'application/json' }, body: JSON.stringify(body),
        signal: AbortSignal.timeout(10000),
      });
      if (!response.ok || !(response.headers.get('content-type') || '').includes('application/json') ||
          (await response.json()).ok !== true) throw new Error('Push subscription synchronization failed');
    }
    await sync('POST', sub);
    if (event.oldSubscription && event.oldSubscription.endpoint !== sub.endpoint) {
      await sync('DELETE', { endpoint: event.oldSubscription.endpoint });
    }
  })());
});
