const CACHE='wisp-1.0.0-rc.1';
const SHELL=['/','/manifest.webmanifest','/icons/icon.svg','/icons/icon-192.png','/icons/icon-512.png','/icons/icon-maskable-512.png'];
self.addEventListener('install',e=>e.waitUntil(caches.open(CACHE).then(c=>c.addAll(SHELL)).then(()=>self.skipWaiting())));
self.addEventListener('activate',e=>e.waitUntil(Promise.all([self.clients.claim(),caches.keys().then(keys=>Promise.all(keys.filter(k=>k!==CACHE).map(k=>caches.delete(k))))])));
self.addEventListener('fetch',e=>{if(e.request.method!=='GET')return;const u=new URL(e.request.url);if(u.origin!==self.location.origin||u.pathname.startsWith('/api/'))return;e.respondWith(fetch(e.request).then(r=>{if(r.ok){const copy=r.clone();caches.open(CACHE).then(c=>c.put(e.request,copy))}return r}).catch(()=>caches.match(e.request).then(r=>r||caches.match('/'))))});
// Stable per-message tags make provider retries replace the same notification.
self.addEventListener('push', event => {
  let data = { title: 'Wisp', body: 'New message' };
  try { data = { ...data, ...event.data.json() }; } catch {}
  const tag = data.messageId ? JSON.stringify([data.chatId || '', data.messageId]) : undefined;
  event.waitUntil(self.registration.showNotification(data.title, {
    body: data.body, icon: '/icons/icon-192.png', badge: '/icons/icon-192.png',
    tag, renotify: false, data: { chatId: data.chatId, messageId: data.messageId },
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
