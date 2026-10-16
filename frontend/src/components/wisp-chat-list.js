import { LitElement, html } from 'lit';
import { chatStamp, dateFmt, displayName, fmt, mediaLabel, rawId } from '../lib/utils.js';

export class WispChatList extends LitElement {
  static properties = {
    chats: { attribute: false }, selected: {}, filter: {}, searchResults: { attribute: false },
    unread: { attribute: false }, keyboardSelection: {}
  };
  constructor(){super();this.chats=[];this.selected=null;this.filter='';this.searchResults=null;this.unread=new Map();this.keyboardSelection=null}
  createRenderRoot(){return this}
  _chatClick(id){this.dispatchEvent(new CustomEvent('chat-open',{detail:{id,fromSearch:Boolean(this.filter&&this.searchResults)},bubbles:true,composed:true}))}
  _messageClick(chatId,messageId){this.dispatchEvent(new CustomEvent('message-open',{detail:{chatId,messageId},bubbles:true,composed:true}))}
  _chat(c, search=false){
    const name=displayName(c.name,c.id,c.type),count=this.unread?.get(c.id)||0;
    if(search)return html`<button class="chat search-chat ${this.selected===c.id?'active':''} ${this.keyboardSelection===`chat:${c.id}`?'keyboard-selected':''}" data-key="chat:${c.id}" data-id=${c.id} @click=${()=>this._chatClick(c.id)}><span class="chat-main"><span class="chat-name">${name}</span></span></button>`;
    const sender=c.type==='group'&&c.last_sender_name&&!rawId(c.last_sender_name)?`${c.last_sender_name}: `:'';
    const preview=`${c.last_from_me?'You: ':sender}${c.last_body||mediaLabel(c.last_type)}`;
    return html`<button class="chat ${this.selected===c.id?'active':''} ${count?'unread':''} ${this.keyboardSelection===`chat:${c.id}`?'keyboard-selected':''}" data-key="chat:${c.id}" data-id=${c.id} @click=${()=>this._chatClick(c.id)}><span class="chat-main"><span class="chat-name">${count?html`<i class="unread-dot"></i>`:''}${name}</span><span class="chat-meta">${count?html`<em class="unread-count">${count}</em>`:''}<time>${chatStamp(c.last_message_at)}</time></span></span><span class="preview">${preview}</span></button>`;
  }
  render(){
    if(this.filter&&this.searchResults){const r=this.searchResults;return html`${r.chats.length?html`<div class="section-label">CHATS</div>`:''}${r.chats.map(c=>this._chat(c,true))}${r.messages.length?html`<div class="section-label">MESSAGES</div>`:''}${r.messages.map(m=>html`<button class="search-hit ${this.keyboardSelection===`message:${m.id}`?'keyboard-selected':''}" data-key="message:${m.id}" data-chat=${m.chat_id} data-message=${m.id} @click=${()=>this._messageClick(m.chat_id,m.id)}><b>${displayName(m.chat_name,m.chat_id,'')}</b><span>${(m.body||'').slice(0,140)}</span><small>${dateFmt(m.timestamp)} ${fmt(m.timestamp)}</small></button>`)}${!r.chats.length&&!r.messages.length?html`<div class="empty small">No results.</div>`:''}`}
    const q=this.filter.toLowerCase(),visible=this.chats.filter(c=>displayName(c.name,c.id,c.type).toLowerCase().includes(q));
    return visible.length?html`${visible.map(c=>this._chat(c))}`:html`<div class="empty small">No chats yet.</div>`;
  }
}
customElements.define('wisp-chat-list',WispChatList);
