import {loadUnreadOpening} from './services/chat-opening.js';
import {createReadTracker,unreadCount,closeReadNotifications,unreadTabTitle} from './services/unread.js';
import './components/wisp-chat-picker.js';
import './components/wisp-chat-controls.js';
import {sessionOutbox} from './services/outbox.js';
import { sessionDrafts } from './core/drafts.js';
import { chatMuted, muteLabel } from './core/chat-state.js';
import { createViewHistory } from './services/view-history.js';
import { attachKeyboardViewport } from './services/keyboard-viewport.js';
import { applyEditState, editStateFor } from './core/edits.js';
import { rememberDeletion } from './core/deletions.js';
import './components/wisp-chat-list.js';
import './components/wisp-message-list.js';
import './components/wisp-composer.js';
import './components/wisp-connection.js';
import './components/wisp-settings.js';
import './components/wisp-gallery.js';
import { $, displayName } from './lib/utils.js';
import { apiJson } from './lib/api.js';
import { createEventBus } from './core/event-bus.js';
import { connectLiveEvents } from './services/live-events.js';
import { applyReactionState, mergeKnownReactions } from './core/reactions.js';
import { createAppStore, createInitialAppState } from './core/app-store.js';
import { createNavigation } from './services/navigation.js';
import { createConversation } from './services/conversation.js';
import { createGallery } from './services/gallery.js';
import { createPwaService } from './services/pwa.js';
import { LitElement, html, render } from 'lit';
import { live } from 'lit/directives/live.js';
import { icon } from './lib/icons.js';
import { installSidebarResize } from './services/sidebar-resize.js';

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
let toastTimer=null,settingsReturnFocus=null,settingsKeyHandler=null;
const earlyMediaEvents=new Map();
let imageScrollIntent=null;
function showToast(message,kind='error'){let el=$('#appToast');if(!el){el=document.createElement('div');el.id='appToast';app.appendChild(el)}el.className=`app-toast ${kind}`;el.textContent=message;el.hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>{el.hidden=true},4500)}
let navigation,conversation,gallery,pwa,viewHistory;
const readTracker=createReadTracker({getState:()=>state,apiJson,onState:applyChatState,onRead:closeReadNotifications,
 canRead:()=>state.connection.state==='connected'&&document.visibilityState==='visible'&&Boolean(state.selectedChatId)&&state.view==='chat'&&!state.chatPreparing&&!state.messageLoading&&!state.chatLoadError&&!state.imageViewer&&!state.settingsOpen&&!state.chatPickerOpen&&app.classList.contains('open')&&!app.classList.contains('initial-setup-mode')&&[...app.querySelectorAll('#messages [data-message-id]')].some(el=>el.dataset.messageId===state.chatStates.get(state.selectedChatId)?.last_incoming_id)
});
const markVisibleChatRead=()=>void readTracker.markVisible();

let imageReturnFocus=null;
let suspendedAnchor=null;
const chatPositions=new Map();
function rememberPosition(){
  if(!state.chatPreparing&&state.selectedChatId&&state.view==='chat')chatPositions.set(state.selectedChatId,{anchor:conversation?.captureScrollAnchor(),bottom:conversation?.isNearBottom()});
}
function backToSelection(fromHistory=false){
  if(!fromHistory&&state.imageViewer){closeImageViewer();return}
  if(!fromHistory&&viewHistory?.toOverview())return;
  dismissImageViewer();imageScrollIntent=null;
  const previous=state.selectedChatId;
  rememberPosition();++chatLoadToken;cancelChatPreparation();gallery.cancel();
  $('#messages')?.closePanel(false);$('#composer')?.closePicker();
  if(state.settingsOpen)closeSettings();
  navigation.clearSearch();conversation.resetForChat();
  state.selectedChatId=null;state.view='chat';state.activePane='left';
  state.keyboardNavActive=window.matchMedia('(min-width: 701px)').matches;
  state.keyboardSelection=previous?`chat:${previous}`:null;
  app.classList.remove('open');renderShell();
  requestAnimationFrame(()=>{if(state.selectedChatId||app.classList.contains('open'))return;const chat=[...app.querySelectorAll('#chatList .chat')].find(el=>el.dataset.id===previous);(chat||$('#settingsBtn'))?.focus({preventScroll:true})});
}

function returnFromGallery(){
  gallery.cancel();state.view='chat';renderShell();
  const chatId=state.selectedChatId,saved=chatPositions.get(chatId);
  void $('#messages')?.updateComplete.then(()=>{
    if(state.view!=='chat'||state.imageViewer||state.selectedChatId!==chatId||state.chatPreparing)return;
    if(saved?.bottom)conversation.scrollBottom({initial:true});else if(saved?.anchor)conversation.restoreScrollAnchor(saved.anchor);
    $('#mediaBtn')?.focus({preventScroll:true});markVisibleChatRead();
  });
}

