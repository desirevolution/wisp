import { LitElement, html, nothing } from 'lit';
import {live} from 'lit/directives/live.js';
import {apiJson} from '../lib/api.js';
import { icon } from '../lib/icons.js';
import { friendlyError } from './wisp-connection.js';

const labelFor = state => ({starting:'Starting…',disconnecting:'Unlinking…',reconnecting:'Reconnecting…',waiting_for_qr:'Waiting for QR scan',authenticating:'Signing in…',connected:'Connected',disconnected:'Disconnected',error:'Connection error'}[state] || state);

export class WispSettings extends LitElement {
  static properties = {
    readModeBusy:{type:Boolean}, readMode:{type:String}, version:{type:String}, releaseUpdate:{attribute:false},
    status: { attribute:false }, followNewMessages:{type:Boolean}, mirrorOnly:{type:Boolean}, pushEnabled:{type:Boolean},
    pushTestEnabled:{type:Boolean}, pushSubscriptions:{type:Number}, pushTestBusy:{state:true}, pushTestResult:{state:true}, pushTestError:{state:true},
    installAvailable:{type:Boolean}, standalone:{type:Boolean}, confirmUnlink:{state:true}
  };
  constructor(){ super(); this.readMode='wisp';this.readModeBusy=false; this.status={state:'starting'}; this.followNewMessages=false; this.mirrorOnly=false; this.pushEnabled=false; this.installAvailable=false; this.standalone=false; this.confirmUnlink=false;this.pushTestEnabled=false;this.pushSubscriptions=0;this.pushTestBusy=false;this.pushTestResult=null;this.pushTestError=''; }
  createRenderRoot(){ return this; }
  emit(name,detail){ this.dispatchEvent(new CustomEvent(name,{detail,bubbles:true,composed:true})); }
  async sendTestPush(){
    if(!this.pushTestEnabled||this.pushTestBusy)return;
    this.pushTestBusy=true;this.pushTestError='';this.pushTestResult=null;
    try{this.pushTestResult=await apiJson('/api/push/test',{method:'POST',signal:AbortSignal.timeout(45000)})}
    catch(e){this.pushTestError=e.message||'Could not send the push test.'}
    finally{this.pushTestBusy=false}
  }
  renderPushTest(){
    if(!this.pushTestEnabled)return nothing;
    const result=this.pushTestResult;
    return html`<section class="push-test-panel" aria-label="Test push notifications"><div class="setting-row"><div><b>Test push notifications</b><p>Send a test to all registered devices (${this.pushSubscriptions}). No WhatsApp message is sent.</p></div><button type="button" ?disabled=${this.pushTestBusy} @click=${this.sendTestPush}>${this.pushTestBusy?'Sending test…':'Send test to all devices'}</button></div>
      ${this.pushTestError?html`<p role="alert">${this.pushTestError}</p>`:nothing}
      ${result?html`<div class="push-test-results"><p role="status">${result.total===0?'No subscriptions are registered.':`${result.accepted} of ${result.total} accepted by push providers. ${result.expired} expired; ${result.failed} failed.`}</p><p>Provider acceptance does not confirm a visible notification on the device.</p><small>Test ID: ${result.test_id}</small><ul>${(result.results||[]).map(row=>html`<li><b>${row.device}</b> · ${row.provider}<br>${row.result==='accepted'?'Accepted by push provider':row.result==='expired'?'Expired subscription':'Failed'}${row.http_status?` · HTTP ${row.http_status}`:''}${row.reason?` · ${row.reason.replaceAll('_',' ')}`:''}</li>`)}</ul></div>`:nothing}
    </section>`;
  }
  trapFocus(e){
    if(e.key!=='Tab')return;
    const buttons=[...this.querySelectorAll('button,input,select,summary,a[href]')].filter(x=>!x.disabled&&x.getClientRects().length);
    const index=buttons.indexOf(document.activeElement);
    if(buttons.length&&((e.shiftKey&&index<=0)||(!e.shiftKey&&index===buttons.length-1))){e.preventDefault();buttons[e.shiftKey?buttons.length-1:0].focus()}
  }
  render(){
    const s=this.status||{}, label=labelFor(s.state);
    return html`<div class="modal-backdrop" @keydown=${this.trapFocus} @click=${e=>{if(e.target===e.currentTarget)this.emit('settings-close')}}><section class="modal" role="dialog" aria-modal="true" aria-label="Settings">
      <div class="modal-head"><h2>Settings</h2><button class="icon-btn" aria-label="Close settings" @click=${()=>this.emit('settings-close')}>${icon('close')}</button></div>
      <div class="setting-row app-version"><div><b>Wisp ${this.version||'dev'}</b><p>${this.releaseUpdate?.error?'Update check unavailable.':this.releaseUpdate?.available?`Version ${this.releaseUpdate.latest} is available.`:!this.releaseUpdate?.comparable?'Development build.':this.releaseUpdate?.checkedAt?'You are up to date.':'Checking for updates…'}</p>${this.releaseUpdate?.checkedAt?html`<small>Last checked: ${new Date(this.releaseUpdate.checkedAt).toLocaleString()}</small>`:nothing}</div>${this.releaseUpdate?.available?html`<a href=${this.releaseUpdate.url} target="_blank" rel="noopener noreferrer">View release</a>`:nothing}</div><hr>
      ${this.mirrorOnly?html`<div class="setting-row"><div><b>Follow new messages</b><p>Automatically open the latest incoming chat. Intended for a dedicated mirror display.</p></div><label class="switch" aria-label="Follow new messages"><input type="checkbox" .checked=${this.followNewMessages} @change=${e=>this.emit('follow-change',e.target.checked)}><span></span></label></div><hr>`:nothing}
      <div class="setting-row read-mode-setting"><div><b>When reading in Wisp</b><p id="read-mode-help">Applies to all Wisp devices. Device sync also marks messages read in Wisp when you read them in WhatsApp.</p></div><select aria-label="When reading in Wisp" aria-describedby="read-mode-help" .value=${live(this.readMode)} ?disabled=${this.readModeBusy} @change=${e=>this.emit('read-mode-change',e.target.value)}>
        <option value="wisp" ?selected=${this.readMode==='wisp'}>Wisp only</option>
        <option value="devices" ?selected=${this.readMode==='devices'}>Sync my devices</option>
        <option value="receipts" ?selected=${this.readMode==='receipts'}>Send WhatsApp read receipts</option>
      </select></div><p class="read-mode-hint">${this.readMode==='receipts'?'Uses your WhatsApp read-receipt privacy setting.':this.readMode==='devices'?'Syncs your own devices without sending a normal read receipt to the sender.':'Keeps read status in Wisp. WhatsApp remains unchanged.'}</p><hr>
      <div class="setting-row"><div><b>WhatsApp connection</b><p><i class="dot ${s.state==='connected'?'ok':s.state==='error'?'bad':''}"></i>${label}${s.account?` · ${s.account}`:''}</p></div><button @click=${()=>this.emit('settings-reconnect')}>Reconnect</button></div>
      ${s.state==='error'?html`<div class="error-detail"><b>Connection problem</b><p>${friendlyError(s)}</p><details><summary>Technical details</summary><pre>${s.error||''}</pre></details></div>`:nothing}
      <hr>
      <div class="setting-row"><div><b>Push notifications</b><p>${this.pushEnabled?'Enabled for this browser/device.':'Show incoming messages as Web Push notifications.'}</p></div><button class="push" @click=${()=>this.emit('push-toggle')}>${this.pushEnabled?'Disable':'Enable'}</button></div>
      ${this.renderPushTest()}
      ${this.installAvailable&&!this.standalone?html`<hr><div class="setting-row"><div><b>Install Wisp</b><p>Install Wisp as an app on this device.</p></div><button @click=${()=>this.emit('install-app')}>Install</button></div>`:nothing}
      <div class="danger-zone"><b>Danger zone</b><p>Remove this Wisp instance from your linked WhatsApp devices. Local Wisp history is kept.</p>
      ${this.confirmUnlink?html`<div class="confirm-box" role="alertdialog" aria-label="Confirm unlink"><b>Unlink WhatsApp?</b><p>You will need to pair Wisp again with a QR code.</p><div class="actions"><button @click=${()=>{this.confirmUnlink=false}}>Cancel</button><button class="warn" @click=${()=>this.emit('settings-unlink')}>Unlink WhatsApp</button></div></div>`:html`<button class="warn" @click=${()=>{this.confirmUnlink=true}}>Unlink WhatsApp</button>`}</div>
    </section></div>`;
  }
}
customElements.define('wisp-settings',WispSettings);
