import { LitElement, html } from 'lit';
import { EMOJI_GROUPS, EMOJI_ALIASES, recentEmojis } from '../lib/emoji.js';

export class WispEmojiPicker extends LitElement {
  static properties = { selected: { type: String } };
  constructor() { super(); this.selected = ''; }
  createRenderRoot() { return this; }
  emit(type, detail) { this.dispatchEvent(new CustomEvent(type, { detail, bubbles: true, composed: true })); }
  button(emoji) {
    const name = (EMOJI_ALIASES.get(emoji) || ['emoji'])[0];
    return html`<button type="button" title=${`:${name}:`} aria-label=${name} aria-pressed=${this.selected === emoji ? 'true' : 'false'} class=${this.selected === emoji ? 'selected' : ''} @click=${() => this.emit('emoji-pick', { emoji })}>${emoji}</button>`;
  }
  keydown(event) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); this.emit('picker-close'); return; }
    const buttons = [...this.querySelectorAll('.emoji-grid button')];
    const index = buttons.indexOf(document.activeElement);
    if (index < 0) return;
    const columns = getComputedStyle(buttons[index].parentElement).gridTemplateColumns.split(' ').length;
    const delta = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: columns, ArrowUp: -columns }[event.key];
    if (delta) { event.preventDefault(); event.stopPropagation(); buttons[(index + delta + buttons.length) % buttons.length].focus(); }
    if (event.key === 'Enter' || event.key === ' ') event.stopPropagation();
  }
  render() {
    const recent = recentEmojis();
    return html`<div @keydown=${this.keydown}>
      ${recent.length ? html`<div class="emoji-label">Recent</div><div class="emoji-grid">${recent.map(e => this.button(e))}</div>` : null}
      ${EMOJI_GROUPS.map((group, i) => html`<div class="emoji-label">${i ? 'Gestures' : 'Smileys'}</div><div class="emoji-grid">${group.map(e => this.button(e))}</div>`)}
    </div>`;
  }
}
customElements.define('wisp-emoji-picker', WispEmojiPicker);
