import { attachKeyboardViewport } from './services/keyboard-viewport.js';
import { applyEditState, editStateFor } from './core/edits.js';
import { rememberDeletion } from './core/deletions.js';
import './components/wisp-chat-list.js';
import './components/wisp-message-list.js';
import './components/wisp-composer.js';
import './components/wisp-connection.js';
import './components/wisp-settings.js';
import './components/wisp-gallery.js';
import { $, esc, mediaLabel, displayName } from './lib/utils.js';
import { apiJson } from './lib/api.js';
import { createEventBus } from './core/event-bus.js';
import { connectLiveEvents } from './services/live-events.js';
import { applyReactionState, mergeKnownReactions } from './core/reactions.js';
import { createAppStore, createInitialAppState } from './core/app-store.js';
import { createNavigation } from './services/navigation.js';
import { createConversation } from './services/conversation.js';
import { createGallery } from './services/gallery.js';
import { createPwaService } from './services/pwa.js';
import { LitElement, html } from 'lit';

/** Root Lit component and top-level Wisp application orchestration. */
export class WispApp extends LitElement {
  createRenderRoot() { return this; }

  render() { return html`<div id="app" class="app"></div>`; }

  connectedCallback() {
    super.connectedCallback();
    const root = this.querySelector('#app');
    if (root && !this._keyboardViewport) this._keyboardViewport = attachKeyboardViewport(root);
  }
  disconnectedCallback() {
    this._keyboardViewport?.(); this._keyboardViewport = null;
    super.disconnectedCallback();
  }
  async firstUpdated() {
    const root = this.querySelector('#app');
    this._keyboardViewport = attachKeyboardViewport(root);
    try {
      await startWisp(root);
    } catch (error) {
      console.error('Wisp startup failed', error);
    }
  }
}

customElements.define('wisp-app', WispApp);

