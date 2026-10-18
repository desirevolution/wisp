import { LitElement, html } from 'lit';
import { icon } from '../lib/icons.js';
import { apiJson } from '../lib/api.js';
import { chatMuted } from '../core/chat-state.js';

// A normal sidebar/page view: no modal, backdrop or focus trap.
export class WispChatPicker extends LitElement {
 static properties = { opened:{state:true}, blocked:{type:Boolean}, query:{state:true}, options:{state:true}, loading:{state:true}, error:{state:true}, busy:{state:true}, chatStates:{attribute:false} };
 constructor(){super();this.opened=false;this.blocked=false;this.query='';this.options=[];this.loading=false;this.error='';this.busy=false;this.chatStates=new Map();this._request=0;this._session=0}
 createRenderRoot(){return this}
 emit(name,detail){this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true}))}
 async open(){
  if(this.opened)return;
  ++this._session;this.opened=true;this.query='';this.options=[];this.error='';this.busy=false;
  void this.load();await this.updateComplete;
  if(!this.opened||this.blocked)return;
  const focus=window.matchMedia('(min-width:701px)').matches?'input':'.picker-back';
  this.querySelector(focus)?.focus({preventScroll:true});
 }
 hide(){if(!this.opened)return;this.opened=false;++this._request;++this._session}
 disconnectedCallback(){++this._request;++this._session;super.disconnectedCallback()}
 async load(){
  const request=++this._request;this.loading=true;this.error='';
  try{const options=await apiJson('/api/chat-options?q=');if(request===this._request)this.options=Array.isArray(options)?options:[]}
  catch(e){if(request===this._request)this.error=e.message||'Could not load chats and contacts.'}
  finally{if(request===this._request)this.loading=false}
 }
 close(){if(!this.busy)this.emit('chat-picker-close')}
 get filtered(){
  const query=this.query.trim().toLocaleLowerCase();
  return this.options.filter(o=>!query||`${o.name||''} ${o.id}`.toLocaleLowerCase().includes(query)).map(o=>({...o,...this.chatStates.get(o.id)}));
 }
 async choose(option){
  if(this.busy||this.blocked||!this.opened)return;
  const session=this._session;this.busy=true;this.error='';
  try{
   const state=await apiJson(`/api/chats/${encodeURIComponent(option.id)}/state`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({action:'open'})});
   this.emit('chat-action-complete',{action:'open',state,option,pickerActive:this.opened&&session===this._session});
  }catch(e){if(session===this._session){this.error=e.message||'Could not open chat.';this.busy=false}}
 }
 keydown(e){
  e.stopPropagation();
  if(e.key==='Escape'){e.preventDefault();this.close();return}
  if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='f'){e.preventDefault();const input=this.querySelector('input');input?.focus();input?.select();return}
  const input=e.target.matches('input'),option=e.target.closest('.chat-option');
  if(!input&&!option)return;
  const buttons=[...this.querySelectorAll('.chat-option:not([disabled])')];
  if(e.key==='Enter'&&input){e.preventDefault();buttons[0]?.click();return}
  if(e.key!=='ArrowDown'&&e.key!=='ArrowUp')return;
  e.preventDefault();if(!buttons.length)return;
  const index=buttons.indexOf(option),delta=e.key==='ArrowDown'?1:-1;
  const next=index<0?(delta>0?0:buttons.length-1):(index+delta+buttons.length)%buttons.length;
  buttons[next].focus({preventScroll:true});buttons[next].scrollIntoView({block:'nearest'});
 }
 render(){
  if(!this.opened)return null;
  const options=this.filtered;
  return html`<section class="chat-picker-view" aria-labelledby="chatPickerTitle" @keydown=${this.keydown}>
   <header class="chat-picker-top top"><button type="button" class="icon-btn picker-back" aria-label="Back to chat list" title="Back to chat list (Escape)" ?disabled=${this.busy} @click=${this.close}>${icon('back')}</button><h2 id="chatPickerTitle">New chat</h2></header>
   <div class="chat-picker-search" role="search" aria-label="Chats and contacts"><input class="chat-options-search" type="search" aria-label="Search chats and contacts" placeholder="Search chats and contacts" autocomplete="off" .value=${this.query} @input=${e=>{this.query=e.target.value}} ?disabled=${this.busy||this.blocked}></div>
   <div class="chat-picker-results" aria-busy=${this.loading?'true':'false'}>
    ${this.error?html`<div class="picker-error" role="alert"><p>${this.error}</p><button type="button" ?disabled=${this.blocked||this.busy} @click=${this.load}>Try again</button></div>`:null}
    ${this.loading&&!this.options.length?html`<p class="empty small" role="status">Loading chats and contacts…</p>`:html`
     <div class="chat-options">${options.map(o=>html`<button type="button" class="chat-option" ?disabled=${this.busy||this.blocked} @click=${()=>this.choose(o)}><b>${o.name||o.id.split('@')[0]}</b><small>${o.closed?'Closed chat':o.contact?'WhatsApp contact':o.type==='group'?'Group':'Chat'}${chatMuted(o)?' · Muted':''}</small></button>`)}</div>
     ${!options.length&&!this.error?html`<p class="empty small">No matching chats or WhatsApp contacts.</p>`:null}`}
    ${this.busy?html`<p class="empty small" role="status">Opening chat…</p>`:null}
   </div>
  </section>`;
 }
}
customElements.define('wisp-chat-picker',WispChatPicker);
