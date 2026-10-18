import { LitElement, html } from 'lit';
import { icon } from '../lib/icons.js';
import { apiJson } from '../lib/api.js';
import { chatMuted, muteLabel } from '../core/chat-state.js';

export class WispChatControls extends LitElement {
 static properties = { mode:{state:true}, context:{attribute:false}, error:{state:true}, busy:{state:true} };
 constructor(){super();this.mode=null;this.context={};this.error='';this.busy=false}
 createRenderRoot(){return this}
 emit(name,detail){this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true}))}
 async show(mode,context={}) {
  this._returnFocus=document.activeElement;document.activeElement?.blur();
  this.context=context;this.mode=mode;this.error='';this.busy=false;
  await this.updateComplete;
  const dialog=this.querySelector('dialog');if(!this.mode||!dialog)return;if(!dialog.open)dialog.showModal();
  this.querySelector('button')?.focus({preventScroll:true});
 }
 hide(){if(!this.mode)return;this.querySelector('dialog')?.close();this.mode=null;this._returnFocus?.isConnected&&this._returnFocus.focus({preventScroll:true})}
 close(){if(!this.busy)this.emit('chat-controls-close')}
 async act(action,extra={}){
  if(this.busy)return;this.busy=true;this.error='';
  try{const state=await apiJson(`/api/chats/${encodeURIComponent(extra.id||this.context.id)}/state`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({action,...(extra.mute_for?{mute_for:extra.mute_for}:{})})});this.emit('chat-action-complete',{action,state,option:extra.option})}
  catch(e){this.error=e.message||'Could not update chat.';this.busy=false}
 }
 render(){
  if(!this.mode)return null;
  const muted=chatMuted(this.context.state),title=({menu:this.context.name||'Chat',mute:'Mute chat',close:'Close chat?'})[this.mode];
  return html`<dialog class="chat-controls" data-mode=${this.mode} aria-labelledby="chatControlsTitle" @keydown=${e=>e.stopPropagation()} @cancel=${e=>{e.preventDefault();this.close()}} @click=${e=>{if(e.target===e.currentTarget){const b=e.currentTarget.getBoundingClientRect();if(e.clientX<b.left||e.clientX>b.right||e.clientY<b.top||e.clientY>b.bottom)this.close()}}}>
   <header><h2 id="chatControlsTitle">${title}</h2><button class="icon-btn" type="button" aria-label="Close dialog" ?disabled=${this.busy} @click=${this.close}>${icon('close')}</button></header>
   <div class="chat-controls-body">
    ${this.error?html`<p class="chat-controls-error" role="alert">${this.error}</p>`:null}
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
