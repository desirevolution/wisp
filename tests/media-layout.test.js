import test from 'node:test';
import assert from 'node:assert/strict';
import {mediaFrameFor,rememberMediaSize} from '../frontend/src/core/media-layout.js';
test('unknown visual media reserves a bounded fallback without a network request',()=>{
  assert.deepEqual(mediaFrameFor({id:'fallback'}),{ratio:1.5,width:420,known:false});
});
test('known dimensions preserve the original ratio within preview bounds',()=>{
  const portrait=mediaFrameFor({id:'portrait',media_width:600,media_height:1200});
  assert.deepEqual(portrait,{ratio:.5,width:180,known:true});
  assert.equal(portrait.width/portrait.ratio,360);
  assert.equal(mediaFrameFor({id:'wide',media_width:1600,media_height:900}).width,420);
});
test('loaded media dimensions are reused on later visits; invalid values are ignored',()=>{
  const m={id:'learned',media_id:'file-learned'};
  const first=mediaFrameFor(m);
  rememberMediaSize(m,400,800);
  assert.equal(first.ratio,1.5,'existing visit frame snapshot is unchanged');
  assert.equal(mediaFrameFor(m).ratio,.5);
  rememberMediaSize(m,Infinity,0);
  assert.equal(mediaFrameFor(m).ratio,.5);
  assert.equal(mediaFrameFor({id:'invalid',media_width:-1,media_height:20}).known,false);
});
