import { LitElement, html, nothing } from 'lit';
import { dateFmt, mediaLabel } from '../lib/utils.js';

const KINDS = ['media', 'audio', 'links', 'documents'];
const safeLink = value => {
  try {
    const url = new URL(String(value || ''));
    return ['http:', 'https:'].includes(url.protocol) ? url.href : null;
  } catch { return null; }
};

export class WispGallery extends LitElement {
  static properties = {
    kind: { type: String }, rows: { attribute: false }, loading: { type: Boolean }, error: { type: String }
  };
  constructor(){ super(); this.kind='media'; this.rows=[]; this.loading=false; this.error=''; }
  createRenderRoot(){ return this; }
  emit(name, detail){ this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true})); }
  renderPreview(item){
    if(item.status!=='ready') return html`<div class="tile-placeholder">${item.status}</div>`;
    const src=`/api/media/${encodeURIComponent(item.id)}`;
    if(this.kind==='media'&&['image','sticker'].includes(item.type)) return html`<img src=${src} loading="lazy" alt="">`;
    if(this.kind==='media'&&['video','gif'].includes(item.type)) return html`<video src=${src} preload="metadata"></video>`;
    return html`<div class="tile-placeholder">${mediaLabel(item.type)}<br>${item.filename||''}</div>`;
  }
  renderLinks(){
    return this.rows.map(item=>{
      const href=safeLink(item.url);
      return html`<div class="link-card">
        ${href?html`<a href=${href} target="_blank" rel="noopener noreferrer">${item.url}</a>`:html`<span>${item.url}</span>`}
        <small>${dateFmt(item.timestamp)}</small>
        <button @click=${()=>this.emit('gallery-jump',{messageId:item.message_id})}>Show in chat</button>
      </div>`;
    });
  }
  renderMedia(){
    return html`<div class="media-grid">${this.rows.map(item=>html`<button class="media-tile" @click=${()=>this.emit('gallery-jump',{messageId:item.message_id})}>${this.renderPreview(item)}<span>${dateFmt(item.timestamp)}</span></button>`)}</div>`;
  }
  render(){
    return html`
      <div class="gallery-tabs">${KINDS.map(kind=>html`<button class=${kind===this.kind?'active':''} @click=${()=>this.emit('gallery-kind',{kind})}>${kind[0].toUpperCase()+kind.slice(1)}</button>`)}</div>
      <div class="gallery-body">
        ${this.loading?html`<div class="empty">Loading…</div>`:nothing}
        ${!this.loading&&this.error?html`<div class="empty">${this.error}</div>`:nothing}
        ${!this.loading&&!this.error&&!this.rows.length?html`<div class="empty">Nothing here yet.</div>`:nothing}
        ${!this.loading&&!this.error&&this.rows.length?(this.kind==='links'?this.renderLinks():this.renderMedia()):nothing}
      </div>`;
  }
}
customElements.define('wisp-gallery', WispGallery);
