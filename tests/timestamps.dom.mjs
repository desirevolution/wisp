import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
const {Window}=await import(process.env.WISP_DOM_MODULE||'happy-dom');
const win=new Window({url:'http://localhost'});
for(const key of ['document','customElements','HTMLElement','Element','Node','EventTarget','Event','CustomEvent','Document','CSSStyleSheet','MutationObserver','ShadowRoot','localStorage'])globalThis[key]=win[key];
globalThis.window=win;globalThis.getComputedStyle=win.getComputedStyle.bind(win);
const style=document.createElement('style');style.textContent=await readFile(new URL('../frontend/src/style.css',import.meta.url),'utf8');document.head.append(style);
await import('../frontend/src/components/wisp-message-list.js');
const base={chat_id:'group@g.us',timestamp:1792054800,type:'chat',from_me:0};
const list=document.createElement('wisp-message-list');list.hasSelection=true;list.isGroup=true;list.messages=[
 {...base,id:'short',body:'test',from_me:1},
 {...base,id:'long',body:'Long message '.repeat(60)},
 {...base,id:'multiline',body:'First line\nSecond line\n'},
 {...base,id:'link',body:'See https://example.com/?a=1&b=2'},
 {...base,id:'caption',type:'image',has_media:1,media_id:'photo',media_status:'ready',body:'A caption'},
 {...base,id:'image-only',type:'image',has_media:1,media_id:'photo',media_status:'ready',body:''},
 {...base,id:'edited',body:'New version',edited:true,edit_original_available:true,edit_history:[{body:'Old version',original:true,timestamp_ms:1792054700000}]},
 {...base,id:'deleted',body:'Retained original',deleted:true},
];document.body.append(list);await list.updateComplete;
for(const m of list.messages){
 const bubble=list.querySelector(`[data-message-id="${m.id}"]`);assert.equal(bubble.querySelectorAll('time.time').length,1,m.id);
 const time=bubble.querySelector('time');assert.ok(time.getAttribute('datetime').endsWith('Z'));assert.ok(time.title);
 if(m.body&&!m.deleted){assert.equal(bubble.querySelector('.message-text').textContent,m.body);assert.ok(bubble.querySelector('.body>.inline-time'));assert.equal(bubble.querySelector(':scope>time'),null)}
 else assert.ok(bubble.querySelector(':scope>time'));
}
assert.equal(getComputedStyle(list.querySelector('.message-row')).display,'grid');
assert.equal(getComputedStyle(list.querySelector('.message-row')).alignItems,'center');
assert.equal(getComputedStyle(list.querySelector('.message-stack')).display,'contents');
assert.equal(getComputedStyle(list.querySelector('.body')).display,'flow-root');assert.equal(getComputedStyle(list.querySelector('.inline-time')).float,'right');
assert.equal(getComputedStyle(list.querySelector('.inline-time')).whiteSpace,'nowrap');
assert.equal(getComputedStyle(list.querySelector('.inline-time')).position,'relative');
assert.equal(getComputedStyle(list.querySelector('.inline-time')).top,'6px');
assert.equal(getComputedStyle(list.querySelector('.inline-time')).fontSize,'12px');
assert.equal(list.querySelectorAll('[data-message-id="link"] .msg-link').length,1);
assert.ok(list.querySelector('[data-message-id="image-only"]').classList.contains('media-only'));
assert.equal(getComputedStyle(list.querySelector('[data-message-id="image-only"]>time')).position,'absolute');
list.toggleDeleted('deleted');list.toggleDeleted('edits:edited');await list.updateComplete;
assert.equal(list.querySelectorAll('[data-message-id="deleted"] time').length,1);assert.ok(list.querySelector('[data-message-id="deleted"] .inline-time'));
assert.ok(list.querySelector('[data-message-id="edited"] .edit-history').textContent.includes('Old version'));assert.equal(list.querySelectorAll('[data-message-id="edited"] time').length,1);
function luminance(hex){const rgb=hex.match(/\w\w/g).map(x=>parseInt(x,16)/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4);return rgb[0]*.2126+rgb[1]*.7152+rgb[2]*.0722}
for(const [fg,bg] of [['b8c7ce','202c33'],['c4ded7','005c4b'],['c1d1d7','005c4b']]){
 const ratio=(luminance(fg)+.05)/(luminance(bg)+.05);assert.ok(ratio>=4.5,`${fg}: ${ratio}`);console.log(`Contrast #${fg} / #${bg}: ${ratio.toFixed(2)}:1`);
}
list.remove();await win.happyDOM.close();console.log('Timestamp DOM passed: one semantic timestamp, inline text/caption, preserved multiline/link text, media overlay, edit/deletion toggles and contrast. Pixel wrapping requires a real browser.');
