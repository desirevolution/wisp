import { replyThumbnail } from '../lib/reply-thumbnail.js';
import { LitElement, html, nothing } from 'lit';
import { repeat } from 'lit/directives/repeat.js';
import { sameMessageGroup } from '../core/message-groups.js';
import { icon } from '../lib/icons.js';
import { mediaFrameFor, rememberMediaSize } from '../core/media-layout.js';
import './wisp-emoji-picker.js';
import { rememberEmoji } from '../lib/emoji.js';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { dayKey, dayLabel, fmt, linkify, mediaLabel, rawId } from '../lib/utils.js';

export class WispMessageList extends LitElement {
  static properties = {
    chatId: { attribute:false }, messages: { attribute: false },
    sendEnabled: { type: Boolean }, pendingReactions: { attribute: false }, menu: { state: true }, expanded: { state: true },
    hasOlder: { type: Boolean },
    loading: { type: Boolean },
    hasSelection: { type: Boolean }, isGroup: { type: Boolean }, loadingOlder: { type: Boolean }, olderError: { type: Boolean }
  };

  constructor(){super();this._mediaFrames=new Map();this.messages=[];this.hasOlder=false;this.loading=false;this.hasSelection=false;this.isGroup=false;this.loadingOlder=false;this.olderError=false;this.sendEnabled=false;this.pendingReactions=new Set();this.menu=null;this.expanded=new Set();
    this._outside=e=>{const panel=this.querySelector('.message-popover');if(this.menu&&!e.composedPath().includes(panel)&&!e.composedPath().includes(this._anchor))this.closePanel()};
    this._viewportWidth=window.innerWidth;
    this._resize=()=>{
      const width=window.innerWidth, widthChanged=width!==this._viewportWidth;
      this._viewportWidth=width;
      // A fixed bottom sheet survives the keyboard closing and viewport height changes.
      if(this.menu?.mode==='actions'&&!widthChanged)return;
      this.closePanel(false);
    };
    this._scroll=()=>{if(this.menu?.mode!=='actions')this.closePanel(false)};
  }
  createRenderRoot(){return this}
  willUpdate(changed){if(changed.has('chatId'))this._mediaFrames.clear()}
  mediaFrame(m){
    if(!this._mediaFrames.has(m.id))this._mediaFrames.set(m.id,mediaFrameFor(m));
    const frame=this._mediaFrames.get(m.id);
    return `--media-ratio:${frame.ratio};--media-frame-width:${frame.width}px`;
  }
  async mediaReady(event,m){
    const el=event.currentTarget;
    const width=el.naturalWidth||el.videoWidth,height=el.naturalHeight||el.videoHeight;
    rememberMediaSize(m,width,height);
    try{await el.decode?.()}catch{/* Broken images retain the stable fallback frame. */}
    if(!el.isConnected)return;
    const frame=this._mediaFrames.get(m.id);
    if(frame&&!frame.known&&width>0&&height>0){
      const atBottom=this.scrollHeight-this.scrollTop-this.clientHeight<80;
      const top=this.getBoundingClientRect().top;
      const anchor=[...this.querySelectorAll('[data-message-id]')].find(node=>node.getBoundingClientRect().bottom>top);
      const offset=anchor?.getBoundingClientRect().top;
      this._mediaFrames.set(m.id,mediaFrameFor({...m,media_width:width,media_height:height}));
      this.requestUpdate();await this.updateComplete;
      if(!el.isConnected)return;
      if(atBottom)this.scrollTop=Math.max(0,this.scrollHeight-this.clientHeight);
      else if(anchor?.isConnected)this.scrollTop+=anchor.getBoundingClientRect().top-offset;
    }
    if(el.isConnected){el.classList.add('media-loaded');el.parentElement?.classList.add('media-ready')}
  }
  mediaFailed(event){const el=event.currentTarget;el.classList.add('media-failed');const label=el.parentElement?.querySelector('.media-frame-status');if(label)label.textContent='Media could not be displayed.'}

