// Only text/reply are persisted. Files, pending requests and errors stay in memory.
const STORAGE_KEY = 'wisp.drafts.v1';
function browserStorage() { try { return globalThis.localStorage; } catch { return null; } }
export function createDrafts({ storage = browserStorage() } = {}) {
  const entries = new Map(), listeners = new Set();
  let saved = {};
  try { const value = JSON.parse(storage?.getItem(STORAGE_KEY) || '{}'); if (value && typeof value === 'object' && !Array.isArray(value)) saved = value; } catch { /* unavailable or corrupt */ }
  const blank = () => ({ text: '', reply: null, image: null, sending: false, error: '', revision: 0, serverRevision: 0 });
  function get(chat) {
    if (!entries.has(chat)) {
      const value = Object.hasOwn(saved, chat) ? saved[chat] : null;
      const text = typeof value?.text === 'string' ? value.text.slice(0, 4000) : '';
      const reply = value?.reply && typeof value.reply.message_id === 'string' ? value.reply : null;
      entries.set(chat, { ...blank(), text, reply, serverRevision: Number(value?.serverRevision)||0 });
    }
    return entries.get(chat);
  }
  function update(chat, patch) {
    const old = get(chat);
    if ('image' in patch && old.image && old.image !== patch.image) URL.revokeObjectURL(old.image.url);
    const revision = old.revision + (('text' in patch || 'reply' in patch || 'image' in patch) ? 1 : 0);
    entries.set(chat, { ...old, ...patch, revision });
    if (chat && ('text' in patch || 'reply' in patch || 'serverRevision' in patch)) {
      const { text, reply, serverRevision } = get(chat);
      if (text || reply || serverRevision) Object.defineProperty(saved, chat, { value: { text, reply, serverRevision }, writable: true, configurable: true, enumerable: true });
      else delete saved[chat];
      try { storage?.setItem(STORAGE_KEY, JSON.stringify(saved)); } catch { /* keep in-memory drafts functional */ }
    }
    for (const listener of listeners) listener(chat);
    return get(chat);
  }
  function invalidate(chat, serverRevision) {
    if (!Number.isSafeInteger(serverRevision) || serverRevision <= get(chat).serverRevision) return false;
    update(chat, { text: '', reply: null, image: null, preparingImage: false, error: '', serverRevision });
    return true;
  }
  return { get, update, invalidate, subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); } };
}
export const sessionDrafts = createDrafts();
