import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { attachKeyboardViewport } from '../frontend/src/services/keyboard-viewport.js';

function setup({ visual = true } = {}) {
 const win = new EventTarget();win.Event=Event;win.innerHeight=800;
 const viewport=Object.assign(new EventTarget(),{height:800,offsetTop:0,scale:1});if(visual)win.visualViewport=viewport;
 const callbacks=new Map();let id=0;
 win.requestAnimationFrame=fn=>{callbacks.set(++id,fn);return id};win.cancelAnimationFrame=id=>callbacks.delete(id);
 const messages={scrollHeight:2000,scrollTop:1300,clientHeight:700};
 const active={matches:()=>true};const doc={activeElement:active};
 const root=Object.assign(new EventTarget(),{ownerDocument:doc,contains:el=>el===active,querySelector:()=>messages});
 const values=new Map();root.style={setProperty:(k,v)=>{values.set(k,v);if(k==='--app-viewport-height')messages.clientHeight=parseFloat(v)-100},removeProperty:k=>{values.delete(k);if(k==='--app-viewport-height')messages.clientHeight=win.innerHeight-100}};
 const stop=attachKeyboardViewport(root,{win});
 const flush=()=>{const fns=[...callbacks.values()];callbacks.clear();for(const fn of fns)fn()};
 flush();return {root,doc,active,win,viewport,messages,values,stop,flush,callbacks};
}
test('keyboard height/panning follows the visible viewport and keeps the latest message visible',()=>{
 const x=setup();x.viewport.height=460;x.viewport.offsetTop=25;x.viewport.dispatchEvent(new Event('resize'));x.viewport.dispatchEvent(new Event('scroll'));
 assert.equal(x.callbacks.size,1);x.flush();assert.equal(x.values.get('--app-viewport-height'),'460px');assert.equal(x.values.get('--app-viewport-top'),'25px');assert.equal(x.messages.scrollTop,1640);
 x.viewport.height=800;x.viewport.offsetTop=0;x.viewport.dispatchEvent(new Event('resize'));x.flush();assert.equal(x.messages.scrollTop,1300);x.stop();
});
test('reading history is not forced to the end on keyboard resize',()=>{
 const x=setup();x.messages.scrollTop=300;x.viewport.height=400;x.viewport.dispatchEvent(new Event('resize'));x.flush();assert.equal(x.messages.scrollTop,300);x.stop();
});
test('blur, zoom and disconnect clear overrides without disabling browser zoom',()=>{
 const x=setup();x.viewport.height=450;x.viewport.dispatchEvent(new Event('resize'));x.flush();
 x.viewport.scale=2;x.viewport.dispatchEvent(new Event('resize'));x.flush();assert.equal(x.values.size,0);
 x.viewport.scale=1;x.viewport.dispatchEvent(new Event('resize'));x.flush();assert.equal(x.values.get('--app-viewport-height'),'450px');
 x.doc.activeElement=null;x.root.dispatchEvent(new Event('focusout'));x.flush();assert.equal(x.values.size,0);
 x.root.dispatchEvent(new Event('focusin'));x.stop();assert.equal(x.callbacks.size,0);x.viewport.dispatchEvent(new Event('resize'));x.win.dispatchEvent(new Event('resize'));assert.equal(x.callbacks.size,0);
});
test('native layout/orientation changes work, including without VisualViewport',()=>{
 const x=setup();x.win.innerHeight=500;x.viewport.height=500;x.win.dispatchEvent(new Event('orientationchange'));x.flush();assert.equal(x.values.get('--app-viewport-height'),'500px');x.stop();
 const legacy=setup({visual:false});legacy.win.innerHeight=400;legacy.win.dispatchEvent(new Event('resize'));legacy.flush();assert.equal(legacy.values.size,0);legacy.stop();
});
test('viewport meta and narrow panes use the keyboard-aware shell',()=>{
 const html=readFileSync(new URL('../frontend/index.html',import.meta.url),'utf8');assert.match(html,/interactive-widget=resizes-content/);assert.doesNotMatch(html,/user-scalable=no|maximum-scale=1/);
 const css=readFileSync(new URL('../frontend/src/style.css',import.meta.url),'utf8');assert.match(css,/\.sidebar,\.conversation,\.app\.connection-mode \.conversation\{height:100%\}/);
});
