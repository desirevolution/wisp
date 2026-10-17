import { LitElement, html, nothing } from 'lit';
import { icon } from '../lib/icons.js';
import { friendlyError } from './wisp-connection.js';

const labelFor = state => ({starting:'Starting…',disconnecting:'Unlinking…',reconnecting:'Reconnecting…',waiting_for_qr:'Waiting for QR scan',authenticating:'Signing in…',connected:'Connected',disconnected:'Disconnected',error:'Connection error'}[state] || state);

export class WispSettings extends LitElement {
  static properties = {
    status: { attribute:false }, followNewMessages:{type:Boolean}, mirrorOnly:{type:Boolean}, pushEnabled:{type:Boolean},
    installAvailable:{type:Boolean}, standalone:{type:Boolean}, confirmUnlink:{state:true}
  };
  constructor(){ super(); this.status={state:'starting'}; this.followNewMessages=false; this.mirrorOnly=false; this.pushEnabled=false; this.installAvailable=false; this.standalone=false; this.confirmUnlink=false; }
  createRenderRoot(){ return this; }
  emit(name,detail){ this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true})); }
  trapFocus(e){
    if(e.key!=='Tab')return;
    const buttons=[...this.querySelectorAll('button,input,summary,a[href]')].filter(x=>!x.disabled&&x.getClientRects().length);
    const index=buttons.indexOf(document.activeElement);
    if(buttons.length&&((e.shiftKey&&index<=0)||(!e.shiftKey&&index===buttons.length-1))){e.preventDefault();buttons[e.shiftKey?buttons.length-1:0].focus()}
  }
  render(){
    const s=this.status||{}, label=labelFor(s.state);
    return html`<div class="modal-backdrop" @keydown=${this.trapFocus} @click=${e=>{if(e.target===e.currentTarget)this.emit('settings-close')}}><section class="modal" role="dialog" aria-modal="true" aria-label="Settings">
      <div class="modal-head"><h2>Settings</h2><button class="icon-btn" aria-label="Close settings" @click=${()=>this.emit('settings-close')}>${icon('close')}</button></div>
      ${this.mirrorOnly?html`<div class="setting-row"><div><b>Follow new messages</b><p>Automatically open the latest incoming chat. Intended for a dedicated mirror display.</p></div><label class="switch" aria-label="Follow new messages"><input type="checkbox" .checked=${this.followNewMessages} @change=${e=>this.emit('follow-change',e.target.checked)}><span></span></label></div><hr>`:nothing}
      <div class="setting-row"><div><b>WhatsApp connection</b><p><i class="dot ${s.state==='connected'?'ok':s.state==='error'?'bad':''}"></i>${label}${s.account?` · ${s.account}`:''}</p></div><button @click=${()=>this.emit('settings-reconnect')}>Reconnect</button></div>
      ${s.state==='error'?html`<div class="error-detail"><b>Connection problem</b><p>${friendlyError(s)}</p><details><summary>Technical details</summary><pre>${s.error||''}</pre></details></div>`:nothing}
      <hr>
      <div class="setting-row"><div><b>Push notifications</b><p>${this.pushEnabled?'Enabled for this browser/device.':'Show incoming messages as Web Push notifications.'}</p></div><button class="push" @click=${()=>this.emit('push-toggle')}>${this.pushEnabled?'Disable':'Enable'}</button></div>
      ${this.installAvailable&&!this.standalone?html`<hr><div class="setting-row"><div><b>Install Wisp</b><p>Install Wisp as an app on this device.</p></div><button @click=${()=>this.emit('install-app')}>Install</button></div>`:nothing}
      <div class="danger-zone"><b>Danger zone</b><p>Remove this Wisp instance from your linked WhatsApp devices. Local Wisp history is kept.</p>
      ${this.confirmUnlink?html`<div class="confirm-box" role="alertdialog" aria-label="Confirm unlink"><b>Unlink WhatsApp?</b><p>You will need to pair Wisp again with a QR code.</p><div class="actions"><button @click=${()=>{this.confirmUnlink=false}}>Cancel</button><button class="warn" @click=${()=>this.emit('settings-unlink')}>Unlink WhatsApp</button></div></div>`:html`<button class="warn" @click=${()=>{this.confirmUnlink=true}}>Unlink WhatsApp</button>`}</div>
    </section></div>`;
  }
}
customElements.define('wisp-settings',WispSettings);
