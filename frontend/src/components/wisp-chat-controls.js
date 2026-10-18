import { LitElement, html } from 'lit';
import { icon } from '../lib/icons.js';
import { apiJson } from '../lib/api.js';
import { chatMuted, muteLabel } from '../core/chat-state.js';

export class WispChatControls extends LitElement {
 static properties = { mode:{state:true}, context:{attribute:false}, query:{state:true}, options:{state:true}, loading:{state:true}, error:{state:true}, busy:{state:true} };
 constructor(){super();this.mode=null;this.context={};this.query='';this.options=[];this.loading=false;this.error='';this.busy=false;this._request=0}
 createRenderRoot(){return this}
 emit(name,detail){this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true}))}
 async show(mode,context={}) {
  this._returnFocus=document.activeElement;document.activeElement?.blur();
  this.context=context;this.mode=mode;this.error='';this.busy=false;
  if(mode==='new'){this.query='';this.options=[];void this.search()}
  await this.updateComplete;
  const dialog=this.querySelector('dialog');if(!dialog.open)dialog.showModal();
  (this.querySelector('input')||this.querySelector('button'))?.focus({preventScroll:true});
 }
 hide(){if(!this.mode)return;++this._request;clearTimeout(this._timer);this.querySelector('dialog')?.close();this.mode=null;this._returnFocus?.isConnected&&this._returnFocus.focus({preventScroll:true})}
 close(){if(!this.busy)this.emit('chat-controls-close')}
 disconnectedCallback(){++this._request;clearTimeout(this._timer);super.disconnectedCallback()}
 async search(){
  const request=++this._request;this.loading=true;this.error='';
  try{const options=await apiJson(`/api/chat-options?q=${encodeURIComponent(this.query.trim())}`);if(request===this._request)this.options=Array.isArray(options)?options:[]}
  catch(e){if(request===this._request)this.error=e.message||'Could not load chats and contacts.'}
  finally{if(request===this._request)this.loading=false}
 }
 input(e){this.query=e.target.value;++this._request;this.loading=true;clearTimeout(this._timer);this._timer=setTimeout(()=>this.search(),180)}
 async act(action,extra={}){
  if(this.busy)return;this.busy=true;this.error='';
  try{const state=await apiJson(`/api/chats/${encodeURIComponent(extra.id||this.context.id)}/state`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({action,...(extra.mute_for?{mute_for:extra.mute_for}:{})})});this.emit('chat-action-complete',{action,state,option:extra.option})}
  catch(e){this.error=e.message||'Could not update chat.';this.busy=false}
 }
 render(){
  if(!this.mode)return null;
  const muted=chatMuted(this.context.state),title=({new:'New chat',menu:this.context.name||'Chat',mute:'Mute chat',close:'Close chat?'})[this.mode];
  return html`<dialog class="chat-controls" aria-labelledby="chatControlsTitle" @keydown=${e=>e.stopPropagation()} @cancel=${e=>{e.preventDefault();this.close()}} @click=${e=>{if(e.target===e.currentTarget){const b=e.currentTarget.getBoundingClientRect();if(e.clientX<b.left||e.clientX>b.right||e.clientY<b.top||e.clientY>b.bottom)this.close()}}}>
   <header><h2 id="chatControlsTitle">${title}</h2><button class="icon-btn" type="button" aria-label="Close dialog" ?disabled=${this.busy} @click=${this.close}>${icon('close')}</button></header>
   <div class="chat-controls-body">
    ${this.error?html`<p class="chat-controls-error" role="alert">${this.error}</p>`:null}
    ${this.mode==='new'?html`
     <input class="chat-options-search" type="search" aria-label="Search chats and contacts" placeholder="Search chats and contacts" .value=${this.query} @input=${this.input} ?disabled=${this.busy}>
     ${this.loading?html`<p role="status">Loading chats and contacts…</p>`:this.error?html`<button @click=${this.search}>Try again</button>`:html`
     <div class="chat-options">${this.options.map(o=>html`<button type="button" class="chat-option" ?disabled=${this.busy} @click=${()=>this.act('open',{id:o.id,option:o})}><b>${o.name||o.id.split('@')[0]}</b><small>${o.closed?'Closed chat':o.contact?'WhatsApp contact':o.type==='group'?'Group':'Chat'}${chatMuted(o)?' · Muted':''}</small></button>`)}</div>
     ${!this.options.length?html`<p>No matching chats or WhatsApp contacts.</p>`:null}`}
    `:null}
    ${this.mode==='menu'?html`<div class="chat-menu-actions">
     ${muted?html`<p>${muteLabel(this.context.state)}</p><button ?disabled=${this.busy} @click=${()=>this.act('mute',{mute_for:'off'})}>Unmute chat</button><button ?disabled=${this.busy} @click=${()=>{this.mode='mute';this.error=''}}>Change mute duration…</button>`:html`<button @click=${()=>{this.mode='mute';this.error=''}}>Mute chat…</button>`}
     <button ?disabled=${this.busy} @click=${()=>{this.mode='close';this.error=''}}>Close chat…</button>
    </div>`:null}
    ${this.mode==='mute'?html`<p>Mute push notifications, MQTT events and in-app alerts on all Wisp devices.</p><div class="chat-menu-actions">${[['8h','8 hours'],['1w','1 week'],['forever','Indefinitely']].map(([value,label])=>html`<button ?disabled=${this.busy} @click=${()=>this.act('mute',{mute_for:value})}>${label}</button>`)}</div>`:null}
    ${this.mode==='close'?html`<p>Hide <b>${this.context.name}</b> from the chat list on all Wisp devices and mark it as read in Wisp.</p><p>Unsent drafts, replies and attached images will be discarded on all devices. Messages and media stay available under “New chat”. WhatsApp is unchanged.</p><p>${muted?'This muted chat stays closed when messages arrive.':'A new message will reopen this chat.'}</p><div class="chat-confirm-actions"><button ?disabled=${this.busy} @click=${this.close}>Cancel</button><button class="danger" ?disabled=${this.busy} @click=${()=>this.act('close')}>Close chat</button></div>`:null}
    ${this.busy?html`<p role="status">Saving…</p>`:null}
   </div>
  </dialog>`;
 }
}
customElements.define('wisp-chat-controls',WispChatControls);
