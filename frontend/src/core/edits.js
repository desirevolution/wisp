// Server revisions grow for every received version, even for late older edits.
const edits=new Map();
const key=(chat,id)=>JSON.stringify([chat,id]);
export function rememberEdit(state){
  if(!state?.chat_id||!state.message_id||!state.edited_at)return;
  const k=key(state.chat_id,state.message_id),previous=edits.get(k);
  if(!previous||Number(state.edit_version)>Number(previous.edit_version))edits.set(k,state);
}
export function learnMessageEdits(messages){
  for(const m of messages)if(m.edited)rememberEdit({chat_id:m.chat_id,message_id:m.id,body:m.body,edited_at:m.edited_at,edit_version:m.edit_version,edit_history:m.edit_history,original_available:m.edit_original_available});
}
export function mergeEdited(m){
  const state=edits.get(key(m.chat_id,m.id));
  let next=m;
  if(state&&Number(state.edit_version)>=Number(m.edit_version||0))next={...m,body:state.body,edited:true,edited_at:state.edited_at,edit_version:state.edit_version,edit_history:state.edit_history||[],edit_original_available:state.original_available};
  if(next.reply){const q=next.reply;const original=edits.get(key(q.chat_id||m.chat_id,q.message_id));
    if(original&&original.edited_at>m.timestamp*1000)next={...next,reply:{...q,original_edited:true}};
  }
  return next;
}
export function applyEditState(messages,state){rememberEdit(state);return messages.map(mergeEdited)}
export function editStateFor(chat,id){return edits.get(key(chat,id))}
