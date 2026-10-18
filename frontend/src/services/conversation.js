import { mergeKnownReactions } from '../core/reactions.js';
export function createConversation({ app, store, apiJson, onRetryMedia, onReply, onReaction, onQuote }) {
  let initialScrollTimer = null;
  let initialScrollObserver = null;
  const state = new Proxy({}, {
    get: (_, key) => store.get(key),
    set: (_, key, value) => { store.set(key, value); return true; },
  });
  const $ = selector => app.querySelector(selector);

  function clearInitialTimer() {
    clearTimeout(initialScrollTimer);
    initialScrollTimer = null;
    initialScrollObserver?.disconnect();
    initialScrollObserver = null;
  }

  function resetForChat() {
    clearInitialTimer();
    store.update({ messages: [], hasOlder: false, loadingOlder: false, olderLoadError: false, messageLoading: false, newWhileReading: 0, scrollMode: 'initial' });
  }

  function ensureMessageList() {
    let el = $('#messages');
    if (el?.tagName === 'WISP-MESSAGE-LIST') return el;
    const next = document.createElement('wisp-message-list');
    next.id = 'messages';
    next.className = 'messages';
    el?.replaceWith(next);
    wireMessageList(next);
    return next;
  }

  function cancelInitialScroll() {
    if (!['initial','follow'].includes(state.scrollMode)) return;
    state.scrollMode = 'idle';
    clearInitialTimer();
  }

  function wireMessageList(messageList = $('#messages')) {
    if (!messageList || messageList.tagName !== 'WISP-MESSAGE-LIST' || messageList.dataset.wired) return;
    messageList.dataset.wired = 'true';
    messageList.addEventListener('load-older', () => loadOlder().catch(console.error));
    messageList.addEventListener('retry-media', event => onRetryMedia?.(event.detail.messageId));
    messageList.addEventListener('reply-select', event => onReply?.(event.detail.message));
    messageList.addEventListener('reaction-set', event => onReaction?.(event.detail));
    messageList.addEventListener('quote-open', event => onQuote?.(event.detail.quote));

    // Browser scroll anchoring and late media/layout changes can emit scroll
    // events while the initial bottom lock is active. Only explicit user input
    // may cancel that lock; a generic scroll event must not.
    messageList.addEventListener('wheel', cancelInitialScroll, { passive: true });
    messageList.addEventListener('touchstart', cancelInitialScroll, { passive: true });
    messageList.addEventListener('pointerdown', cancelInitialScroll);

    messageList.addEventListener('scroll', () => {
      if (!state.chatPreparing && state.scrollMode !== 'initial' && state.scrollMode !== 'prepend' && messageList.scrollTop < 160 && !state.olderLoadError) loadOlder().catch(console.error);
      if (isNearBottom(messageList)) {
        state.newWhileReading = 0;
      }
      updateNewButton();
    });
  }

  async function renderMessages({ loading = state.messageLoading } = {}) {
    const el = ensureMessageList();
    if (!el) return null;
    el.chatId=state.selectedChatId;
    el.messages = state.messages;
    el.sendEnabled = state.sendEnabled && !state.mirrorOnly;
    el.pendingReactions = state.pendingReactions;
    el.hasOlder = state.hasOlder;
    el.loading = loading;
    el.loadingOlder = state.loadingOlder;
    el.olderError = state.olderLoadError;
    el.hasSelection = Boolean(state.selectedChatId);
    el.isGroup = state.chats.find(chat => chat.id === state.selectedChatId)?.type === 'group';
    el.requestUpdate();
    await el.updateComplete;
    if(!state.chatPreparing)wireInitialMediaScroll(el);
    updateNewButton();
    return el;
  }

  function captureScrollAnchor(el = $('#messages')) {
    if (!el) return null;
    const box = el.getBoundingClientRect();
    for (const node of el.querySelectorAll('[data-message-id]')) {
      const rect = node.getBoundingClientRect();
      if (rect.bottom >= box.top) return { id: node.dataset.messageId, offset: rect.top - box.top };
    }
    return null;
  }

  function restoreScrollAnchor(anchor, el = $('#messages')) {
    if (!anchor || !el) return;
    const node = [...el.querySelectorAll('[data-message-id]')].find(item => item.dataset.messageId === anchor.id);
    if (!node) return;
    const box = el.getBoundingClientRect();
    const rect = node.getBoundingClientRect();
    el.scrollTop += rect.top - box.top - anchor.offset;
  }

  function positionMessages({anchor=null,bottom=false,messageId=null}={}){
    clearInitialTimer();state.scrollMode='idle';
    const el=$('#messages');if(!el)return;
    if(messageId){[...el.querySelectorAll('[data-message-id]')].find(node=>node.dataset.messageId===messageId)?.scrollIntoView({block:'center'})}
    else if(bottom||!anchor)el.scrollTop=el.scrollHeight;
    else restoreScrollAnchor(anchor,el);
  }

  async function loadOlder() {
    if (state.chatPreparing || !state.selectedChatId || state.loadingOlder || !state.hasOlder || !state.messages.length) return;
    const chatId = state.selectedChatId;
    state.loadingOlder = true;
    state.olderLoadError = false;
    state.scrollMode = 'prepend';
    await renderMessages();
    const el = $('#messages');
    const anchor = captureScrollAnchor(el);
    const oldHeight = el?.scrollHeight || 0;
    const oldTop = el?.scrollTop || 0;
    const first = state.messages[0];
    try {
      const url = `/api/chats/${encodeURIComponent(state.selectedChatId)}/messages?limit=50&beforeTs=${encodeURIComponent(first.timestamp)}&beforeId=${encodeURIComponent(first.id)}`;
      const older = await apiJson(url);
      if (chatId !== state.selectedChatId) return;
      const known = new Set(state.messages.map(message => message.id));
      const fresh = mergeKnownReactions(older,state.reactionStates).filter(message => !known.has(message.id));
      state.messages = [...fresh, ...state.messages];
      state.messageCache.set(state.selectedChatId,{messages:[...state.messages],hasOlder:older.length>0,at:Date.now()});
      // A short local page does not prove end-of-history: WhatsApp pages can
      // contain filtered protocol messages, and the backend may still have a
      // valid on-demand cursor. Keep paging until the backend explicitly
      // returns an empty page.
      state.hasOlder = older.length > 0;
      await renderMessages();
      const next = $('#messages');
      if (next && state.scrollMode === 'prepend') {
        if (anchor) restoreScrollAnchor(anchor, next);
        else next.scrollTop = oldTop + (next.scrollHeight - oldHeight);
      }
    } catch (error) {
      if (chatId !== state.selectedChatId) return;
      state.olderLoadError = true;
      throw error;
    } finally {
      if (chatId !== state.selectedChatId) return;
      if(state.scrollMode==='prepend')state.scrollMode = 'idle';
      state.loadingOlder = false;
      await renderMessages();
    }
  }

  function isNearBottom(el = $('#messages')) {
    return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 100;
  }

  function setScrollTop(el, value) {
    el.scrollTop = value;
  }

  function wireInitialMediaScroll(el = $('#messages')) {
    if (!el || state.scrollMode !== 'initial') return;

    const keep = () => {
      if (state.scrollMode !== 'initial') return;
      setScrollTop(el, el.scrollHeight);
    };

    const wireMedia = () => {
      el.querySelectorAll('img').forEach(image => {
        if (!image.complete && !image.dataset.initialScrollWired) {
          image.dataset.initialScrollWired = 'true';
          image.addEventListener('load', keep, { once: true });
          image.addEventListener('error', keep, { once: true });
        }
      });
      el.querySelectorAll('video').forEach(video => {
        if (!video.dataset.initialScrollWired) {
          video.dataset.initialScrollWired = 'true';
          video.addEventListener('loadedmetadata', keep, { once: true });
        }
      });
    };

    if (!initialScrollObserver) {
      initialScrollObserver = new MutationObserver(() => {
        if (state.scrollMode !== 'initial') return;
        wireMedia();
        keep();
      });
      initialScrollObserver.observe(el, { childList: true, subtree: true });
    }

    wireMedia();
    keep();

    // Keep the initial bottom anchor alive long enough for pending history media
    // and late Lit/layout work. Any real user scroll/touch/pointer input cancels
    // this immediately via cancelInitialScroll().
    if (!initialScrollTimer) {
      initialScrollTimer = setTimeout(() => {
        if (state.scrollMode === 'initial') state.scrollMode = 'idle';
        clearInitialTimer();
      }, 10000);
    }
  }

  function scrollBottom({ initial = false, force = false } = {}) {
    if (state.scrollMode === 'prepend' && !force) return;
    const el = $('#messages');
    if (!el) return;
    if (initial) state.scrollMode = 'initial';
    else if (state.scrollMode !== 'initial') state.scrollMode = 'follow';
    const chatId=state.selectedChatId;
    const go = () => {
      if (chatId!==state.selectedChatId || !['initial','follow'].includes(state.scrollMode) || state.connection.state!=='connected') return;
      setScrollTop(el, el.scrollHeight);
      state.newWhileReading = 0;
      updateNewButton();
    };
    // Position the already-rendered page before the next paint. Later frames
    // only account for layout settling and must still belong to this chat.
    go();
    requestAnimationFrame(() => {
      if(chatId!==state.selectedChatId)return;
      go();
      requestAnimationFrame(() => {
        if(chatId!==state.selectedChatId)return;
        go();
        wireInitialMediaScroll(el);
        if (!initial && state.scrollMode === 'follow') state.scrollMode = 'idle';
      });
    });
  }

  function updateNewButton() {
    const button = $('#newMessages');
    if (!button) return;
    button.hidden = Boolean(state.imageViewer) || state.chatPreparing || !state.selectedChatId || state.view !== 'chat' || isNearBottom();
    button.setAttribute('aria-label','Scroll to latest message');
    button.title = 'Scroll to latest message';
    button.style.bottom = `${($('#composer')?.offsetHeight || 0) + 20}px`;
    button.textContent = state.newWhileReading ? `↓ ${state.newWhileReading} new message${state.newWhileReading === 1 ? '' : 's'}` : '↓';
  }

  async function highlightMessage(messageId) {
    clearInitialTimer();
    state.scrollMode = 'idle';
    await renderMessages();
    const selector = `[data-message-id="${CSS.escape(messageId)}"]`;
    const message = $(selector);
    message?.scrollIntoView({ block: 'center' });
    if (message) {
      message.classList.add('highlight');
      setTimeout(() => message.classList.remove('highlight'), 2200);
    }
  }

  return {
    resetForChat,
    positionMessages,
    wireMessageList,
    renderMessages,
    loadOlder,
    captureScrollAnchor,
    restoreScrollAnchor,
    isNearBottom,
    scrollBottom,
    updateNewButton,
    highlightMessage,
    clearInitialTimer,
  };
}
