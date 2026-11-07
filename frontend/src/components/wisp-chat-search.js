import { LitElement, html } from 'lit';
import { apiJson } from '../lib/api.js';
import { dateFmt, fmt } from '../lib/utils.js';

export class WispChatSearch extends LitElement {
  static properties = {
    chatId: { state: true },
    query: { state: true },
    results: { state: true },
    pending: { state: true },
    error: { state: true },
  };

  createRenderRoot() { return this; }

  async open(chatId, { reset = false } = {}) {
    if (this.chatId !== chatId || reset) {
      this.cancelSearch();
      this.chatId = chatId;
      this.query = '';
      this.results = null;
      this.pending = false;
      this.error = '';
    }
    await this.updateComplete;
    const dialog = this.querySelector('dialog');
    if (!dialog.open) dialog.showModal();
    const previous = [...this.querySelectorAll('[data-result]')].find(button => button.dataset.result === this.lastResult);
    (previous || this.querySelector('input')).focus();
  }

  hide() {
    this.querySelector('dialog')?.close();
  }

  cancelSearch() {
    clearTimeout(this.timer);
    this.request?.abort();
    this.request = null;
    this.lastResult = null;
  }

  disconnectedCallback() {
    this.cancelSearch();
    super.disconnectedCallback();
  }

  search(value) {
    this.cancelSearch();
    this.query = value;
    this.results = null;
    this.error = '';
    const q = value.trim();
    this.pending = Boolean(q);
    if (q) this.timer = setTimeout(() => this.load(q), 180);
  }

  async load(q) {
    const request = new AbortController();
    this.request = request;
    const params = new URLSearchParams({ q, chat: this.chatId });
    try {
      const response = await apiJson(`/api/search?${params}`, { signal: request.signal });
      if (this.request === request) this.results = response.messages;
    } catch {
      if (this.request === request) this.error = 'Could not search this chat.';
    } finally {
      if (this.request === request) {
        this.pending = false;
        this.request = null;
      }
    }
  }

  emit(type, detail) {
    this.dispatchEvent(new CustomEvent(type, { detail, bubbles: true }));
  }

  render() {
    if (!this.chatId) return null;
    return html`<dialog class="chat-search-dialog" aria-labelledby="chatSearchTitle"
      @cancel=${event => { event.preventDefault(); this.emit('chat-search-close'); }}>
      <header><h2 id="chatSearchTitle">Search this chat</h2>
        <button type="button" @click=${() => this.emit('chat-search-close')}>Close</button></header>
      <input type="search" aria-label="Search messages in this chat" placeholder="Search messages"
        .value=${this.query || ''} @input=${event => this.search(event.target.value)}>
      <div class="chat-search-results" aria-busy=${Boolean(this.pending)}>
        ${this.pending ? html`<p role="status">Searching…</p>` : null}
        ${this.error ? html`<p role="alert">${this.error} <button type="button" @click=${() => this.search(this.query)}>Retry</button></p>` : null}
        ${this.results?.length === 0 ? html`<p role="status">No results in this chat.</p>` : null}
        ${this.results?.map(message => html`<button type="button" class="search-hit" data-result=${message.id}
          @click=${() => { this.lastResult = message.id; this.emit('chat-search-result', message.id); }}>
          <span>${(message.body || '').slice(0, 180)}</span>
          <small>${dateFmt(message.timestamp)} ${fmt(message.timestamp)}</small>
        </button>`)}
      </div>
    </dialog>`;
  }
}

customElements.define('wisp-chat-search', WispChatSearch);
