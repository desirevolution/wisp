/**
 * Minimal observable application store.
 *
 * The store owns durable UI/application state while timers, DOM references and
 * transport handles stay in their respective orchestration modules. Mutations
 * go through set/update so later migration slices can subscribe without adding
 * a state-management dependency.
 */
export function createAppStore(initialState = {}) {
  let state = { ...initialState };
  const listeners = new Set();

  const notify = (changed) => {
    const snapshot = state;
    for (const listener of listeners) listener(snapshot, changed);
  };

  return {
    get state() {
      return state;
    },
    get(key) {
      return state[key];
    },
    set(key, value) {
      if (Object.is(state[key], value)) return value;
      state = { ...state, [key]: value };
      notify(new Set([key]));
      return value;
    },
    update(patch) {
      const next = typeof patch === 'function' ? patch(state) : patch;
      if (!next || typeof next !== 'object') return state;
      const changed = new Set(Object.keys(next).filter(key => !Object.is(state[key], next[key])));
      if (!changed.size) return state;
      state = { ...state, ...next };
      notify(changed);
      return state;
    },
    mutate(key, mutator) {
      const value = mutator(state[key]);
      return this.set(key, value === undefined ? state[key] : value);
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

export function createInitialAppState() {
  return {
    chats: [],
    selectedChatId: null,
    connection: { state: 'starting' },
    sendEnabled: false,
    mirrorOnly: false,
    filter: '',
    settingsOpen: false,
    messages: [],
    messageCache: new Map(),
    pendingReactions: new Set(),
    reactionStates: new Map(),
    messageLoading: false,
    hasOlder: false,
    loadingOlder: false,
    olderLoadError: false,
    newWhileReading: 0,
    view: 'chat',
    galleryKind: 'media',
    searchResults: null,
    followNewMessages: false,
    mqttEnabled: false, mqttConnected: false, mqttBroker: '', mqttTopicPrefix: 'wisp', mqttUsername: '', mqttPasswordSet: false, managed: {},
    unread: new Map(),
    scrollMode: 'idle',
    pushEnabled: false,
    installPrompt: null,
    isStandalone: window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true,
    keyboardSelection: null,
    keyboardNavActive: false,
    activePane: 'right',
  };
}
