import { learnMessageEdits, mergeEdited } from './edits.js';
import { mergeDeleted } from './deletions.js';
// Ignore late HTTP/SSE snapshots that predate a state already displayed.
export function applyReactionState(messages, state) {
  return messages.map(message => message.id === state.message_id && message.chat_id === state.chat_id &&
    Number(state.version || 0) >= Number(message.reaction_version || 0)
    ? { ...message, reactions: state.reactions || [], reaction_version: state.version || 0 } : message);
}

export function mergeKnownReactions(messages, states) {
  learnMessageEdits(messages);
  return messages.map(raw => {
    const message=mergeDeleted(mergeEdited(raw));
    const state = states.get(message.id);
    return state ? applyReactionState([message], state)[0] : message;
  });
}
