import { dayKey } from '../lib/utils.js';
export function sameMessageGroup(previous, current) {
  if (!previous || !current || previous.deleted || current.deleted) return false;
  const gap = current.timestamp - previous.timestamp;
  const sender = m => m.from_me ? 'self' : (m.sender_id || m.sender_name || m.chat_id);
  return Boolean(previous.from_me) === Boolean(current.from_me) && sender(previous) === sender(current)
    && previous.chat_id === current.chat_id && dayKey(previous.timestamp) === dayKey(current.timestamp)
    && gap >= 0 && gap <= 300;
}