async function startWisp(app) {


const store=createAppStore(createInitialAppState());
const state=new Proxy({}, {
  get:(_,key)=>store.get(key),
  set:(_,key,value)=>{store.set(key,value);return true},
});
let noticeTimer=null,toastTimer=null,settingsReturnFocus=null,settingsKeyHandler=null;
const earlyMediaEvents=new Map();
function showToast(message,kind='error'){let el=$('#appToast');if(!el){el=document.createElement('div');el.id='appToast';app.appendChild(el)}el.className=`app-toast ${kind}`;el.textContent=message;el.hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>{el.hidden=true},4500)}
let navigation,conversation,gallery,pwa;
function connectionLabel(){if(state.connection.state==='connected'&&state.connection.syncing)return state.connection.syncPhase==='importing_history'?'Syncing chats…':state.connection.syncPhase==='settling_history'?'Finishing sync…':state.connection.syncPhase==='checking_history'?'Checking history…':'Syncing…';return({starting:'Starting…',disconnecting:'Disconnecting…',reconnecting:'Reconnecting…',waiting_for_qr:'Waiting for QR scan',authenticating:'Signing in…',connected:'Connected',disconnected:'Disconnected',error:'Connection error'}[state.connection.state]||state.connection.state)}
async function loadStatus({render=true}={}){const x=await apiJson('/api/status');state.connection=x.whatsapp;state.sendEnabled=Boolean(x.send?.enabled);if(render)renderShell()}
async function loadConfig(){const x=await apiJson('/api/config');state.sendEnabled=Boolean(x.sendEnabled);state.mirrorOnly=Boolean(x.mirrorOnly);state.followNewMessages=Boolean(x.followNewMessages);}
async function saveConfig(patch){const x=await apiJson('/api/config',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(patch)});state.sendEnabled=Boolean(x.sendEnabled);state.mirrorOnly=Boolean(x.mirrorOnly);state.followNewMessages=Boolean(x.followNewMessages);syncSettings()}async function loadChats({render=true}={}){state.chats=await apiJson('/api/chats');if(render)renderChatList();renderConversationHeader()}
let chatLoadToken=0;
const MESSAGE_CACHE_TTL=2*60*1000;
async function selectChat(id){
  // Selecting the already open chat is idempotent. In particular, repeated
  // pointer events must not rebuild the responsive shell while a load is active.
  if(id===state.selectedChatId&&state.view==='chat'){
    // Re-selecting the active chat must never toggle layout state. Restore the
    // chat-open class (important after responsive/back transitions) and stop.
    app.classList.add('open');
    state.unread.delete(id);
    renderChatList();
    renderConversationHeader();
    return;
  }
  const token=++chatLoadToken;
  state.selectedChatId=id;state.unread.delete(id);state.view='chat';conversation.resetForChat();app.classList.add('open');
  const cached=state.messageCache.get(id);
  if(cached?.messages?.length){
    state.messages=cached.messages;state.hasOlder=cached.hasOlder;state.messageLoading=false;
  }else{
    state.messageLoading=true;
  }
  renderShell();
  await conversation.renderMessages({loading:state.messageLoading});
  if(cached?.messages?.length){conversation.scrollBottom({initial:true});focusComposer()}
  try{
    const data=await apiJson(`/api/chats/${encodeURIComponent(id)}/messages?limit=50`);
    if(token!==chatLoadToken||state.selectedChatId!==id)return;
    state.messages=mergeKnownReactions(data,state.reactionStates);state.hasOlder=data.length>0;state.messageLoading=false;
    state.messageCache.set(id,{messages:data,hasOlder:state.hasOlder,at:Date.now()});
    // Keep the cache deliberately small and short-lived. It exists to make
    // back-and-forth chat switching instant, not to become a second database.
    for(const [chat,entry] of state.messageCache){if(Date.now()-entry.at>MESSAGE_CACHE_TTL)state.messageCache.delete(chat)}
    while(state.messageCache.size>8)state.messageCache.delete(state.messageCache.keys().next().value);
    await conversation.renderMessages({loading:false});conversation.scrollBottom({initial:true});focusComposer();
  }catch(err){
    if(token!==chatLoadToken||state.selectedChatId!==id)return;
    state.messageLoading=false;await conversation.renderMessages({loading:false});throw err;
  }
}
async function openMessage(chatId,messageId,loaded=null){
  const token=++chatLoadToken;
  state.selectedChatId=chatId;state.view='chat';state.unread.delete(chatId);conversation.resetForChat();app.classList.add('open');
  state.messageLoading=true;renderShell();
  try{
    const data=loaded||await apiJson(`/api/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/context`);
    if(token!==chatLoadToken||state.selectedChatId!==chatId)return;
    state.messages=mergeKnownReactions(data,state.reactionStates);state.hasOlder=true;state.messageLoading=false;
    await conversation.renderMessages({loading:false});
    await conversation.highlightMessage(messageId);
    wireComposer();
  }catch(err){
    if(token===chatLoadToken&&state.selectedChatId===chatId){state.messageLoading=false;await conversation.renderMessages({loading:false})}
    throw err;
  }
}
async function openQuote(quote){
  const origin=state.selectedChatId,chat=quote.chat_id||origin;
  if(chat===origin&&state.messages.some(m=>m.id===quote.message_id)){await conversation.highlightMessage(quote.message_id);return}
  try{
    const rows=await apiJson(`/api/chats/${encodeURIComponent(chat)}/messages/${encodeURIComponent(quote.message_id)}/context`);
    if(state.selectedChatId===origin)await openMessage(chat,quote.message_id,rows);
  }catch(error){showToast(error.status===404?'Original message is not available.':'Could not open the original message.','info')}
}
function selectReply(message){if(state.sendEnabled&&!state.mirrorOnly&&message.chat_id===state.selectedChatId)$('#composer')?.setReply(message)}
async function updateReactionView(data){
  const known=state.reactionStates.get(data.message_id);
  if(known&&known.version>data.version)return;
  state.reactionStates.delete(data.message_id);state.reactionStates.set(data.message_id,data);
  while(state.reactionStates.size>500)state.reactionStates.delete(state.reactionStates.keys().next().value);
  const cached=state.messageCache.get(data.chat_id);
  if(cached)state.messageCache.set(data.chat_id,{...cached,messages:applyReactionState(cached.messages,data)});
  if(state.selectedChatId!==data.chat_id)return;
  const anchor=conversation.captureScrollAnchor(),bottom=conversation.isNearBottom();
  state.messages=applyReactionState(state.messages,data);
  if(state.view==='chat'){await conversation.renderMessages();if(state.selectedChatId!==data.chat_id)return;if(bottom)conversation.scrollBottom();else conversation.restoreScrollAnchor(anchor)}
}
async function setReaction({message,emoji}){
  if(!state.sendEnabled||state.mirrorOnly||state.pendingReactions.has(message.id))return;
  state.pendingReactions.add(message.id);await conversation.renderMessages();
  try{
    const data=await apiJson(`/api/chats/${encodeURIComponent(message.chat_id)}/messages/${encodeURIComponent(message.id)}/reactions`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({emoji})});
    if(data?.ok!==true)throw new Error('Reaction could not be sent.');
    await updateReactionView(data);
  }catch(error){showToast(error.message||'Reaction could not be sent.')}
  finally{state.pendingReactions.delete(message.id);if(state.selectedChatId===message.chat_id&&state.view==='chat')await conversation.renderMessages()}
}
async function updateEditView(data){
  const anchor=conversation.captureScrollAnchor(),atBottom=conversation.isNearBottom();
  state.messages=applyEditState(state.messages,data);
  for(const [chat,cached] of state.messageCache)cached.messages=applyEditState(cached.messages,data);
  // Only the preview may change; never sort, increment unread, or follow a chat.
  const c=state.chats.find(c=>c.id===data.chat_id);
  if(c?.last_message_id===data.message_id){const latest=editStateFor(data.chat_id,data.message_id);if(latest)c.last_body=latest.body}
  renderChatList();
  if(state.view==='chat'){await conversation.renderMessages();if(atBottom)conversation.scrollBottom();else conversation.restoreScrollAnchor(anchor)}
}
function markDeleted(messages,data){rememberDeletion(data);return messages.map(m=>({...m,...(m.id===data.message_id&&m.chat_id===data.chat_id?{deleted:true}:{}),...(m.reply?.message_id===data.message_id&&(m.reply.chat_id||m.chat_id)===data.chat_id?{reply:{...m.reply,deleted:true}}:{})}))}
async function updateDeletionView(data){
  for(const [chat,cached] of state.messageCache){cached.messages=markDeleted(cached.messages,data)}
  const anchor=conversation.captureScrollAnchor();
  state.messages=markDeleted(state.messages,data);
  if(state.selectedChatId===data.chat_id&&!state.messages.some(m=>m.id===data.message_id)){
    try{
      const rows=await apiJson(`/api/chats/${encodeURIComponent(data.chat_id)}/messages/${encodeURIComponent(data.message_id)}/context`);
      const target=rows.find(m=>m.id===data.message_id);
      if(target&&state.selectedChatId===data.chat_id){state.messages=[...state.messages,target].sort((a,b)=>a.timestamp-b.timestamp||a.id.localeCompare(b.id))}
    }catch{} // Local target may have been explicitly purged.
  }
  if(state.view==='chat'){await conversation.renderMessages();conversation.restoreScrollAnchor(anchor)}
}
async function syncReactions(){
  const chat=state.selectedChatId;if(!chat||!state.messages.length)return;
  const deletions=await apiJson(`/api/chats/${encodeURIComponent(chat)}/deletions`);
  if(state.selectedChatId!==chat)return;
  for(const data of deletions)state.messages=markDeleted(state.messages,data);
  const ids=state.messages.map(m=>m.id);
  // Long conversations may contain several loaded pages.
  for(let i=0;i<ids.length;i+=200){
    const q=new URLSearchParams();ids.slice(i,i+200).forEach(id=>q.append('message_id',id));
    const states=await apiJson(`/api/chats/${encodeURIComponent(chat)}/reactions?${q}`);
    if(state.selectedChatId!==chat)return;
    for(const data of states){state.messages=applyReactionState(state.messages,data)}
  }
  const editIDs=[...new Set(state.messages.flatMap(m=>[m.id,...(m.reply?.message_id&&(!m.reply.chat_id||m.reply.chat_id===chat)?[m.reply.message_id]:[])]))];
  for(let i=0;i<editIDs.length;i+=200){
    const q=new URLSearchParams();editIDs.slice(i,i+200).forEach(id=>q.append('message_id',id));
    const changes=await apiJson(`/api/chats/${encodeURIComponent(chat)}/edits?${q}`);
    if(state.selectedChatId!==chat)return;
    for(const data of changes){
      state.messages=applyEditState(state.messages,data);
      const c=state.chats.find(c=>c.id===data.chat_id);
      if(c?.last_message_id===data.message_id){const latest=editStateFor(data.chat_id,data.message_id);if(latest)c.last_body=latest.body}
    }
  }
  renderChatList();
  const anchor=conversation.captureScrollAnchor();
  state.messageCache.set(chat,{messages:[...state.messages],hasOlder:state.hasOlder,at:Date.now()});
  if(state.view==='chat'){await conversation.renderMessages();conversation.restoreScrollAnchor(anchor)}
}
async function refreshSelectedMessages(){if(!state.selectedChatId)return;const data=await apiJson(`/api/chats/${encodeURIComponent(state.selectedChatId)}/messages?limit=50`);state.messages=mergeKnownReactions(data,state.reactionStates);state.messageCache.set(state.selectedChatId,{messages:data,hasOlder:state.hasOlder,at:Date.now()});await conversation.renderMessages()}
function renderShell(){
  const connected=state.connection.state==='connected',hasError=state.connection.state==='error';
  const initialSetup=connected&&state.connection.initialBootstrap===true;
  app.classList.toggle('connection-mode',!connected);
  app.classList.toggle('initial-setup-mode',initialSetup);

  if(initialSetup){
    app.innerHTML=`<div class="initial-setup" role="status" aria-live="polite">
      <div class="initial-setup-card">
        <span class="connection-spinner" aria-hidden="true"></span>
        <h2>${state.connection.syncPhase==='checking_history'?'Finishing setup…':state.connection.syncPhase==='settling_history'?'Preparing chats…':'Importing messages…'}</h2>
        <p>${state.connection.syncPhase==='checking_history'?'Resolving names and checking imported history.':state.connection.syncPhase==='settling_history'?'Waiting for the initial WhatsApp import to settle.':'Importing your initial chats, messages and contact information.'}</p>
      </div>
    </div>${state.settingsOpen?'<wisp-settings id="settings"></wisp-settings>':''}`;
    wireSettings();
    return;
  }

  const sidebarBody=connected?`<div id="searchWrap" class="search" data-key="search"><input id="search" type="search" name="wa-mirror-filter" placeholder="Search chats and messages" value="${esc(state.filter)}" autocomplete="off" autocorrect="off" spellcheck="false"></div><wisp-chat-list id="chatList" class="chat-list"></wisp-chat-list><div id="keyboardHint" class="keyboard-hint" aria-hidden="true">↑↓ Navigate · Enter Open</div>`:`<div class="connection-summary"><i class="dot ${hasError?'bad':''}"></i><span>${esc(connectionLabel())}</span></div>`;
  const mainBody=connected?`<wisp-message-list id="messages" class="messages"></wisp-message-list><button id="newMessages" class="new-messages" hidden></button><div id="incomingNotice" class="incoming-notice" hidden></div>${state.sendEnabled&&state.selectedChatId&&state.view==='chat'?`<wisp-composer id="composer"></wisp-composer>`:''}`:`<wisp-connection id="connectionView"></wisp-connection>`;
  app.innerHTML=`<aside class="sidebar"><div class="sidebar-top top"><h1>Wisp</h1><button class="icon-btn" id="settingsBtn" aria-label="Open settings" title="Settings">⚙</button></div>${sidebarBody}</aside><main class="conversation"><div class="conversation-top top"><button class="back" id="back" aria-label="Back to chats">‹</button><h2 id="conversationTitle">Messages</h2><button class="icon-btn media-btn" id="mediaBtn" aria-label="Media, links and documents" title="Media, links & documents" ${connected&&state.selectedChatId?'':'hidden'}>▦</button></div>${mainBody}</main>${state.settingsOpen?'<wisp-settings id="settings"></wisp-settings>':''}`;
  wireShell();
  navigation?.updateActivePane();
  if(connected){
    renderChatList();
    renderConversationHeader();
    if(state.selectedChatId){
      if(state.view==='chat'&&state.messages.length)void conversation.renderMessages();
      else if(state.view==='gallery')void gallery.render({loading:true}).then(()=>gallery.refresh());
    }
  }
}
function renderChatList(){navigation?.renderChatList()}
function renderConversationHeader(){const title=$('#conversationTitle'),btn=$('#mediaBtn');if(!title)return;const c=state.chats.find(c=>c.id===state.selectedChatId);title.textContent=c?displayName(c.name,c.id,c.type):'Messages';if(btn)btn.hidden=!state.selectedChatId}
async function retryMedia(messageId){
  const el=$('#messages');
  const button=[...el?.querySelectorAll('.retry-media')||[]].find(b=>b.closest('[data-message-id]')?.dataset.messageId===messageId);
  if(button){button.disabled=true;button.textContent='Retrying…'}
  const controller=new AbortController();
  const timeout=setTimeout(()=>controller.abort(),25000);
  try{
    const r=await fetch(`/api/media/message/${encodeURIComponent(messageId)}/retry`,{method:'POST',signal:controller.signal});
    if(!r.ok){const x=await r.json().catch(()=>({}));throw new Error(x.error||'Media retry failed')}
    // The backend retry is asynchronous. Refresh immediately to show pending,
    // then once more after the backend's retry timeout so pending cannot look
    // permanent even if no SSE event arrives.
    await refreshSelectedMessages();
    setTimeout(()=>{if(state.selectedChatId)refreshSelectedMessages().catch(()=>{})},21000);
  }catch(err){
    showToast(err?.name==='AbortError'?'Media retry timed out':(err?.message||'Media retry failed'));
    await refreshSelectedMessages().catch(()=>{});
  }finally{
    clearTimeout(timeout);
    if(button?.isConnected){button.disabled=false;button.textContent='Retry'}
  }
}
function focusComposer(){if(!state.sendEnabled||!state.selectedChatId||state.view!=='chat'||!window.matchMedia('(min-width: 701px)').matches||state.settingsOpen)return;state.activePane='right';state.keyboardNavActive=false;requestAnimationFrame(()=>{$('#composer')?.focusInput();navigation?.updateKeyboardModeIndicator();navigation?.updateActivePane()})}
function wireComposer(){const composer=$('#composer');if(!composer)return;composer.chatId=state.selectedChatId;composer.requestUpdate();focusComposer()}
function syncConnection(){const el=$('#connectionView');if(el){el.status=state.connection;el.requestUpdate()}}
function syncSettings(){const el=$('#settings');if(!el)return;el.status=state.connection;el.followNewMessages=state.followNewMessages;el.mirrorOnly=state.mirrorOnly;el.pushEnabled=state.pushEnabled;el.installAvailable=Boolean(state.installPrompt);el.standalone=state.isStandalone;el.requestUpdate()}
function renderSettings(){const old=$('#settings');if(!state.settingsOpen){old?.remove();focusComposer();return}if(!old){const el=document.createElement('wisp-settings');el.id='settings';app.appendChild(el);wireSettings(el)}syncSettings()}
function openSettings(){settingsReturnFocus=document.activeElement;state.settingsOpen=true;renderSettings();requestAnimationFrame(()=>$('#settings')?.querySelector('.modal-head .icon-btn')?.focus())}
function removeSettingsKeyHandler(){if(settingsKeyHandler){document.removeEventListener('keydown',settingsKeyHandler);settingsKeyHandler=null}}
function closeSettings(){removeSettingsKeyHandler();state.settingsOpen=false;renderSettings();requestAnimationFrame(()=>{if(settingsReturnFocus?.isConnected)settingsReturnFocus.focus();else $('#settingsBtn')?.focus();settingsReturnFocus=null})}
function reconnectWhatsApp(){state.connection={...state.connection,state:'reconnecting',qr:null,error:null};renderShell();fetch('/api/whatsapp/reconnect',{method:'POST'}).catch(()=>{state.connection={...state.connection,state:'error',error:'Reconnect request failed.'};renderShell()})}
function wireShell(){conversation?.wireMessageList($('#messages'));navigation?.wireShell();$('#back')?.addEventListener('click',()=>app.classList.remove('open'));$('#settingsBtn')?.addEventListener('click',openSettings);$('#errorBanner')?.addEventListener('click',openSettings);$('#newMessages')?.addEventListener('click',()=>conversation.scrollBottom());$('#mediaBtn')?.addEventListener('click',()=>gallery.open('media'));const connection=$('#connectionView');if(connection){connection.status=state.connection;connection.addEventListener('connection-reconnect',reconnectWhatsApp)}const settings=$('#settings');if(settings){wireSettings(settings);syncSettings()}wireComposer()}
function wireSettings(el=$('#settings')){if(!state.settingsOpen||!el||el.dataset.wired)return;el.dataset.wired='true';removeSettingsKeyHandler();settingsKeyHandler=e=>{if(e.key==='Escape'){e.preventDefault();closeSettings()}};document.addEventListener('keydown',settingsKeyHandler);el.addEventListener('settings-close',closeSettings);el.addEventListener('follow-change',e=>{saveConfig({followNewMessages:e.detail}).catch(()=>showToast('Could not save setting.'))});el.addEventListener('settings-reconnect',()=>{removeSettingsKeyHandler();state.settingsOpen=false;reconnectWhatsApp()});el.addEventListener('settings-unlink',async()=>{removeSettingsKeyHandler();state.settingsOpen=false;state.selectedChatId=null;state.connection={...state.connection,state:'disconnecting',qr:null,error:null};renderShell();try{const r=await fetch('/api/whatsapp/logout',{method:'POST'});if(!r.ok)throw new Error('Unlink failed')}catch{state.connection={...state.connection,state:'error',error:'Could not unlink WhatsApp.'};renderShell()}});el.addEventListener('push-toggle',()=>pwa.togglePush().catch(err=>showToast(err?.message||'Could not update push notifications.')));el.addEventListener('install-app',()=>pwa.installApp())}

