export function createGallery({ app, store, apiJson, openMessage }) {
  const state = new Proxy({}, {
    get: (_, key) => store.get(key),
    set: (_, key, value) => { store.set(key, value); return true; },
  });
  const $ = selector => app.querySelector(selector);
  let requestToken = 0;

  function ensureGallery() {
    let el = $('#messages');
    if (el?.tagName === 'WISP-GALLERY') return el;
    const next = document.createElement('wisp-gallery');
    next.id = 'messages';
    next.className = 'messages';
    el?.replaceWith(next);
    wire(next);
    return next;
  }

  function wire(el = $('#messages')) {
    if (!el || el.tagName !== 'WISP-GALLERY' || el.dataset.wired) return;
    el.dataset.wired = 'true';
    el.addEventListener('gallery-kind', event => open(event.detail.kind));
    el.addEventListener('gallery-jump', event => {
      if (state.selectedChatId) openMessage(state.selectedChatId, event.detail.messageId);
    });
  }

  async function render({ loading = false, rows = [], error = '' } = {}) {
    const el = ensureGallery();
    if (!el) return null;
    el.kind = state.galleryKind;
    el.rows = rows;
    el.loading = loading;
    el.error = error;
    el.requestUpdate();
    await el.updateComplete;
    return el;
  }

  async function open(kind = state.galleryKind || 'media') {
    if (!state.selectedChatId) return;
    state.view = 'gallery';
    state.galleryKind = kind;
    const token = ++requestToken;
    await render({ loading: true });
    const chatId = state.selectedChatId;
    const url = kind === 'links'
      ? `/api/chats/${encodeURIComponent(chatId)}/links?limit=100`
      : `/api/chats/${encodeURIComponent(chatId)}/media?kind=${encodeURIComponent(kind)}&limit=100`;
    try {
      const rows = await apiJson(url);
      if (token !== requestToken || state.selectedChatId !== chatId || state.view !== 'gallery' || state.galleryKind !== kind) return;
      await render({ rows });
    } catch (error) {
      if (token !== requestToken || state.view !== 'gallery') return;
      console.error('Gallery load failed', error);
      await render({ error: 'Could not load gallery.' });
    }
  }

  function refresh() {
    if (state.view === 'gallery' && state.selectedChatId) return open(state.galleryKind);
  }

  function cancel() { requestToken++; }

  return { open, refresh, render, wire, cancel };
}
