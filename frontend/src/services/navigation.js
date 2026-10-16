import { apiJson } from '../lib/api.js';

/**
 * Owns desktop chat/search navigation and the search request lifecycle.
 * UI side effects that belong to the conversation (opening a chat/message or
 * focusing the composer) are injected so this module stays transport/UI agnostic.
 */
export function createNavigation({ app, store, selectChat, openMessage, focusComposer }) {
  let searchTimer = null;
  let globalListenersInstalled = false;
  const state = new Proxy({}, {
    get: (_, key) => store.get(key),
    set: (_, key, value) => { store.set(key, value); return true; },
  });
  const $ = (selector) => document.querySelector(selector);
  const desktop = () => window.matchMedia('(min-width: 701px)').matches;
  const keyboardItems = () => [...document.querySelectorAll('#chatList .chat,#chatList .search-hit')];

  function renderChatList() {
    const list = $('#chatList');
    if (!list) return;
    list.chats = state.chats;
    list.selected = state.selectedChatId;
    list.filter = state.filter;
    list.searchResults = state.searchResults;
    list.unread = state.unread;
    list.keyboardSelection = state.keyboardSelection;
    list.requestUpdate();
  }

  function updateKeyboardModeIndicator() {
    const hint = $('#keyboardHint');
    if (!hint) return;
    const active = document.activeElement;
    const isComposer = active?.id === 'composerInput';
    const isSearch = active?.id === 'search';
    hint.hidden = !desktop() || !state.keyboardNavActive || isComposer || isSearch;
    hint.classList.toggle('active', !hint.hidden);
    hint.title = hint.hidden ? '' : 'Arrow keys navigate · Enter opens';
  }

  function updateActivePane() {
    if (!desktop()) return;
    app.classList.toggle('pane-left', state.activePane === 'left');
    app.classList.toggle('pane-right', state.activePane === 'right');
  }

  function applyKeyboardSelection() {
    const list = $('#chatList');
    if (list) {
      list.keyboardSelection = state.keyboardSelection;
      list.requestUpdate();
    }
    const items = keyboardItems();
    items.forEach(el => el.classList.toggle('keyboard-selected', Boolean(state.keyboardSelection) && el.dataset.key === state.keyboardSelection));
    items.find(el => el.dataset.key === state.keyboardSelection)?.scrollIntoView({ block: 'nearest' });
    updateKeyboardModeIndicator();
  }

  function activateChatPane() {
    state.activePane = 'left';
    state.keyboardNavActive = true;
    document.activeElement?.blur?.();
    if (!state.keyboardSelection || !keyboardItems().some(el => el.dataset.key === state.keyboardSelection)) {
      state.keyboardSelection = state.selectedChatId ? `chat:${state.selectedChatId}` : (keyboardItems()[0]?.dataset.key || null);
    }
    applyKeyboardSelection();
    updateActivePane();
  }

  function activateConversationPane() {
    state.activePane = 'right';
    state.keyboardNavActive = false;
    applyKeyboardSelection();
    updateActivePane();
    focusComposer();
  }

  function moveKeyboardSelection(delta) {
    const items = keyboardItems();
    if (!items.length) return;
    state.activePane = 'left';
    state.keyboardNavActive = true;
    updateActivePane();
    let index = items.findIndex(el => el.dataset.key === state.keyboardSelection);
    if (index < 0) {
      const active = items.findIndex(el => el.classList.contains('active'));
      index = active >= 0 ? active : (delta > 0 ? -1 : 0);
    }
    index = (index + delta + items.length) % items.length;
    state.keyboardSelection = items[index].dataset.key;
    applyKeyboardSelection();
  }

  function activateKeyboardSelection() {
    keyboardItems().find(el => el.dataset.key === state.keyboardSelection)?.click();
  }

  function clearSearch() {
    state.filter = '';
    state.searchResults = null;
    clearTimeout(searchTimer);
    const input = $('#search');
    if (input) input.value = '';
    renderChatList();
  }

  async function doSearch() {
    const query = state.filter.trim();
    if (!query) {
      state.searchResults = null;
      renderChatList();
      return;
    }
    state.searchResults = await apiJson(`/api/search?q=${encodeURIComponent(query)}&limit=50`);
    renderChatList();
  }

  function restoreChatKeyboardNavigation(chatId) {
    document.activeElement?.blur?.();
    state.activePane = 'left';
    state.keyboardNavActive = true;
    state.keyboardSelection = chatId ? `chat:${chatId}` : (keyboardItems()[0]?.dataset.key || null);
    applyKeyboardSelection();
    updateActivePane();
  }

  async function openSearchChat(chatId) {
    clearSearch();
    await selectChat(chatId);
    restoreChatKeyboardNavigation(chatId);
  }

  async function openSearchMessage(chatId, messageId) {
    clearSearch();
    await openMessage(chatId, messageId);
    restoreChatKeyboardNavigation(chatId);
  }

  function wireChatList() {
    const list = $('#chatList');
    if (!list) return;
    if (!list.dataset.wired) {
      list.dataset.wired = 'true';
      list.addEventListener('chat-open', event => {
        const { id, fromSearch } = event.detail;
        state.keyboardSelection = `chat:${id}`;
        if (fromSearch) void openSearchChat(id);
        else void selectChat(id);
      });
      list.addEventListener('message-open', event => {
        const { chatId, messageId } = event.detail;
        state.keyboardSelection = `message:${messageId}`;
        void openSearchMessage(chatId, messageId);
      });
    }
    renderChatList();
    applyKeyboardSelection();
  }

  function wireShell() {
    const search = $('#search');
    search?.addEventListener('focus', () => {
      state.activePane = 'left';
      state.keyboardNavActive = false;
      applyKeyboardSelection();
      updateActivePane();
    });
    search?.addEventListener('input', event => {
      state.filter = event.target.value;
      clearTimeout(searchTimer);
      searchTimer = setTimeout(() => void doSearch(), 180);
    });
    wireChatList();
    updateActivePane();
  }

  function installGlobalListeners() {
    if (globalListenersInstalled) return;
    globalListenersInstalled = true;
    document.addEventListener('keydown', event => {
      if (event.defaultPrevented || event.target.closest?.('.message-actions,.message-popover,.reaction-chips,.quoted-message,wisp-emoji-picker') || !desktop() || state.settingsOpen || state.connection.state !== 'connected') return;
      const active = document.activeElement;
      const isComposer = active?.id === 'composerInput';
      const isSearch = active?.id === 'search';
      const isEditable = active && active.matches('input,textarea,[contenteditable="true"]');
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f') {
        event.preventDefault();
        state.activePane = 'left';
        state.keyboardNavActive = false;
        const input = $('#search');
        input?.focus({ preventScroll: true });
        input?.select();
        updateActivePane();
        updateKeyboardModeIndicator();
        return;
      }
      if (event.key === 'Escape' && isSearch) {
        event.preventDefault();
        if (state.filter) clearSearch();
        active.blur();
        activateChatPane();
        return;
      }
      if (event.key === 'Escape' && isComposer) {
        event.preventDefault();
        active.blur();
        activateChatPane();
        return;
      }
      if (isComposer || (isEditable && !isSearch)) return;
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault();
        moveKeyboardSelection(event.key === 'ArrowDown' ? 1 : -1);
        return;
      }
      if (event.key === 'Enter' && state.keyboardSelection) {
        event.preventDefault();
        activateKeyboardSelection();
      }
    });
    document.addEventListener('pointerdown', event => {
      if (!desktop() || state.settingsOpen || state.connection.state !== 'connected') return;
      if (event.target.closest('button,input,textarea,a,[contenteditable="true"],.chat,.search-hit,.bubble,.media-tile,.link-card,.modal')) return;
      if (event.target.closest('.sidebar')) activateChatPane();
      else if (event.target.closest('.conversation')) activateConversationPane();
    });
    document.addEventListener('focusin', updateKeyboardModeIndicator);
    document.addEventListener('focusout', () => requestAnimationFrame(updateKeyboardModeIndicator));
  }

  return {
    renderChatList,
    wireShell,
    installGlobalListeners,
    updateActivePane,
    updateKeyboardModeIndicator,
  };
}