function connectionLabel(){if(state.connection.state==='connected'&&state.connection.syncing)return state.connection.syncPhase==='importing_history'?'Syncing chats…':state.connection.syncPhase==='settling_history'?'Finishing sync…':state.connection.syncPhase==='checking_history'?'Checking history…':'Syncing…';return({starting:'Starting…',disconnecting:'Disconnecting…',reconnecting:'Reconnecting…',waiting_for_qr:'Waiting for QR scan',authenticating:'Signing in…',connected:'Connected',disconnected:'Disconnected',error:'Connection error'}[state.connection.state]||state.connection.state)}
async function loadStatus({render=true}={}){const x=await apiJson('/api/status');state.version=x.version||'dev';state.update=x.update;state.pushStatus=x.push||{};state.connection=x.whatsapp;state.sendEnabled=Boolean(x.send?.enabled);if(Array.isArray(x.chat_states))for(const p of x.chat_states)applyChatState(p);scheduleMuteRefresh();if(render)renderShell();markVisibleChatRead()}
async function loadConfig(){const x=await apiJson('/api/config');state.sendEnabled=Boolean(x.sendEnabled);state.mirrorOnly=Boolean(x.mirrorOnly);state.followNewMessages=Boolean(x.followNewMessages);state.readMode=x.readMode||'wisp';}
async function saveConfig(patch){const x=await apiJson('/api/config',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(patch)});state.sendEnabled=Boolean(x.sendEnabled);state.mirrorOnly=Boolean(x.mirrorOnly);state.followNewMessages=Boolean(x.followNewMessages);state.readMode=x.readMode||'wisp';syncSettings()}async function loadChats({render=true}={}){const chats=await apiJson('/api/chats');state.chats=Array.isArray(chats)?chats:[];for(const c of state.chats)state.knownChats.set(c.id,c);scheduleMuteRefresh();if(render)renderChatList();renderConversationHeader()}
let muteRefreshTimer;
function scheduleMuteRefresh(){
 clearTimeout(muteRefreshTimer);
 const values=[...state.chatStates.values(),...state.chats].map(p=>p.mute_until*1000-Date.now()).filter(ms=>ms>0);
 if(values.length)muteRefreshTimer=setTimeout(()=>{renderChatList();renderConversationHeader();$('#chatControls')?.requestUpdate();$('#chatPicker')?.requestUpdate();scheduleMuteRefresh()},Math.min(...values,2147483647)+20);
}
function applyChatState(p){
 if(!p?.chat_id||!Number.isSafeInteger(p.version))return;
 const previous=state.chatStates.get(p.chat_id);
 if(previous&&previous.version>=p.version)return;
 state.chatStates.set(p.chat_id,p);
 if(Number.isSafeInteger(p.unread_sequence)){
  const count=unreadCount(p);if(count)state.unread.set(p.chat_id,count);else state.unread.delete(p.chat_id);
 }
 const cleared=sessionDrafts.invalidate(p.chat_id,p.draft_revision);sessionOutbox.invalidate(p.chat_id,p.draft_revision);
 if(p.closed||(cleared&&!Number.isSafeInteger(p.unread_sequence)))state.unread.delete(p.chat_id);
 if(p.closed&&state.searchResults){state.searchResults={chats:state.searchResults.chats.filter(c=>c.id!==p.chat_id),messages:state.searchResults.messages.filter(m=>m.chat_id!==p.chat_id)}}
 $('#chatPicker')?.requestUpdate();
 const controls=$('#chatControls');if(controls?.context.id===p.chat_id)controls.context={...controls.context,state:p};
 scheduleMuteRefresh();renderChatList();renderConversationHeader();
}
function showChatControls(mode){
 if(mode==='new'){showChatPicker();return}
 if(state.chatPickerOpen){hideChatPicker();viewHistory.replaceOverlay(mode)}
 if(state.connection.state!=='connected')return;
 const id=state.selectedChatId,c=state.chats.find(c=>c.id===id)||state.knownChats.get(id);
 $('#messages')?.closePanel(false);$('#composer')?.closePicker();
 viewHistory.openOverlay(mode);
 void $('#chatControls').show(mode,{id,name:c?displayName(c.name,id,c.type):id,state:state.chatStates.get(id)||c||{}});
}
let pickerReturnFocus=null,pickerPreviousPane='left';
function showChatPicker(){
 if(state.connection.state!=='connected'||state.chatPickerOpen)return;
 pickerReturnFocus=$('#newChatBtn');pickerPreviousPane=state.activePane;
 document.activeElement?.blur();$('#messages')?.closePanel(false);$('#composer')?.closePicker();
 viewHistory.openOverlay('new');state.chatPickerOpen=true;state.keyboardNavActive=false;state.activePane='left';
 renderShell({refreshMessages:false});void $('#chatPicker').open();
}
function hideChatPicker(){
 if(!state.chatPickerOpen)return;
 state.chatPickerOpen=false;$('#chatPicker')?.hide();state.activePane=pickerPreviousPane;
 renderShell({refreshMessages:false});
 if(pickerReturnFocus?.isConnected)pickerReturnFocus.focus({preventScroll:true});
}
async function closeChatControls(){
 hideChatPicker();
 $('#chatControls')?.hide();
 if(viewHistory.current().overlay){await new Promise(resolve=>{window.addEventListener('popstate',resolve,{once:true});viewHistory.closeOverlay()})}
}
async function completeChatAction({action,state:next,option,pickerActive}){
 applyChatState(next);
 if(option)state.knownChats.set(option.id,option);
 if(pickerActive===false){await loadChats();return}
 await closeChatControls();
 if(action==='close'){dismissImageViewer();if(state.selectedChatId===next.chat_id)backToSelection();await loadChats();return}
 await loadChats();
 if(action==='open'){navigation.clearSearch();await selectChat(next.chat_id)}
}
let chatLoadToken=0;
const MESSAGE_CACHE_TTL=2*60*1000;
const nextLayoutFrame=()=>new Promise(resolve=>requestAnimationFrame(resolve));
let chatPreparation=null;
function cancelChatPreparation(){chatPreparation=null;state.chatPreparing=false;state.chatLoadError='';app.classList.remove('chat-preparing','chat-revealing')}
function beginChatPreparation(token){
  app.classList.remove('chat-revealing');
  $('#messages')?.closePanel(false);$('#composer')?.closePicker();
  state.chatPreparing=true;state.chatLoadError='';state.activePane='right';state.keyboardNavActive=false;
  const context={token,shownAt:nextLayoutFrame().then(()=>performance.now())};
  chatPreparation=context;
}
const isCurrentChat=token=>token===chatLoadToken&&chatPreparation?.token===token;
async function waitForLoadingMinimum(token){
  const context=chatPreparation;if(context?.token!==token)return false;
  const shownAt=await context.shownAt;
  const remaining=150-(performance.now()-shownAt);
  if(remaining>0)await new Promise(resolve=>setTimeout(resolve,remaining));
  return isCurrentChat(token);
}
async function revealPreparedChat(token,position){
  await conversation.renderMessages({loading:false});
  await $('#composer')?.updateComplete;
  if(!await waitForLoadingMinimum(token))return false;
  // Keep normal layout dimensions while hidden. Position both after composer
  // sizing and once more in the next frame, then fade in the prepared surface.
  for(let i=0;i<2;i++){
    await nextLayoutFrame();if(!isCurrentChat(token))return false;
    conversation.positionMessages(position);
  }
  if(!isCurrentChat(token))return false;
  state.chatPreparing=false;chatPreparation=null;
  renderShell({refreshMessages:false});conversation.updateNewButton();
  app.classList.toggle('chat-revealing',!window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  markVisibleChatRead();
  return true;
}
async function failChatPreparation(token,error){
  if(!await waitForLoadingMinimum(token))return;
  state.messageLoading=false;state.chatLoadError=error.message||'Could not load messages.';
  renderShell({refreshMessages:false});
}
async function selectChat(id,{restore=false}={}){if(state.imageViewer)dismissImageViewer();viewHistory?.openChat(id,{preserveImage:restore});imageScrollIntent=null;
  if(id===state.selectedChatId&&state.view==='chat'){
    if(state.chatPreparing&&!state.chatLoadError)return;
    if(!state.chatPreparing&&!unreadCount(state.chatStates.get(id))){app.classList.add('open');markVisibleChatRead();renderChatList();renderConversationHeader();focusComposer();return}
  }
  rememberPosition();
  const token=++chatLoadToken;
  state.selectedChatId=id;if(!Number.isSafeInteger(state.chatStates.get(id)?.unread_sequence))state.unread.delete(id);state.view='chat';state.keyboardNavActive=false;
  conversation.resetForChat();app.classList.add('open');beginChatPreparation(token);
  const pref=state.chatStates.get(id);
  const unreadTarget=unreadCount(pref)>0?(pref.first_unread_id||pref.last_incoming_id):null;
  const cached=state.messageCache.get(id),hasCached=Boolean(cached)&&!unreadTarget,saved=chatPositions.get(id);
  let openingTarget=unreadTarget;
  if(hasCached){state.messages=cached.messages;state.hasOlder=cached.hasOlder;state.messageLoading=false}
  else state.messageLoading=true;
  renderShell({refreshMessages:false});
  // Fetch concurrently with cached preparation. Convert rejection immediately so
  // even a fast failure cannot become an unhandled promise during the 150ms hold.
  const response=(unreadTarget
    ?loadUnreadOpening({chatId:id,targetId:unreadTarget,apiJson,isCurrent:()=>token===chatLoadToken&&state.selectedChatId===id})
      .then(result=>{openingTarget=result?.targetId;return result?.messages||[]})
    :apiJson(`/api/chats/${encodeURIComponent(id)}/messages?limit=50`))
    .then(data=>({data}),error=>({error}));
  if(hasCached){
    if(!await revealPreparedChat(token,{anchor:saved?.anchor,bottom:!saved||saved.bottom}))return;
    if(token!==chatLoadToken)return;if(state.activePane==='right'&&!state.keyboardNavActive)focusComposer();
  }
  const result=await response;
  if(token!==chatLoadToken||state.selectedChatId!==id)return;
  if(result.error){
    if(hasCached)showToast('Could not refresh messages. Showing stored messages.','info');
    else await failChatPreparation(token,result.error);
    return;
  }
  const data=result.data;
  const atBottom=conversation.isNearBottom(),anchor=conversation.captureScrollAnchor();
  const hasOlder=hasCached?state.hasOlder:data.length>0;
  const merged=[...new Map([...state.messages,...data].map(m=>[m.id,m])).values()]
    .sort((a,b)=>a.timestamp-b.timestamp||a.id.localeCompare(b.id));
  state.messages=mergeKnownReactions(merged,state.reactionStates);state.hasOlder=hasOlder;state.messageLoading=false;
  state.messageCache.set(id,{messages:state.messages,hasOlder,at:Date.now()});
  for(const [chat,entry] of state.messageCache){if(Date.now()-entry.at>MESSAGE_CACHE_TTL)state.messageCache.delete(chat)}
  while(state.messageCache.size>8)state.messageCache.delete(state.messageCache.keys().next().value);
  if(hasCached){
    await conversation.renderMessages({loading:false});
    if(token!==chatLoadToken||state.selectedChatId!==id)return;
    if(atBottom)conversation.scrollBottom();else conversation.positionMessages({anchor});markVisibleChatRead();
  }else if(await revealPreparedChat(token,openingTarget?{messageId:openingTarget}:unreadTarget?{bottom:true}:{anchor:saved?.anchor,bottom:!saved||saved.bottom})){
    if(openingTarget&&token===chatLoadToken)await conversation.highlightMessage(openingTarget);
    if(state.activePane==='right'&&!state.keyboardNavActive)focusComposer();
  }
}

async function openMessage(chatId,messageId,loaded=null){if(state.imageViewer)dismissImageViewer();viewHistory?.openChat(chatId);imageScrollIntent=null;
  rememberPosition();
  const token=++chatLoadToken;
  state.selectedChatId=chatId;state.view='chat';if(!Number.isSafeInteger(state.chatStates.get(chatId)?.unread_sequence))state.unread.delete(chatId);conversation.resetForChat();app.classList.add('open');beginChatPreparation(token);
  state.messageLoading=true;renderShell({refreshMessages:false});
  try{
    const data=loaded||await apiJson(`/api/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/context`);
    if(token!==chatLoadToken||state.selectedChatId!==chatId)return;
    state.messages=mergeKnownReactions(data,state.reactionStates);state.hasOlder=true;state.messageLoading=false;
    if(await revealPreparedChat(token,{messageId})){
      await conversation.highlightMessage(messageId);if(state.activePane==='right'&&!state.keyboardNavActive)focusComposer();
    }
  }catch(error){if(token===chatLoadToken)await failChatPreparation(token,error)}
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
async function refreshSelectedMessages(){
 const id=state.selectedChatId,token=chatLoadToken;if(!id)return;
 const data=await apiJson(`/api/chats/${encodeURIComponent(id)}/messages?limit=50`);
 if(state.selectedChatId!==id||token!==chatLoadToken||state.chatPreparing||state.view!=='chat')return;
 const anchor=conversation.captureScrollAnchor(),bottom=conversation.isNearBottom();
 state.messages=mergeKnownReactions([...new Map([...state.messages,...data].map(m=>[m.id,m])).values()].sort((a,b)=>a.timestamp-b.timestamp||a.id.localeCompare(b.id)),state.reactionStates);
 state.messageCache.set(id,{messages:state.messages,hasOlder:state.hasOlder,at:Date.now()});
 await conversation.renderMessages();if(state.selectedChatId!==id||token!==chatLoadToken)return;
 if(bottom)conversation.scrollBottom();else conversation.restoreScrollAnchor(anchor);
 markVisibleChatRead();
}
function renderShell({refreshMessages=true}={}){
  const connected=state.connection.state==='connected',initialSetup=connected&&state.connection.initialBootstrap===true;
  const paused=!connected||initialSetup;
  const oldMessages=$('#messages');
  if(paused&&oldMessages&&!oldMessages.hidden&&!suspendedAnchor){suspendedAnchor=conversation?.captureScrollAnchor();conversation?.clearInitialTimer()}
  app.classList.toggle('chat-preparing',state.chatPreparing);
  app.classList.toggle('picker-open',state.chatPickerOpen&&!paused);
  app.classList.toggle('connection-mode',!connected);
  app.classList.toggle('initial-setup-mode',initialSetup);
  render(html`
    <div class="initial-setup" ?hidden=${!initialSetup} role="status" aria-live="polite"><div class="initial-setup-card"><span class="connection-spinner" aria-hidden="true"></span><h2>${state.connection.syncPhase==='checking_history'?'Finishing setup…':state.connection.syncPhase==='settling_history'?'Preparing chats…':'Importing messages…'}</h2><p>Preparing your chats, messages and contact information.</p></div></div>
    <aside class="sidebar" ?hidden=${initialSetup}>
      <div class="sidebar-top top" ?hidden=${state.chatPickerOpen&&!paused}><h1><a class="wisp-brand" href="https://github.com/desirevolution/wisp" target="_blank" rel="noopener noreferrer" aria-label="Wisp project on GitHub (opens in a new tab)" title="Wisp on GitHub"><img src="/icons/icon.svg" alt="" width="32" height="32"><span>Wisp</span></a></h1><button class="icon-btn" id="newChatBtn" aria-label="New chat" title="New chat" ?hidden=${paused} @pointerdown=${e=>{if(document.activeElement?.matches('textarea'))e.preventDefault()}} @click=${()=>showChatControls('new')}>${icon('newChat')}</button><button class="icon-btn" id="settingsBtn" aria-label="Open settings" title="Settings">${icon('settings')}</button></div>
      <div id="searchWrap" class="search" data-key="search" ?hidden=${paused||state.chatPickerOpen}><input id="search" type="search" aria-label="Search chats and messages" placeholder="Search chats and messages" .value=${live(state.filter)} autocomplete="off" autocorrect="off" spellcheck="false"></div>
      <wisp-chat-list id="chatList" class="chat-list" ?hidden=${paused||state.chatPickerOpen}></wisp-chat-list>
      <wisp-chat-picker id="chatPicker" class="chat-picker" .blocked=${paused} .chatStates=${state.chatStates} ?hidden=${paused||!state.chatPickerOpen} @chat-picker-close=${()=>void closeChatControls()} @chat-action-complete=${e=>void completeChatAction(e.detail).catch(()=>showToast('Chat was opened, but the view could not refresh. Try again.'))}></wisp-chat-picker>
      <div class="connection-summary" ?hidden=${connected}><i class="dot"></i><span>${connectionLabel()}</span></div>
      <div id="keyboardHint" class="keyboard-hint" aria-hidden="true" hidden>↑↓ Navigate · Enter Open</div>
    </aside>
    <div id="sidebarResize" class="sidebar-resize" role="separator" tabindex="0" aria-label="Chat list width" aria-orientation="vertical" title="Resize chat list (arrow keys)" ?hidden=${paused}></div>
    <main class=${state.imageViewer?'conversation image-open':'conversation'} ?hidden=${initialSetup}>
      <div class="conversation-top top"><button class="back" id="back" aria-label=${state.imageViewer||state.view==='gallery'?'Back to chat':'Back to chats'} title=${state.imageViewer||state.view==='gallery'?'Back to chat':'Back to chats'} ?hidden=${paused||!state.selectedChatId}>${icon('back')}</button><div class="conversation-heading"><h2 id="conversationTitle">Messages</h2><span id="headerMuted" class="chat-muted" hidden>${icon('mute')}</span></div><button class="icon-btn media-btn" id="mediaBtn" aria-label="Media, links and documents" title="Media, links and documents" ?disabled=${state.chatPreparing} ?hidden=${paused||!state.selectedChatId}>${icon('grid')}</button><button id="chatMenuBtn" class="icon-btn" aria-haspopup="dialog" aria-expanded="false" aria-label="Chat menu" title="Chat menu" ?hidden=${paused||!state.selectedChatId} @pointerdown=${e=>{if(document.activeElement?.matches('textarea'))e.preventDefault()}} @click=${()=>$('#chatControls')?.mode?void closeChatControls():showChatControls('menu')}>${icon('more')}</button></div>
      <wisp-message-list id="messages" class="messages" .inert=${state.chatPreparing||Boolean(state.imageViewer)} aria-busy=${state.chatPreparing?'true':'false'} ?hidden=${paused||state.view!=='chat'}></wisp-message-list>
      <div id="chatLoading" class="chat-loading" role="status" aria-live="polite" ?hidden=${!state.chatPreparing||paused||state.view!=='chat'}>${state.chatLoadError?html`<div><b>Could not load messages</b><p>${state.chatLoadError}</p><button @click=${()=>selectChat(state.selectedChatId)}>Try again</button></div>`:html`<span>Loading messages…</span>`}</div>
      <wisp-gallery id="gallery" class="messages" ?hidden=${paused||state.view!=='gallery'}></wisp-gallery>
      <button id="newMessages" class="new-messages" hidden></button>
      <wisp-composer id="composer" .inert=${state.chatPreparing||Boolean(state.imageViewer)} ?hidden=${paused||!state.sendEnabled||state.mirrorOnly||!state.selectedChatId||state.view!=='chat'}></wisp-composer>
      <section id="imageViewer" class="image-viewer" role="region" aria-label="Image viewer" ?hidden=${paused||!state.imageViewer}>
        ${state.imageViewer?html`<div class="image-viewer-stage"><img src=${state.imageViewer.src} alt=${state.imageViewer.alt||'Image'}></div><p>${state.imageViewer.alt||''}</p>`:null}
      </section>
      <wisp-connection id="connectionView" ?hidden=${connected}></wisp-connection>
    </main>
    <div id="updateNotice" class="update-notice" role="status" ?hidden=${!state.update?.available}>Update ${state.update?.latest} available <a href=${state.update?.url||'https://github.com/desirevolution/wisp/releases'} target="_blank" rel="noopener noreferrer">View release</a></div>
    <wisp-chat-controls id="chatControls" @chat-controls-close=${()=>void closeChatControls()} @chat-action-complete=${e=>void completeChatAction(e.detail).catch(()=>showToast('Chat settings were saved, but the view could not refresh. Try again.'))}></wisp-chat-controls>
    <wisp-settings id="settings" ?hidden=${!state.settingsOpen}></wisp-settings>
    <div id="appToast" role="status" aria-live="polite" hidden></div>
  `,app);
  wireShell();navigation?.updateActivePane();
  syncConnection();syncSettings();if(paused&&$('#newMessages'))$('#newMessages').hidden=true;
  if(connected&&!initialSetup){
    renderChatList();renderConversationHeader();
    if(refreshMessages&&state.view==='chat')void conversation.renderMessages().then(()=>{if(suspendedAnchor){state.scrollMode='idle';conversation.restoreScrollAnchor(suspendedAnchor);suspendedAnchor=null}});

  }
}
function renderChatList(){document.title=unreadTabTitle(state);navigation?.renderChatList()}
function renderConversationHeader(){const title=$('#conversationTitle'),btn=$('#mediaBtn');if(!title)return;const c=state.chats.find(c=>c.id===state.selectedChatId)||state.knownChats.get(state.selectedChatId);const pref=state.chatStates.get(state.selectedChatId)||c;const badge=$('#headerMuted');if(badge){badge.hidden=!chatMuted(pref);badge.title=muteLabel(pref);badge.setAttribute('aria-label',muteLabel(pref))}title.textContent=(c?displayName(c.name,c.id,c.type):'Messages')+(state.imageViewer?' · Image':'');if(btn)btn.hidden=!state.selectedChatId||Boolean(state.imageViewer)}
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
function renderUpdateNotice(){renderShell({refreshMessages:false})}
async function followSentImage({chatId,messageId}){
  if(state.selectedChatId!==chatId||state.view!=='chat'||state.chatPreparing)return;
  const intent={chatId,messageId};imageScrollIntent=intent;
  await $('#composer')?.updateComplete;
  if(imageScrollIntent!==intent||state.selectedChatId!==chatId)return;
  conversation.scrollBottom({initial:true,force:true});
  // Acknowledgement can arrive before SSE (or after an SSE interruption).
  // Merge the local message context without replacing already loaded history.
  if(messageId&&!state.messages.some(m=>m.id===messageId)){
    try{
      const rows=await apiJson(`/api/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/context`);
      if(imageScrollIntent!==intent||state.selectedChatId!==chatId||state.view!=='chat')return;
      state.messages=mergeKnownReactions([...new Map([...state.messages,...rows].map(m=>[m.id,m])).values()].sort((a,b)=>a.timestamp-b.timestamp||a.id.localeCompare(b.id)),state.reactionStates);
      state.messageCache.set(chatId,{messages:[...state.messages],hasOlder:state.hasOlder,at:Date.now()});
      await conversation.renderMessages();
      if(state.selectedChatId===chatId)conversation.scrollBottom({initial:true,force:true});
    }catch{/* The live event may still arrive; an acknowledged send is never retried. */}
  }
  if(state.messages.some(m=>m.id===messageId)&&imageScrollIntent===intent)imageScrollIntent=null;
}
function dismissImageViewer(){
  const wasOpen=Boolean(state.imageViewer);state.imageViewer=null;
  if(wasOpen){renderShell({refreshMessages:false});markVisibleChatRead()}
  if(imageReturnFocus?.isConnected)imageReturnFocus.focus({preventScroll:true});
  imageReturnFocus=null;
}
function closeImageViewer(){if(!viewHistory?.closeImage())dismissImageViewer()}
function openImageViewer(image,{history=true}={}){
  if(!state.selectedChatId)return;
  if(!state.imageViewer)imageReturnFocus=document.activeElement;
  $('#messages')?.closePanel(false);
  if(history)viewHistory.openImage(image,state.selectedChatId);
  state.imageViewer=image;renderShell({refreshMessages:false});
  $('#back')?.focus({preventScroll:true});
}
function focusComposer(){if(state.chatPreparing||!state.sendEnabled||!state.selectedChatId||state.view!=='chat'||!window.matchMedia('(min-width: 701px)').matches||state.settingsOpen)return;state.activePane='right';state.keyboardNavActive=false;requestAnimationFrame(()=>{if(state.view!=='chat'||state.chatPreparing||state.chatPickerOpen||state.settingsOpen||state.imageViewer||$('#chatControls')?.mode||document.querySelector('dialog[open]'))return;$('#composer')?.focusInput();navigation?.updateKeyboardModeIndicator();navigation?.updateActivePane()})}
function wireComposer(){const composer=$('#composer');if(!composer)return;composer.chatId=state.selectedChatId;composer.blocked=state.chatPreparing;composer.requestUpdate()}
function syncConnection(){const el=$('#connectionView');if(el){el.status=state.connection;el.requestUpdate()}}
function syncSettings(){const el=$('#settings');if(!el)return;el.version=state.version;el.releaseUpdate=state.update;el.status=state.connection;el.followNewMessages=state.followNewMessages;el.readMode=state.readMode;el.mirrorOnly=state.mirrorOnly;el.pushEnabled=state.pushEnabled;el.pushTestEnabled=Boolean(state.pushStatus?.test_enabled);el.pushSubscriptions=state.pushStatus?.subscriptions||0;el.installAvailable=Boolean(state.installPrompt);el.standalone=state.isStandalone;el.requestUpdate()}
function renderSettings(){const el=$('#settings');if(!el)return;el.hidden=!state.settingsOpen;for(const node of app.querySelectorAll('.sidebar,.conversation,.sidebar-resize'))node.inert=state.settingsOpen;wireSettings(el);syncSettings()}
function openSettings(){if(state.settingsOpen)return;void loadConfig().then(syncSettings).catch(()=>{});void reconcileUnread().then(()=>{syncSettings();renderUpdateNotice()}).catch(()=>{});settingsReturnFocus=document.activeElement;state.settingsOpen=true;renderSettings();requestAnimationFrame(()=>$('#settings')?.querySelector('.modal-head .icon-btn')?.focus())}
function removeSettingsKeyHandler(){if(settingsKeyHandler){document.removeEventListener('keydown',settingsKeyHandler);settingsKeyHandler=null}}
function closeSettings(){if(!state.settingsOpen)return;removeSettingsKeyHandler();state.settingsOpen=false;if($('#settings'))$('#settings').confirmUnlink=false;renderSettings();requestAnimationFrame(()=>{if(settingsReturnFocus?.isConnected)settingsReturnFocus.focus();else $('#settingsBtn')?.focus();settingsReturnFocus=null;markVisibleChatRead()})}
function reconnectWhatsApp(){state.connection={...state.connection,state:'reconnecting',qr:null,error:null};renderShell();fetch('/api/whatsapp/reconnect',{method:'POST'}).catch(()=>{state.connection={...state.connection,state:'error',error:'Reconnect request failed.'};renderShell()})}
function wireShell(){
  conversation?.wireMessageList($('#messages'));navigation?.wireShell();
  const once=(id,event,handler)=>{const el=$(id);if(el&&!el.dataset.shellWired){el.addEventListener(event,handler);el.dataset.shellWired='true'}};
  once('#back','click',()=>{if(state.view==='gallery'&&!state.imageViewer){if(!viewHistory.closeGallery())returnFromGallery();return}backToSelection()});
  once('#messages','message-notice',event=>showToast(event.detail.message,event.detail.kind));
  once('#composer','image-sent',event=>followSentImage(event.detail));
  const messages=$('#messages');if(messages&&!messages.dataset.imageWired){messages.dataset.imageWired='true';messages.addEventListener('outbox-preview-ready',e=>sessionOutbox.completePreview(e.detail.id));messages.addEventListener('outbox-retry',event=>{if(state.connection.state==='connected'&&state.sendEnabled&&!state.mirrorOnly)void sessionOutbox.retry(event.detail.id)});messages.addEventListener('image-open',event=>openImageViewer(event.detail));for(const event of ['animationend','animationcancel'])messages.addEventListener(event,e=>{if(e.target===messages&&e.animationName==='chat-message-enter')app.classList.remove('chat-revealing')})}
  once('#settingsBtn','click',openSettings);
  once('#newMessages','click',()=>conversation.scrollBottom());
  once('#mediaBtn','click',()=>{rememberPosition();$('#messages')?.closePanel(false);$('#composer')?.closePicker();return gallery.open('media')});
  once('#connectionView','connection-reconnect',reconnectWhatsApp);
  const handle=$('#sidebarResize');if(handle&&!handle.dataset.wired){handle.dataset.wired='true';installSidebarResize(app,handle)}
  wireSettings($('#settings'));wireComposer();
}
function installSettingsKeys(){removeSettingsKeyHandler();if(!state.settingsOpen)return;settingsKeyHandler=e=>{if(e.key==='Escape'){e.preventDefault();closeSettings()}};document.addEventListener('keydown',settingsKeyHandler)}
function wireSettings(el=$('#settings')){if(!el)return;if(el.dataset.wired){if(state.settingsOpen)installSettingsKeys();return};el.dataset.wired='true';installSettingsKeys();el.addEventListener('settings-close',()=>closeSettings());el.addEventListener('follow-change',e=>{saveConfig({followNewMessages:e.detail}).catch(()=>showToast('Could not save setting.'))});el.addEventListener('read-mode-change',async e=>{el.readModeBusy=true;try{await saveConfig({readMode:e.detail})}catch{syncSettings();showToast('Could not save read setting.')}finally{el.readModeBusy=false}});el.addEventListener('settings-reconnect',()=>{closeSettings();reconnectWhatsApp()});el.addEventListener('settings-unlink',async()=>{closeSettings();state.selectedChatId=null;state.connection={...state.connection,state:'disconnecting',qr:null,error:null};renderShell();try{const r=await fetch('/api/whatsapp/logout',{method:'POST'});if(!r.ok)throw new Error('Unlink failed')}catch{state.connection={...state.connection,state:'error',error:'Could not unlink WhatsApp.'};renderShell()}});el.addEventListener('push-toggle',()=>pwa.togglePush().catch(err=>showToast(err?.message||'Could not update push notifications.')));el.addEventListener('install-app',()=>pwa.installApp())}

async function handleLaunchTarget(){
  const q=new URLSearchParams(location.search),chat=q.get('chat'),message=q.get('message');
  if(chat){history.replaceState(history.state,'',location.pathname);if(message)await openMessage(chat,message);else await selectChat(chat);return}
  const saved=viewHistory.current();
  if(saved.chatId){await selectChat(saved.chatId,{restore:true});if(saved.view==='image'&&saved.image)openImageViewer(saved.image,{history:false});if(saved.view==='gallery')await gallery.open(saved.kind,{history:false})}
}
conversation=createConversation({app,store,apiJson,onRetryMedia:retryMedia,onReply:selectReply,onReaction:setReaction,onQuote:openQuote});
gallery=createGallery({app,store,apiJson,onViewChange:renderShell,onOpen:kind=>viewHistory.openGallery(state.selectedChatId,kind),openMessage});
navigation=createNavigation({app,store,selectChat,openMessage,focusComposer,onError:message=>showToast(message)});
viewHistory=createViewHistory({onChange:async target=>{
  if(target.overlay!=='new')hideChatPicker();
  if(!target.overlay)$('#chatControls')?.hide();
  const overlay=['new','menu','mute','close'].includes(target.overlay)?target.overlay:null;
  if(target.view==='overview'){if(state.selectedChatId||app.classList.contains('open'))backToSelection(true);else renderShell({refreshMessages:false});if(overlay)showChatControls(overlay);return}
  dismissImageViewer();
  if(state.selectedChatId!==target.chatId)await selectChat(target.chatId,{restore:true});
  if(viewHistory.current().chatId!==target.chatId||viewHistory.current().view!==target.view)return;
  if(target.view==='gallery')await gallery.open(target.kind,{history:false});
  else if(state.view==='gallery')returnFromGallery();
  if(target.view==='image'&&target.image)openImageViewer(target.image,{history:false});
  if(overlay)showChatControls(overlay);
}});
sessionOutbox.subscribe(({type,entry})=>{
  if(type==='acknowledged'&&entry){
    void apiJson(`/api/chats/${encodeURIComponent(entry.chatId)}/messages/${encodeURIComponent(entry.serverId||entry.id)}/context`).then(rows=>{
      if(state.selectedChatId!==entry.chatId)return;
      const anchor=conversation.captureScrollAnchor(),bottom=conversation.isNearBottom();
      state.messages=mergeKnownReactions([...new Map([...state.messages,...rows].map(m=>[m.id,m])).values()].sort((a,b)=>a.timestamp-b.timestamp),state.reactionStates);
      state.messageCache.set(entry.chatId,{messages:[...state.messages],hasOlder:state.hasOlder,at:Date.now()});
      void conversation.renderMessages().then(()=>{if(state.selectedChatId!==entry.chatId)return;if(bottom)conversation.scrollBottom();else conversation.restoreScrollAnchor(anchor)});
    }).catch(()=>{});
  }
  if(!state.selectedChatId||state.view!=='chat'||state.chatPreparing||(entry&&entry.chatId!==state.selectedChatId))return;
  const chat=state.selectedChatId,anchor=conversation.captureScrollAnchor(),bottom=conversation.isNearBottom();
  void conversation.renderMessages().then(()=>{if(state.selectedChatId!==chat||state.view!=='chat')return;if(type==='enqueued'||bottom)conversation.scrollBottom({initial:true,force:true});else conversation.restoreScrollAnchor(anchor)});
});
navigation.installGlobalListeners();
pwa=createPwaService({store,apiJson,onStateChange:()=>{if(state.settingsOpen)renderSettings()},onError:message=>showToast(message)});
pwa.start();
const bus=createEventBus();
connectLiveEvents(bus);
async function reconcileUnread(){
 await loadStatus({render:false});
 const id=state.selectedChatId,p=state.chatStates.get(id);
 if(document.visibilityState==='visible'&&id&&state.view==='chat'&&!state.chatPreparing&&p?.last_incoming_id&&!state.messages.some(m=>m.id===p.last_incoming_id))await refreshSelectedMessages();
 markVisibleChatRead();
}
bus.on('live:open',()=>{void loadConfig().then(syncSettings).catch(()=>{});void reconcileUnread().then(()=>{syncSettings();renderUpdateNotice()}).catch(()=>{});loadChats({render:false}).then(()=>{renderChatList();renderConversationHeader()}).catch(()=>{});syncReactions().catch(()=>{})});
bus.on('live:unread',next=>{applyChatState(next);markVisibleChatRead()});
bus.on('live:chat-state',next=>{applyChatState(next);void loadChats().catch(()=>showToast('Could not refresh chat list.'))});
bus.on('live:edit',data=>updateEditView(data).catch(console.error));
bus.on('live:deletion',data=>updateDeletionView(data).catch(console.error));
bus.on('live:reaction',data=>updateReactionView(data).catch(console.error));
document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible'){syncReactions().catch(()=>{});void loadConfig().then(syncSettings).catch(()=>{});void reconcileUnread().then(()=>loadChats()).catch(()=>{})}});
bus.on('live:update',update=>{state.update=update;syncSettings();renderUpdateNotice()});
bus.on('live:connection',next=>{state.connection=next;renderShell()});
bus.on('live:config',x=>{if(x.readMode)state.readMode=x.readMode;state.followNewMessages=Boolean(x.followNewMessages);syncSettings()});
bus.on('live:message',async m=>{const localEcho=sessionOutbox.list(m.chat_id).some(e=>(e.serverId||e.id)===m.id);sessionOutbox.observe(m);m=mergeKnownReactions([m],state.reactionStates)[0];const atBottom=conversation.isNearBottom(),c=state.chats.find(x=>x.id===m.chat_id);if(c){c.last_message_id=m.id;c.last_message_at=m.timestamp;c.last_body=m.body;c.last_type=m.type;c.last_from_me=m.from_me;c.last_sender_name=m.sender_name;state.chats.sort((a,b)=>(b.last_message_at||0)-(a.last_message_at||0));renderChatList();renderConversationHeader()}else{loadChats({render:false}).then(()=>{renderChatList();renderConversationHeader()}).catch(()=>{})}if(state.selectedChatId===m.chat_id&&app.classList.contains('open')){if(!state.messages.some(x=>x.id===m.id)){state.messages.push(m);state.messageCache.set(m.chat_id,{messages:[...state.messages],hasOlder:state.hasOlder,at:Date.now()});if(state.view==='chat'){await conversation.renderMessages();if(atBottom||imageScrollIntent?.messageId===m.id){const ownImage=imageScrollIntent?.messageId===m.id;conversation.scrollBottom(ownImage?{initial:true,force:true}:{});if(ownImage)imageScrollIntent=null}else if(!localEcho){state.newWhileReading++;conversation.updateNewButton()}}if(earlyMediaEvents.delete(m.id)){refreshSelectedMessages().catch(()=>{})}}markVisibleChatRead();return}if(m.from_me||state.chatStates.get(m.chat_id)?.closed)return;if(!Number.isSafeInteger(state.chatStates.get(m.chat_id)?.unread_sequence)){state.unread.set(m.chat_id,(state.unread.get(m.chat_id)||0)+1);renderChatList()}if(state.mirrorOnly&&state.followNewMessages&&!state.chatPickerOpen&&!m.silent&&!chatMuted(state.chatStates.get(m.chat_id))&&!state.chatStates.get(m.chat_id)?.closed){selectChat(m.chat_id)}});
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
    const overlay=viewHistory.current().overlay;if(['new','menu','mute','close'].includes(overlay))showChatControls(overlay);
  }catch(err){
    console.error('Wisp initialization failed',err);
    renderShell();
  }
}
app.addEventListener('keydown',event=>{
  if(state.chatPickerOpen&&(event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='f'){
    event.preventDefault();const input=$('#chatPicker input');input?.focus();input?.select();return;
  }
  if(state.chatPickerOpen&&event.key==='Escape'&&!event.defaultPrevented&&!document.querySelector('dialog[open]')){
    event.preventDefault();event.stopPropagation();void closeChatControls();
  }
});
// Clicking nonfocusable image content can move focus to body, outside #app.
// Capture Escape before sidebar shortcuts consume it, while letting overlays
// handle their own Escape before the image underneath them.
document.addEventListener('keydown',event=>{
  if(event.key!=='Escape'||event.defaultPrevented||event.isComposing||event.repeat||!state.imageViewer||state.connection.state!=='connected')return;
  if(state.chatPickerOpen||state.settingsOpen||document.querySelector('dialog[open]')||$('#messages')?.menu)return;
  event.preventDefault();event.stopPropagation();closeImageViewer();
},true);
app.addEventListener('viewport-layout', () => conversation?.updateNewButton());
await init();
app.addEventListener('composer-resize',()=>conversation.updateNewButton());
}
