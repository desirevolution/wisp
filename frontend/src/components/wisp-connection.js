import { LitElement, html, nothing } from 'lit';

function friendlyError(status = {}) {
  if (status.errorCode === 'auth_failure') return 'WhatsApp authentication failed. Open Settings and pair again.';
  return status.error || 'WhatsApp could not connect.';
}

export class WispConnection extends LitElement {
  static properties = { status: { attribute: false } };
  constructor() { super(); this.status = { state: 'starting' }; }
  createRenderRoot() { return this; }
  reconnect() { this.dispatchEvent(new CustomEvent('connection-reconnect', { bubbles: true, composed: true })); }
  render() {
    const status = this.status || {};
    const state = status.state;
    const q = status.qr;
    let title = 'Starting Wisp';
    let text = 'Preparing the WhatsApp connection…';
    let action = false;
    if (state === 'disconnecting') { title = 'Disconnecting WhatsApp'; text = 'Removing the current linked-device session…'; }
    else if (state === 'reconnecting') { title = 'Reconnecting'; text = 'Restarting the WhatsApp connection…'; }
    else if (state === 'waiting_for_qr') { title = 'Connect WhatsApp'; text = q ? 'Scan this QR code with WhatsApp to link Wisp.' : 'Generating a QR code…'; }
    else if (state === 'authenticating') { title = 'Signing in'; text = 'WhatsApp accepted the QR code. Finishing the connection…'; }
    else if (state === 'disconnected') { title = 'WhatsApp disconnected'; text = 'The connection was interrupted. Wisp will try to reconnect automatically.'; action = true; }
    else if (state === 'error') { title = 'Connection problem'; text = friendlyError(status); action = true; }
    return html`<div class="connection-view"><div class="connection-card">
      <div class="connection-spinner ${state === 'waiting_for_qr' && q ? 'hidden' : ''}"></div>
      <h2>${title}</h2><p>${text}</p>
      ${state === 'waiting_for_qr' && q ? html`<div class="qr-wrap"><img src=${q} alt="WhatsApp pairing QR code"></div><ol><li>Open WhatsApp on your phone</li><li>Open <b>Linked devices</b></li><li>Choose <b>Link a device</b> and scan the code</li></ol><small>The QR code refreshes automatically when needed.</small>` : nothing}
      ${state === 'waiting_for_qr' ? html`<aside class="pairing-notice" role="note"><b>Before linking</b><p>Wisp is community software using an unofficial WhatsApp connection. WhatsApp changes can interrupt the connection. Push delivery and access to older media are best effort.</p></aside>` : nothing}
      ${action ? html`<button @click=${this.reconnect}>${state === 'error' ? 'Try again' : 'Reconnect now'}</button>` : nothing}
    </div></div>`;
  }
}
customElements.define('wisp-connection', WispConnection);
export { friendlyError };