function showIncomingNotice(m){
  const el=$('#incomingNotice');if(!el)return;
  const c=state.chats.find(c=>c.id===m.chat_id),name=displayName(c?.name,m.chat_id,c?.type),preview=m.body||mediaLabel(m.type);
  el.innerHTML=`<div><b>${esc(name)}</b><span>${esc(preview.slice(0,120))}</span></div><button>Show</button>`;
  el.hidden=false;el.querySelector('button').onclick=()=>{el.hidden=true;selectChat(m.chat_id)};
  clearTimeout(noticeTimer);noticeTimer=setTimeout(()=>{if(el)el.hidden=true},8000);
}

async function handleLaunchTarget(){const q=new URLSearchParams(location.search),chat=q.get('chat'),message=q.get('message');if(!chat)return;history.replaceState(null,'',location.pathname);if(message)await openMessage(chat,message);else await selectChat(chat)}
conversation=createConversation({app,store,apiJson,onRetryMedia:retryMedia,onReply:selectReply,onReaction:setReaction,onQuote:openQuote});
gallery=createGallery({app,store,apiJson,openMessage});
navigation=createNavigation({app,store,selectChat,openMessage,focusComposer});
navigation.installGlobalListeners();
pwa=createPwaService({store,apiJson,onStateChange:()=>{if(state.settingsOpen)renderSettings()},onError:message=>showToast(message)});
pwa.start();
const bus=createEventBus();
connectLiveEvents(bus);
bus.on('live:open',()=>{loadChats({render:false}).then(()=>{renderChatList();renderConversationHeader()}).catch(()=>{});syncReactions().catch(()=>{})});
bus.on('live:edit',data=>updateEditView(data).catch(console.error));
bus.on('live:deletion',data=>updateDeletionView(data).catch(console.error));
bus.on('live:reaction',data=>updateReactionView(data).catch(console.error));
document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible')syncReactions().catch(()=>{})});
bus.on('live:connection',next=>{state.connection=next;renderShell()});
bus.on('live:message',async m=>{m=mergeKnownReactions([m],state.reactionStates)[0];const atBottom=conversation.isNearBottom(),c=state.chats.find(x=>x.id===m.chat_id);if(c){c.last_message_id=m.id;c.last_message_at=m.timestamp;c.last_body=m.body;c.last_type=m.type;c.last_from_me=m.from_me;c.last_sender_name=m.sender_name;state.chats.sort((a,b)=>(b.last_message_at||0)-(a.last_message_at||0));renderChatList();renderConversationHeader()}else{loadChats({render:false}).then(()=>{renderChatList();renderConversationHeader()}).catch(()=>{})}if(state.selectedChatId===m.chat_id){if(!state.messages.some(x=>x.id===m.id)){state.messages.push(m);state.messageCache.set(m.chat_id,{messages:[...state.messages],hasOlder:state.hasOlder,at:Date.now()});if(state.view==='chat'){await conversation.renderMessages();if(atBottom)conversation.scrollBottom();else{state.newWhileReading++;conversation.updateNewButton()}}if(earlyMediaEvents.delete(m.id)){refreshSelectedMessages().catch(()=>{})}}return}if(m.from_me)return;state.unread.set(m.chat_id,(state.unread.get(m.chat_id)||0)+1);renderChatList();if(state.mirrorOnly&&state.followNewMessages){selectChat(m.chat_id)}else{showIncomingNotice(m)}});
bus.on('live:media',async d=>{
  if(state.selectedChatId!==d.chatId)return;
  if(state.view==='gallery'){gallery.refresh();return}
  if(state.view!=='chat')return;

  const messageId=d.messageId||d.media?.MessageID||d.media?.message_id;
  const index=state.messages.findIndex(m=>m.id===messageId);
  if(index<0){
    if(messageId){
      earlyMediaEvents.set(messageId,d);
      setTimeout(()=>earlyMediaEvents.delete(messageId),30000);
    }
    return;
  }

  // The backend has already persisted the final media row before emitting this
  // event. Update only the affected message instead of reloading the newest 50
  // messages: the media may belong to an older page currently visible after
  // scrolling, and a latest-page reload would never update that object.
  const media=d.media||{};
  const current=state.messages[index];
  state.messages[index]={
    ...current,
    media_id: media.ID||media.id||current.media_id||messageId,
    media_status: media.Status||media.status||d.status||'ready',
    mime_type: media.MimeType?.String||media.mime_type||current.mime_type,
    filename: media.Filename?.String||media.filename||current.filename
  };

  const anchor=conversation.captureScrollAnchor();
  const atBottom=conversation.isNearBottom();
  await conversation.renderMessages();
  if(atBottom&&state.scrollMode!=='prepend')conversation.scrollBottom();
  else conversation.restoreScrollAnchor(anchor);
});
bus.on('live:metadata',()=>loadChats());
bus.on('live:parse-error',({type,error})=>console.warn(`Invalid SSE ${type} payload`,error));
async function init(){
  void pwa.refreshPushState();
  try{
    await Promise.all([loadStatus({render:false}),loadConfig(),loadChats({render:false})]);
    renderShell();
    await handleLaunchTarget();
  }catch(err){
    console.error('Wisp initialization failed',err);
    renderShell();
  }
}
app.addEventListener('viewport-layout', () => conversation?.updateNewButton());
await init();
}
