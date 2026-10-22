import assert from 'node:assert/strict';
import {Window} from 'happy-dom';
const win=new Window({url:'http://localhost'});
for(const key of ['document','customElements','HTMLElement','Element','Node','EventTarget','Event','CustomEvent','Document','CSSStyleSheet','MutationObserver','ShadowRoot'])globalThis[key]=win[key];
globalThis.window=win;
await import('../frontend/src/components/wisp-settings.js');
const el=document.createElement('wisp-settings');document.body.append(el);await el.updateComplete;
const select=el.querySelector('select');assert.equal(select.value,'wisp');assert.equal(select.options.length,3);
let mode;el.addEventListener('read-mode-change',e=>mode=e.detail);
select.value='devices';select.dispatchEvent(new Event('change',{bubbles:true}));assert.equal(mode,'devices');
// The parent updates the property only after the backend confirms saving.
el.readMode='devices';el.readModeBusy=true;await el.updateComplete;
assert.equal(select.disabled,true);assert.ok(el.querySelector('.read-mode-hint').textContent.includes('without sending a normal read receipt'));
el.readModeBusy=false;select.value='receipts';el.requestUpdate();await el.updateComplete;assert.equal(select.value,'devices');
el.readMode='receipts';await el.updateComplete;assert.ok(el.querySelector('.read-mode-hint').textContent.includes('privacy setting'));
el.remove();await win.happyDOM.close();console.log('Read settings DOM passed: modes, confirmation, failure reset and busy state.');
