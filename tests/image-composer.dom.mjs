// Optional: WISP_DOM_MODULE=... node --conditions=browser tests/image-composer.dom.mjs
import assert from 'node:assert/strict';
const {Window}=await import(process.env.WISP_DOM_MODULE||'happy-dom');
const win=new Window({url:'http://localhost'});
for(const key of ['document','customElements','HTMLElement','Element','Node','EventTarget','Event','CustomEvent','KeyboardEvent','Document','CSSStyleSheet','MutationObserver','ShadowRoot','localStorage','File','FormData'])globalThis[key]=win[key];
globalThis.window=win;
let next=0;const revoked=[];URL.createObjectURL=()=>`blob:test-${++next}`;URL.revokeObjectURL=url=>revoked.push(url);
await import('../frontend/src/components/wisp-composer.js');
const {sessionDrafts}=await import('../frontend/src/core/drafts.js');
const c=document.createElement('wisp-composer');c.chatId='group@g.us';document.body.append(c);await c.updateComplete;
const image=new File(['PNG'], 'screenshot.png',{type:'image/png'});
let calls=[];globalThis.fetch=async(url,options)=>{calls.push({url,options});return {ok:true,json:async()=>({ok:true})}};
function paste(items){const e=new Event('paste',{bubbles:true,cancelable:true});Object.defineProperty(e,'clipboardData',{value:{items}});c.input.dispatchEvent(e);return e}
const text=paste([{kind:'string',type:'text/plain'}]);assert.equal(text.defaultPrevented,false);
sessionDrafts.update(c.chatId,{text:'Caption',reply:{message_id:'original',sender_name:'Arthur',body:'Question'}});
const e=paste([{kind:'file',type:'image/png',getAsFile:()=>image}]);assert.equal(e.defaultPrevented,true);
await c.updateComplete;assert.ok(c.querySelector('.image-draft img'));assert.equal(c.draft.text,'Caption');assert.equal(c.draft.reply.message_id,'original');assert.equal(calls.length,0,'paste must never send');
const oldURL=c.draft.image.url;c._attachImage(image);assert.ok(revoked.includes(oldURL));
const url=c.draft.image.url;c._attachImage(new File(['SVG'],'vector.svg',{type:'image/svg+xml'}));assert.equal(c.draft.image.url,url);assert.match(c.draft.error,/PNG/);
c._attachImage(new File([new Uint8Array(10*1024*1024+1)],'large.png',{type:'image/png'}));assert.match(c.draft.error,/10 MiB/);assert.equal(c.draft.image.url,url);
globalThis.fetch=async()=>({ok:false,json:async()=>({error:'Upload failed'})});await c.send();assert.equal(c.draft.image.url,url);assert.equal(c.draft.text,'Caption');assert.equal(c.draft.reply.message_id,'original');assert.equal(c.draft.error,'Upload failed');
let finish;globalThis.fetch=async(url,options)=>{calls.push({url,options});return new Promise(resolve=>finish=()=>resolve({ok:true,json:async()=>({ok:true})}))};
const sending=c.send();assert.equal(c.draft.sending,true);c._attachImage(image);assert.equal(c.draft.image.url,url);
await c.updateComplete;
assert.match(c.querySelector('.send-progress').textContent,/Sending image/);
assert.equal(c.querySelector('#sendBtn').disabled,true);
const req=calls.at(-1);assert.ok(req.url.endsWith('/send-image'));assert.equal(req.options.headers,undefined);assert.equal(req.options.body.get('caption'),'Caption');assert.equal(req.options.body.get('reply_to'),'original');assert.equal(req.options.body.get('image').name,'screenshot.png');
c.chatId='other@g.us';await c.updateComplete;c._attachImage(image);const otherURL=c.draft.image.url;
finish();await sending;assert.equal(sessionDrafts.get('group@g.us').image,null);assert.equal(sessionDrafts.get('group@g.us').text,'');assert.ok(revoked.includes(url));assert.equal(c.draft.image.url,otherURL);
c.removeImage();await c.updateComplete;assert.equal(c.querySelector('.image-draft'),null);assert.ok(revoked.includes(otherURL));
// File selection uses the same preview and supports image-only messages.
const target={files:[image],value:'selected'};c._onFile({target});assert.equal(target.value,'');await c.updateComplete;
globalThis.fetch=async(url,options)=>{assert.equal(options.body.get('caption'),'');return {ok:true,json:async()=>({ok:true})}};await c.send();assert.equal(c.draft.image,null);
c.remove();await win.happyDOM.close();console.log('Image composer DOM passed: paste, preview, replacement, size/type errors, failed send, caption/reply, chat switch, removal, file input and image-only send');
