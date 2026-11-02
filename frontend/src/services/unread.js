import {chatMuted} from '../core/chat-state.js';

// The server owns counts and monotonically increasing read positions.
export function unreadCount(p){
  return p?.closed?0:Math.max(0,(p?.unread_sequence||0)-(p?.read_sequence||0));
}

// The title reflects the shared unread state, independent of the selected chat,
// search filter or document visibility. Muted/closed chats stay quiet.
export function unreadTabTitle({unread,chatStates,knownChats},now=Date.now()){
 let total=0;
 for(const [id,count] of unread){
  const pref=chatStates.get(id)||knownChats.get(id);
  if(!pref?.closed&&!chatMuted(pref,now)&&Number.isSafeInteger(count)&&count>0)total+=count;
 }
 return total?`(${total}) Wisp`:'Wisp';
}

export function createReadTracker({getState,canRead,apiJson,onState,onRead=()=>{},isRendered=()=>true}){
  const pending=new Set(),cleaned=new Map(),synced=new Map();
  async function markVisible(){
    const state=getState(),id=state.selectedChatId,p=state.chatStates.get(id);
    const sync=state.readMode==='devices'||state.readMode==='receipts';
    if(!canRead()||!id||(!p&&!sync)||pending.has(id))return;
    // An unread snapshot still requires its most recent incoming row to have
    // reached the prepared DOM. A zero Wisp count must not block device sync.
    if(unreadCount(p)&&(!p.last_incoming_id||!state.messages.some(m=>m.id===p.last_incoming_id)||!isRendered(p.last_incoming_id)))return;
    const through=p?.unread_sequence||0;
    const ids=new Set(state.messages.map(m=>m.id));
    let last=null;
    if(sync){
      const confirmed=state.messages.filter(m=>!m.outbox);
      const chat=state.chats?.find(c=>c.id===id)||state.knownChats?.get(id);
      // Search/context views must not acknowledge a newer, unloaded timeline.
      if(chat?.last_message_id&&!confirmed.some(m=>m.id===chat.last_message_id))return;
      last=confirmed.reduce((latest,m)=>!latest||(m.timestamp||0)>(latest.timestamp||0)||((m.timestamp||0)===(latest.timestamp||0)&&m.id>latest.id)?m:latest,null);
      if(!last||!isRendered(last.id))return;
    }
    const syncKey=sync?JSON.stringify([state.readMode,last.id,through]):null;
    if(!unreadCount(p)&&(!sync||synced.get(id)===syncKey)){
      if(p&&p.read_sequence>=through&&cleaned.get(id)!==p.version){
        cleaned.set(id,p.version);void Promise.resolve(onRead(id,ids)).catch(()=>{});
      }
      return;
    }
    pending.add(id);
    let succeeded=false;
    try{
      const result=await apiJson(`/api/chats/${encodeURIComponent(id)}/read`,{
        method:'POST',headers:{'content-type':'application/json'},
        body:JSON.stringify({through_sequence:through,...(last?{last_message_id:last.id}:{})}),signal:AbortSignal.timeout(10000),
      });
      if(result?.chat_id!==id||!Number.isSafeInteger(result.version)||!Number.isSafeInteger(result.read_sequence)||result.read_sequence<through)throw new Error('Read position was not confirmed');
      if(syncKey)synced.set(id,syncKey);
      onState(result);succeeded=true;
      cleaned.set(id,result.version);
      // Only remove notifications covered by this read, never another chat,
      // newly arrived messages or diagnostic test notifications.
      void Promise.resolve(onRead(id,ids)).catch(()=>{});
    }catch{/* Keep the server count; a later resume/event/open retries. */}
    finally{pending.delete(id)}
    if(succeeded||(getState().chatStates.get(id)?.unread_sequence||0)>through)void markVisible();
  }
  return {markVisible,reset:id=>synced.delete(id)};
}

export async function closeReadNotifications(chatId,messageIds){
  if(!globalThis.navigator?.serviceWorker?.getRegistration)return;
  const reg=await navigator.serviceWorker.getRegistration();
  if(!reg?.getNotifications)return;
  for(const n of await reg.getNotifications()){
    if(!n.data?.testId&&n.data?.chatId===chatId&&messageIds.has(n.data?.messageId))n.close();
  }
}
