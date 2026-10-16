// In-memory, per-chat drafts. A reload deliberately starts a new session.
export function createDrafts() {
  const entries = new Map(), listeners = new Set();
  function get(chat) {
    if (!entries.has(chat)) entries.set(chat, { text: '', reply: null, image: null, sending: false, error: '', revision: 0 });
    return entries.get(chat);
  }
  function update(chat, patch) {
    const old = get(chat);
    if ('image' in patch && old.image && old.image !== patch.image) URL.revokeObjectURL(old.image.url);
    const revision = old.revision + (('text' in patch || 'reply' in patch || 'image' in patch) ? 1 : 0);
    entries.set(chat, { ...old, ...patch, revision });
    for (const listener of listeners) listener(chat);
    return get(chat);
  }
  return { get, update, subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); } };
}
export const sessionDrafts = createDrafts();
