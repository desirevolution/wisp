// Deletions are monotonic, including when an earlier HTTP response arrives late.
const known = new Map();
export function rememberDeletion(data){
  if(!known.has(data.chat_id))known.set(data.chat_id,new Set());
  known.get(data.chat_id).add(data.message_id);
}
export function mergeDeleted(message){
  const deleted=message.deleted||known.get(message.chat_id)?.has(message.id)||false;
  const q=message.reply;
  return {...message,deleted,...(q?{reply:{...q,deleted:q.deleted||known.get(q.chat_id||message.chat_id)?.has(q.message_id)||false}}:{})};
}
