import { LitElement, html } from 'lit';
import { icon } from '../lib/icons.js';
import { apiJson } from '../lib/api.js';
import { chatMuted, muteLabel } from '../core/chat-state.js';

export class WispChatControls extends LitElement {
 static properties = { mode:{state:true}, context:{attribute:false}, error:{state:true}, busy:{state:true} };
 constructor(){super();this.mode=null;this.context={};this.error='';this.busy=false;this._outside=e=>{if(this.mode&&this._popup&&!this.contains(e.target)&&!document.querySelector('#chatMenuBtn')?.contains(e.target))this.close()};this._resize=()=>{if(this.mode)void this.present()}}
 connectedCallback(){super.connectedCallback();document.addEventListener('pointerdown',this._outside,true);window.addEventListener('resize',this._resize)}
 disconnectedCallback(){document.removeEventListener('pointerdown',this._outside,true);window.removeEventListener('resize',this._resize);super.disconnectedCallback()}
 createRenderRoot(){return this}
 emit(name,detail){this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true}))}
 async show(mode,context={}) {
  this._returnFocus=document.activeElement;document.activeElement?.blur();
  this.context=context;this.mode=mode;this.error='';this.busy=false;
  await this.updateComplete;
  await this.present();
  this.focusFirst();
 }
 async present(){
  await this.updateComplete;
  const dialog=this.querySelector('dialog');if(!this.mode||!dialog)return;
  const popup=window.matchMedia('(min-width:701px)').matches&&this.mode!=='close';
  if(dialog.open&&this._popup!==popup)dialog.close();
  this._popup=popup;dialog.classList.toggle('chat-controls-popup',popup);
  if(!dialog.open){if(popup)dialog.show();else dialog.showModal()}
  document.querySelector('#chatMenuBtn')?.setAttribute('aria-expanded','true');
  if(popup){
   const anchor=document.querySelector('#chatMenuBtn')?.getBoundingClientRect();
   const box=dialog.getBoundingClientRect(),gap=8;
   dialog.style.left=`${Math.max(gap,Math.min((anchor?.right??window.innerWidth-gap)-box.width,window.innerWidth-box.width-gap))}px`;
   dialog.style.top=`${Math.max(gap,Math.min((anchor?.bottom??gap)+6,window.innerHeight-box.height-gap))}px`;
  }else{dialog.style.removeProperty('left');dialog.style.removeProperty('top')}
 }
 focusFirst(){(this.querySelector('.chat-menu-actions button:not(:disabled),.chat-confirm-actions button:not(:disabled)')||this.querySelector('button:not(:disabled)'))?.focus({preventScroll:true})}
 async changeMode(mode){this.mode=mode;this.error='';await this.present();this.focusFirst()}
 keydown(e){
  e.stopPropagation();
  if(e.key==='Escape'){e.preventDefault();this.close();return}
  if(!this._popup)return;
  const buttons=[...this.querySelectorAll('button:not(:disabled)')].filter(button=>button.getClientRects().length),index=buttons.indexOf(document.activeElement);
  if(['ArrowDown','ArrowUp','Home','End'].includes(e.key)){
   e.preventDefault();buttons[e.key==='Home'?0:e.key==='End'?buttons.length-1:(index+(e.key==='ArrowUp'?-1:1)+buttons.length)%buttons.length]?.focus();
  }else if(e.key==='Tab'){e.preventDefault();buttons[(index+(e.shiftKey?-1:1)+buttons.length)%buttons.length]?.focus()}
 }
 hide(){if(!this.mode)return;this.querySelector('dialog')?.close();this.mode=null;document.querySelector('#chatMenuBtn')?.setAttribute('aria-expanded','false');this._returnFocus?.isConnected&&this._returnFocus.focus({preventScroll:true})}
 close(){if(!this.busy)this.emit('chat-controls-close')}
 async act(action,extra={}){
  if(this.busy)return;this.busy=true;this.error='';
  try{const state=await apiJson(`/api/chats/${encodeURIComponent(extra.id||this.context.id)}/state`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({action,...(extra.mute_for?{mute_for:extra.mute_for}:{})})});this.emit('chat-action-complete',{action,state,option:extra.option})}
  catch(e){this.error=e.message||'Could not update chat.';this.busy=false}
 }
 render(){
  if(!this.mode)return null;
  const muted=chatMuted(this.context.state),title=({menu:this.context.name||'Chat',mute:'Mute chat',close:'Close chat?'})[this.mode];
  return html`<dialog class="chat-controls" data-mode=${this.mode} aria-labelledby="chatControlsTitle" @keydown=${this.keydown} @cancel=${e=>{e.preventDefault();this.close()}} @click=${e=>{if(e.target===e.currentTarget){const b=e.currentTarget.getBoundingClientRect();if(e.clientX<b.left||e.clientX>b.right||e.clientY<b.top||e.clientY>b.bottom)this.close()}}}>
   <header>${this.mode==='mute'?html`<button class="icon-btn" aria-label="Back to chat menu" ?disabled=${this.busy} @click=${()=>this.changeMode('menu')}>${icon('back')}</button>`:null}<h2 id="chatControlsTitle">${title}</h2><button class="icon-btn" type="button" aria-label="Close dialog" ?disabled=${this.busy} @click=${this.close}>${icon('close')}</button></header>
   <div class="chat-controls-body">
    ${this.error?html`<p class="chat-controls-error" role="alert">${this.error}</p>`:null}
    ${this.mode==='menu'?html`<div class="chat-menu-actions">
     ${muted?html`<p>${muteLabel(this.context.state)}</p><button ?disabled=${this.busy} @click=${()=>this.act('mute',{mute_for:'off'})}>Unmute chat</button><button ?disabled=${this.busy} @click=${()=>this.changeMode('mute')}>Change mute duration…</button>`:html`<button @click=${()=>this.changeMode('mute')}>Mute chat…</button>`}
     <button ?disabled=${this.busy} @click=${()=>this.changeMode('close')}>Close chat…</button>
    </div>`:null}
    ${this.mode==='mute'?html`<p>Mute push notifications, MQTT events and in-app alerts on all Wisp devices.</p><div class="chat-menu-actions">${[['8h','8 hours'],['1w','1 week'],['forever','Indefinitely']].map(([value,label])=>html`<button ?disabled=${this.busy} @click=${()=>this.act('mute',{mute_for:value})}>${label}</button>`)}</div>`:null}
    ${this.mode==='close'?html`<p>Hide <b>${this.context.name}</b> from the chat list on all Wisp devices and mark it as read in Wisp.</p><p>Unsent drafts, replies and attached images will be discarded on all devices. Messages and media stay available under “New chat”. WhatsApp is unchanged.</p><p>${muted?'This muted chat stays closed when messages arrive.':'A new message will reopen this chat.'}</p><div class="chat-confirm-actions"><button ?disabled=${this.busy} @click=${this.close}>Cancel</button><button class="danger" ?disabled=${this.busy} @click=${()=>this.act('close')}>Close chat</button></div>`:null}
    ${this.busy?html`<p role="status">Saving…</p>`:null}
   </div>
  </dialog>`;
 }
}
customElements.define('wisp-chat-controls',WispChatControls);