  connectedCallback(){super.connectedCallback();document.addEventListener('pointerdown',this._outside);window.addEventListener('resize',this._resize);this.addEventListener('scroll',this._scroll)}
  disconnectedCallback(){this.closePanel(false);document.removeEventListener('pointerdown',this._outside);window.removeEventListener('resize',this._resize);this.removeEventListener('scroll',this._scroll);super.disconnectedCallback()}
  keepMenuTapTarget(event){
    // Do not let blur reflow the message list between pointerdown and click.
    // Focus moves into the sheet after it has opened, dismissing the keyboard then.
    if(event.pointerType==='touch'&&document.activeElement?.matches('textarea,input,[contenteditable="true"]'))event.preventDefault();
  }
  emit(type,detail){this.dispatchEvent(new CustomEvent(type,{detail,bubbles:true,composed:true}))}
  closePanel(restore=true){const panel=this.querySelector('.message-popover');if(panel?.matches(':popover-open'))panel.hidePopover();this.menu=null;if(restore&&this._anchor?.isConnected)this._anchor.focus({preventScroll:true})}
  async openPanel(mode,message,event,emoji=''){
    if(mode==='picker'&&(!this.sendEnabled||this.pendingReactions.has(message.id)))return;
    const opening=!this.menu;if(opening)this._anchor=event.currentTarget;this.menu={mode,id:message.id,emoji};
    await this.updateComplete;
    const panel=this.querySelector('.message-popover');if(!panel)return;
    panel.showPopover?.();
    const picker=panel.querySelector('wisp-emoji-picker');if(picker)await picker.updateComplete;
    if(this.menu?.mode==='actions'){panel.querySelector('button:not([tabindex="-1"])')?.focus({preventScroll:true});return}
    const rect=this._anchor.getBoundingClientRect(),box=panel.getBoundingClientRect();
    const width=document.documentElement.clientWidth,height=window.visualViewport?.height || window.innerHeight;
    panel.style.left=`${Math.max(8,Math.min(rect.left,width-box.width-8))}px`;
    const below=height-rect.bottom-12;
    panel.style.top=`${Math.max(8,below>=box.height?rect.bottom+5:rect.top-box.height-5)}px`;
    const first=[...panel.querySelectorAll('button:not(:disabled)')].find(button=>button.getClientRects().length);
    (first||panel).focus({preventScroll:true});
  }
  panelKey(e){
    if(e.key==='Escape'){e.preventDefault();e.stopPropagation();this.closePanel();return}
    if(e.key==='Tab'){
      const buttons=[...e.currentTarget.querySelectorAll('button:not(:disabled):not([tabindex="-1"])')].filter(button=>button.getClientRects().length);
      if(!buttons.length)return;
      const index=buttons.indexOf(document.activeElement);
      if((e.shiftKey&&index<=0)||(!e.shiftKey&&index===buttons.length-1)){e.preventDefault();buttons[e.shiftKey?buttons.length-1:0].focus()}
    }
    if(['ArrowUp','ArrowDown','Home','End'].includes(e.key)){
      const buttons=[...e.currentTarget.querySelectorAll('button:not(:disabled):not([tabindex="-1"])')].filter(button=>button.getClientRects().length);
      if(buttons.length){e.preventDefault();const index=buttons.indexOf(document.activeElement);buttons[e.key==='Home'?0:e.key==='End'?buttons.length-1:(index+(e.key==='ArrowUp'?-1:1)+buttons.length)%buttons.length].focus()}
    }
    if(['ArrowUp','ArrowDown','Home','End','Enter',' '].includes(e.key))e.stopPropagation();
  }
  setReaction(m,emoji){
    if(!this.sendEnabled||this.pendingReactions.has(m.id))return;
    const own=(m.reactions||[]).find(r=>r.from_me);
    const next=emoji&&own?.emoji===emoji?'':emoji;
    if(emoji)rememberEmoji(emoji);
    this.emit('reaction-set',{message:m,emoji:next});this.closePanel();
  }
  reactionChips(m){
    const groups=new Map();for(const r of m.reactions||[]){const group=groups.get(r.emoji)||[];group.push(r);groups.set(r.emoji,group)}
    if(!groups.size)return null;
    return html`<div class="reaction-chips">${[...groups].map(([emoji,people])=>html`<button type="button" class=${people.some(r=>r.from_me)?'own':''} aria-label=${`${emoji}: ${people.length} reactions. Show people`} @click=${e=>this.openPanel('people',m,e,emoji)}>${emoji}${people.length>1?html`<span>${people.length}</span>`:null}</button>`)}</div>`;
  }
  renderPicker(m,own){
    return html`<wisp-emoji-picker .selected=${own?.emoji||''} .disabled=${this.pendingReactions.has(m.id)} @emoji-pick=${e=>this.setReaction(m,e.detail.emoji)} @picker-close=${()=>this.closePanel()}></wisp-emoji-picker>`;
  }
  renderPanel(){
    if(!this.menu)return null;
    const m=this.messages.find(x=>x.id===this.menu.id);if(!m)return null;
    const own=(m.reactions||[]).find(r=>r.from_me),sheet=this.menu.mode==='actions';
    return html`<div class="message-popover ${sheet?'message-sheet':''}" popover="manual" role="dialog" tabindex="-1" aria-modal=${sheet?'true':nothing} aria-label=${sheet?'Message actions':this.menu.mode==='picker'?'React to message':'Reactions'} @keydown=${this.panelKey}>
      ${sheet?html`
        <button class="message-sheet-backdrop" tabindex="-1" aria-label="Close message actions" @click=${()=>this.closePanel()}></button>
        <section class="message-sheet-content">
          <div class="reaction-panel-head"><b>Message actions</b><button type="button" aria-label="Close" @click=${()=>this.closePanel()}>${icon('close')}</button></div>
          <blockquote class="sheet-message-preview"><b>${m.from_me?'You':m.sender_name&&!rawId(m.sender_name)?m.sender_name:'Message'}</b><span>${m.body||mediaLabel(m.type)}</span></blockquote>
          <div class="sheet-reactions" aria-label="Choose a reaction">${this.renderPicker(m,own)}</div>
          <button class="sheet-reply" type="button" @click=${()=>{this.closePanel();this.emit('reply-select',{message:m})}}>${icon('reply')} Reply</button>
        </section>`:html`
        <div class="reaction-panel-head"><b>${this.menu.mode==='picker'?'React to message':`${this.menu.emoji} Reactions`}</b><button class="popover-close" type="button" aria-label="Close" @click=${()=>this.closePanel()}>${icon('close')}</button></div>
        ${this.menu.mode==='picker'?this.renderPicker(m,own):html`<ul class="reaction-people">${(m.reactions||[]).filter(r=>r.emoji===this.menu.emoji).map(r=>html`<li><span>${r.from_me?'You':r.actor_name||'Unknown participant'}</span>${r.from_me&&this.sendEnabled?html`<button type="button" ?disabled=${this.pendingReactions.has(m.id)} @click=${()=>this.setReaction(m,'')}>Remove</button>`:null}</li>`)}</ul>`}`}
    </div>`;
  }
  _loadOlder(){this.dispatchEvent(new CustomEvent('load-older',{bubbles:true,composed:true}))}
  _retry(messageId){this.dispatchEvent(new CustomEvent('retry-media',{detail:{messageId},bubbles:true,composed:true}))}
  _media(m){
    if(!m.has_media)return null;
    const visual=['image','sticker','video','gif'].includes(m.type);
    const frame=visual?this.mediaFrame(m):'';
    if(m.media_status==='pending'||!m.media_status)return visual?html`<div class="media-placeholder media-frame" style=${frame}><span class="media-frame-status">${mediaLabel(m.type)} · loading…</span></div>`:html`<div class="media-placeholder">${mediaLabel(m.type)} · loading…</div>`;
    if(m.media_status==='failed'){const status=html`${mediaLabel(m.type)} · download failed <button class="retry-media" @click=${e=>{e.stopPropagation();this._retry(m.id)}}>Retry</button>`;return visual?html`<div class="media-placeholder failed media-frame" style=${frame}><div class="media-frame-status">${status}</div></div>`:html`<div class="media-placeholder failed">${status}</div>`}
    const src=`/api/media/${encodeURIComponent(m.media_id)}`;
    if(m.type==='image'||m.type==='sticker')return html`<a class="media-frame" style=${frame} href=${src} aria-label="Open image" @click=${e=>{e.preventDefault();this.emit('image-open',{src,alt:m.body||mediaLabel(m.type)})}}><span class="media-frame-status">${mediaLabel(m.type)} · loading…</span><img class="inline-image" src=${src} loading="lazy" alt=${m.body||mediaLabel(m.type)} @load=${e=>this.mediaReady(e,m)} @error=${this.mediaFailed}></a>`;
    if(m.type==='video'||m.type==='gif')return html`<div class="media-frame" style=${frame}><span class="media-frame-status">${mediaLabel(m.type)} · loading…</span><video class="inline-video" controls preload="metadata" src=${src} @loadedmetadata=${e=>this.mediaReady(e,m)} @error=${this.mediaFailed}></video></div>`;
    if(m.type==='audio'||m.type==='ptt')return html`<audio controls preload="metadata" src=${src}></audio>`;
    return html`<a class="document" href=${src} target="_blank" rel="noopener noreferrer">${icon('paperclip')} ${m.filename||m.media_filename||'Document'}</a>`;
  }
  toggleDeleted(key){const next=new Set(this.expanded);if(next.has(key))next.delete(key);else next.add(key);this.expanded=next}
  quote(q,key){
    const open=this.expanded.has(key);
    return html`${q.deleted?html`<button class="deleted-toggle" aria-expanded=${open} @click=${()=>this.toggleDeleted(key)}>${open?'Hide deleted quote':'Show deleted quote'}</button>`:null}
      ${!q.deleted||open?html`<button type="button" class="quoted-message" aria-label="Open original message" @click=${()=>this.emit('quote-open',{quote:q})}><div class="quote-copy"><b>${q.from_me?'You':q.sender_name||'Unknown sender'}</b><span>${q.body||mediaLabel(q.type)}</span>${q.original_edited?html`<small class="quote-edited">Original edited since this reply</small>`:null}</div>${replyThumbnail(q)}</button>`:null}`;
  }
  editHistory(m){
    if(!m.edited)return null;
    const open=this.expanded.has(`edits:${m.id}`),history=m.edit_history||[];
    return html`<div class="message-edits"><button type="button" class="edit-toggle" aria-expanded=${open} @click=${()=>this.toggleDeleted(`edits:${m.id}`)}>Edited · History</button>
      ${open?html`<div class="edit-history">${!m.edit_original_available?html`<small>Original message was not received.</small>`:null}
        ${!history.length?html`<small>No earlier versions received.</small>`:null}
        ${history.map(v=>html`<div class="edit-version"><small>${v.original?'First stored version':'Edited'} · ${new Date(v.timestamp_ms).toLocaleString()}</small><div class="version-body">${v.body||mediaLabel(v.type)}</div></div>`)}
        <small>Only versions received by Wisp are available.</small></div>`:null}</div>`;
  }
  timestamp(m,inline=false){
    const date=new Date(m.timestamp*1000),valid=Number.isFinite(date.getTime());
    return html`<time class=${inline?'time inline-time':'time'} datetime=${valid?date.toISOString():''} title=${valid?date.toLocaleString():''}>${valid?fmt(m.timestamp):''}</time>`;
  }
  render(){
    if(!this.hasSelection)return html`<div class="conversation-empty" role="status"><div class="empty-mark">W</div><b>Select a chat</b><span>Choose a conversation from the chat list.</span></div>`;
    if(this.loading)return html`<div class="message-loading" role="status">Loading messages…</div>`;
    if(!this.messages.length)return html`<div class="empty">No mirrored messages yet.</div>`;
    let lastDay=null;
    return html`
      <div class="history-head" aria-live="polite">
        ${this.loadingOlder?html`<div class="history-loading"><span class="sync-spinner"></span><span>Loading older messages…</span></div>`:
          this.olderError?html`<div class="history-error"><span>Could not load older messages.</span><button class="load-older" @click=${this._loadOlder}>Retry</button></div>`:
          !this.hasOlder?html`<div class="history-start">Beginning of stored history</div>`:null}
      </div>
      ${repeat(this.messages,m=>m.id,(m,index)=>{
        const key=dayKey(m.timestamp),showDay=key!==lastDay;lastDay=key;
        const grouped=sameMessageGroup(this.messages[index-1],m);
        const inlineTime=Boolean(m.body)&&(!m.deleted||this.expanded.has(m.id));
        return html`${showDay?html`<div class="date-separator"><span>${dayLabel(m.timestamp)}</span></div>`:null}
          <div class="message-row ${m.from_me?'outgoing':''} ${grouped?'grouped':'group-start'}">
            <div class="message-stack"><div class="bubble ${m.from_me?'me':''} ${m.has_media?'media-bubble':''} ${!m.deleted&&!m.edited&&m.has_media&&!m.body&&!m.reply&&!m.reactions?.length?'media-only':''}" data-message-id=${m.id}>
              ${this.isGroup&&!grouped&&!m.from_me&&m.sender_name&&!rawId(m.sender_name)?html`<div class="sender">${m.sender_name}</div>`:null}
              ${m.deleted?html`<div class="deleted-label">Message deleted</div>${m.type!=='deleted'?html`<button class="deleted-toggle" aria-expanded=${this.expanded.has(m.id)} @click=${()=>this.toggleDeleted(m.id)}>${this.expanded.has(m.id)?'Hide deleted message':'Show deleted message'}</button>`:null}`:null}
              ${!m.deleted||this.expanded.has(m.id)?html`${m.reply?this.quote(m.reply,`quote:${m.id}`):null}${this._media(m)}${m.body?html`<div class="body"><span class="message-text">${unsafeHTML(linkify(m.body))}</span>${this.timestamp(m,true)}</div>`:null}${this.editHistory(m)}`:null}
              ${!inlineTime?this.timestamp(m):null}
            </div>
            ${!m.deleted||this.expanded.has(m.id)?this.reactionChips(m):null}</div>
            ${this.sendEnabled&&!m.deleted?html`<div class="message-actions">
              <button type="button" aria-label="Reply to message" class="desktop-message-action" title="Reply" @click=${()=>this.emit('reply-select',{message:m})}>${icon('reply')}</button>
              <button type="button" class="reaction-action desktop-message-action" aria-label="React to message" title="Add reaction" ?disabled=${this.pendingReactions.has(m.id)} @click=${e=>this.openPanel('picker',m,e)}>${icon('smile')}</button>
              <button class="mobile-menu-action" type="button" aria-label="Message actions" title="Message actions" aria-haspopup="dialog" aria-expanded=${this.menu?.id===m.id} @pointerdown=${this.keepMenuTapTarget} @click=${e=>this.openPanel('actions',m,e)}>${icon('more')}</button>
            </div>`:null}
          </div>`;
      })}
      ${this.renderPanel()}
    `;
  }
}
customElements.define('wisp-message-list',WispMessageList);
