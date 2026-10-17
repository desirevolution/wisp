import { LitElement, html } from 'lit';
import { icon } from '../lib/icons.js';
import { live } from 'lit/directives/live.js';
import { emojiMatch, rememberEmoji } from '../lib/emoji.js';
import { mediaLabel } from '../lib/utils.js';
import { sessionDrafts } from '../core/drafts.js';
import './wisp-emoji-picker.js';

export class WispComposer extends LitElement {
  static properties = { blocked:{type:Boolean}, chatId: { attribute: false }, pickerOpen: { state: true }, completion: { state: true }, completionIndex: { state: true } };
  constructor() {
    super(); this.blocked=false; this.chatId = null; this.pickerOpen = false; this.completion = null; this.completionIndex = 0;
    this._outside = e => { if (this.pickerOpen && !e.composedPath().includes(this)) this.closePicker(); };
  }
  createRenderRoot() { return this; }
  connectedCallback() {
    super.connectedCallback(); document.addEventListener('pointerdown', this._outside);
    this._unsubscribe = sessionDrafts.subscribe(chat => { if (chat === this.chatId) this.requestUpdate(); });
  }
  disconnectedCallback() { document.removeEventListener('pointerdown', this._outside); this._unsubscribe?.(); super.disconnectedCallback(); }
  closePicker() {
    if(!this.pickerOpen)return;
    this.pickerOpen=false;
  }
  togglePicker() {
    if(this.pickerOpen){this.closePicker();return}
    this._hideCompletion();this.pickerOpen=true;
  }
  get input() { return this.querySelector('#composerInput'); }
  get draft() { return sessionDrafts.get(this.chatId); }
  focusInput() { if(this.blocked)return;this.input?.focus({ preventScroll: true }); }
  setReply(message) {
    if (!this.chatId || this.draft.sending) return;
    sessionDrafts.update(this.chatId, { reply: { message_id: message.id, sender_name: message.from_me ? 'You' : (message.sender_name || 'Unknown sender'), body: message.body, type: message.type }, error: '' });
    this.updateComplete.then(() => this.focusInput());
  }
  cancelReply() { if (!this.draft.sending) sessionDrafts.update(this.chatId, { reply: null }); }
  _hideCompletion() { this.completion = null; this.completionIndex = 0; }
  _onInput() {
    sessionDrafts.update(this.chatId, { text: this.input.value });
    this.completion = emojiMatch(this.input);
    this.completionIndex = Math.min(this.completionIndex, (this.completion?.items.length || 1) - 1);
  }
  _insertEmoji(emoji) {
    const input = this.input; if (!input || this.draft.sending) return;
    const a = input.selectionStart ?? input.value.length, b = input.selectionEnd ?? a;
    input.setRangeText(emoji, a, b, 'end'); rememberEmoji(emoji); this._onInput(); this._hideCompletion(); this.closePicker(); input.focus();
  }
  _acceptCompletion() {
    if (!this.completion) return;
    const emoji = this.completion.items[this.completionIndex].emoji;
    this.input.setRangeText(emoji, this.completion.start, this.completion.end, 'end');
    rememberEmoji(emoji); this._onInput(); this._hideCompletion(); this.focusInput();
  }
  _onKeydown(e) {
    if (e.isComposing || e.keyCode === 229) return;
    if (this.completion) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); e.stopPropagation(); this.completionIndex = (this.completionIndex + (e.key === 'ArrowDown' ? 1 : -1) + this.completion.items.length) % this.completion.items.length; return; }
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this._acceptCompletion(); return; }
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); this._hideCompletion(); return; }
    }
    if (e.key === 'Escape' && (this.pickerOpen || this.draft.reply)) {
      e.preventDefault(); e.stopPropagation(); if (this.pickerOpen) this.closePicker(); else this.cancelReply(); return;
    }
    if (e.key === 'Enter' && !e.shiftKey && window.matchMedia('(min-width: 701px)').matches) { e.preventDefault(); void this.send(); }
  }
  _attachImage(file) {
    if (!this.chatId || this.draft.sending || !file) return;
    let error = '';
    if (!['image/png', 'image/jpeg'].includes(file.type)) error = 'Choose a PNG or JPEG image.';
    else if (!file.size || file.size > 10 * 1024 * 1024) error = 'Image must be between 1 byte and 10 MiB.';
    if (error) { sessionDrafts.update(this.chatId, { error }); return; }
    sessionDrafts.update(this.chatId, { image: { file, url: URL.createObjectURL(file) }, error: '' });
    this._hideCompletion(); this.closePicker();
    this.updateComplete.then(() => this.focusInput());
  }
  _onPaste(e) {
    const files = [...(e.clipboardData?.items || [])].filter(x => x.kind === 'file' && x.type.startsWith('image/')).map(x => x.getAsFile()).filter(Boolean);
    if (!files.length) files.push(...[...(e.clipboardData?.files || [])].filter(x => x.type.startsWith('image/')));
    if (!files.length) return; // Text and image URLs retain normal paste behaviour.
    e.preventDefault(); this._attachImage(files[0]);
  }
  _onFile(e) { this._attachImage(e.target.files?.[0]); e.target.value = ''; }
  removeImage() { if (!this.draft.sending) sessionDrafts.update(this.chatId, { image: null, error: '' }); }
  async send() {
    const chat = this.chatId, draft = this.draft, text = draft.text.trim();
    if (this.blocked || !chat || draft.sending || (!text && !draft.image)) return;
    const revision = draft.revision;
    sessionDrafts.update(chat, { sending: true, error: '' });
    try {
      let path = 'send', options;
      if (draft.image) {
        const form = new FormData();
        form.append('image', draft.image.file, draft.image.file.name || 'pasted-image');
        form.append('caption', text); form.append('reply_to', draft.reply?.message_id || '');
        path = 'send-image'; options = { method: 'POST', body: form };
      } else options = { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ text, reply_to: draft.reply?.message_id || '' }) };
      const response = await fetch(`/api/chats/${encodeURIComponent(chat)}/${path}`, options);
      const result = await response.json().catch(() => null);
      if (!response.ok || result?.ok !== true) throw new Error(result?.error || 'Message could not be sent');
      if (sessionDrafts.get(chat).revision === revision) sessionDrafts.update(chat, { text: '', reply: null, image: null });
      this.closePicker(); this._hideCompletion();
    } catch (e) { sessionDrafts.update(chat, { error: e.message || 'Message could not be sent' }); }
    finally { sessionDrafts.update(chat, { sending: false }); await this.updateComplete; if (this.isConnected && this.chatId === chat) this.focusInput(); }
  }
  updated() {
    const input = this.input;
    if (!input) return;
    input.style.height = 'auto';
    input.style.height = `${Math.min(input.scrollHeight, 120)}px`;
    this.dispatchEvent(new CustomEvent('composer-resize', { bubbles: true }));
  }
  render() {
    const d = this.draft;
    return html`<div class="composer-wrap">
      ${d.image ? html`<div class="image-draft"><img src=${d.image.url} alt="Image to send"><div><b>Image ready to send</b><small>${(d.image.file.size / 1024 / 1024).toFixed(1)} MiB · PNG/JPEG · max. 10 MiB</small></div><button type="button" aria-label="Remove image" title="Remove image" ?disabled=${d.sending || this.blocked} @click=${this.removeImage}>${icon('close')}</button></div>` : null}
      ${d.reply ? html`<div class="reply-draft"><div><b>Reply to ${d.reply.sender_name}</b><span>${d.reply.body || mediaLabel(d.reply.type)}</span></div><button type="button" aria-label="Cancel reply" title="Cancel reply (Escape)" ?disabled=${d.sending || this.blocked} @click=${this.cancelReply}>${icon('close')}</button></div>` : null}
      <div id="emojiPicker" class="emoji-picker" ?hidden=${!this.pickerOpen}>
        <wisp-emoji-picker @emoji-pick=${e => this._insertEmoji(e.detail.emoji)} @picker-close=${() => { this.closePicker(); this.focusInput(); }}></wisp-emoji-picker>
      </div>
      <div id="emojiComplete" class="emoji-complete" ?hidden=${!this.completion}>
        ${this.completion?.items.map((x, i) => html`<button type="button" class=${i === this.completionIndex ? 'active' : ''} @mousedown=${e => { e.preventDefault(); this.completionIndex = i; this._acceptCompletion(); }}><span>${x.emoji}</span><small>:${x.names[0]}:</small></button>`)}
      </div>
      <input id="imageFile" type="file" accept="image/png,image/jpeg" hidden ?disabled=${d.sending || this.blocked} @change=${this._onFile}>
      <div class="composer">
        <button class="image-btn" type="button" aria-label="Choose image" title="Choose image (PNG/JPEG, max. 10 MiB)" ?disabled=${d.sending || this.blocked} @click=${() => this.querySelector('#imageFile')?.click()}>${icon('image')}</button>
        <button id="emojiBtn" class="emoji-btn" type="button" aria-label="Emoji" title="Emoji" ?disabled=${d.sending || this.blocked} @click=${this.togglePicker}>${icon('smile')}</button>
        <textarea id="composerInput" rows="1" maxlength="4000" placeholder=${d.image ? "Image caption (optional)" : "Message"} aria-label=${d.image ? "Image caption" : "Message"} .value=${live(d.text)} ?disabled=${d.sending || this.blocked} @paste=${this._onPaste} @input=${this._onInput} @keydown=${this._onKeydown}></textarea>
        <button id="sendBtn" class="send-btn" type="button" aria-label="Send" title="Send" ?disabled=${d.sending || this.blocked || (!d.text.trim() && !d.image)} @click=${this.send}>${icon('send')}</button>
      </div>
      <div id="sendError" class="send-error" role="alert" ?hidden=${!d.error}>${d.error}</div>
    </div>`;
  }
}
customElements.define('wisp-composer', WispComposer);
