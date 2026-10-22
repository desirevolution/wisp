// The server owns counts and monotonically increasing read positions.
export function unreadCount(p){
  return p?.closed?0:Math.max(0,(p?.unread_sequence||0)-(p?.read_sequence||0));
}

export function createReadTracker({getState,canRead,apiJson,onState,onRead=()=>{}}){
  const pending=new Set(),cleaned=new Map();
  async function markVisible(){
    const state=getState(),id=state.selectedChatId,p=state.chatStates.get(id);
    if(!canRead()||!p||pending.has(id))return;
    // Never acknowledge a server snapshot whose latest incoming message has
    // not reached this view yet (late HTTP responses, SSE gaps, chat switches).
    if(!p.last_incoming_id||!state.messages.some(m=>m.id===p.last_incoming_id))return;
    const through=p.unread_sequence;
    const ids=new Set(state.messages.map(m=>m.id));
    if(!unreadCount(p)){
      if(p.read_sequence>=through&&cleaned.get(id)!==p.version){
        cleaned.set(id,p.version);void Promise.resolve(onRead(id,ids)).catch(()=>{});
      }
      return;
    }
    pending.add(id);
    let succeeded=false;
    try{
      const result=await apiJson(`/api/chats/${encodeURIComponent(id)}/read`,{
        method:'POST',headers:{'content-type':'application/json'},
        body:JSON.stringify({through_sequence:through}),signal:AbortSignal.timeout(10000),
      });
      if(result?.chat_id!==id||!Number.isSafeInteger(result.version)||!Number.isSafeInteger(result.read_sequence)||result.read_sequence<through)throw new Error('Read position was not confirmed');
      onState(result);succeeded=true;
      cleaned.set(id,result.version);
      // Only remove notifications covered by this read, never another chat,
      // newly arrived messages or diagnostic test notifications.
      void Promise.resolve(onRead(id,ids)).catch(()=>{});
    }catch{/* Keep the server count; a later resume/event/open retries. */}
    finally{pending.delete(id)}
    if(succeeded)void markVisible();
  }
  return {markVisible};
}

export async function closeReadNotifications(chatId,messageIds){
  if(!globalThis.navigator?.serviceWorker?.getRegistration)return;
  const reg=await navigator.serviceWorker.getRegistration();
  if(!reg?.getNotifications)return;
  for(const n of await reg.getNotifications()){
    if(!n.data?.testId&&n.data?.chatId===chatId&&messageIds.has(n.data?.messageId))n.close();
  }
}
