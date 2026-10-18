import test from 'node:test';
import assert from 'node:assert/strict';
import { createDrafts } from '../frontend/src/core/drafts.js';
import { chatMuted, muteLabel } from '../frontend/src/core/chat-state.js';

test('remote close discards text, reply and attachment, retaining a durable revision',()=>{
 const saved=new Map(),storage={getItem:key=>saved.get(key),setItem:(key,value)=>saved.set(key,value)};
 const drafts=createDrafts({storage});
 drafts.update('chat',{text:'Unsent',reply:{message_id:'original'},image:{url:'blob:test'}});
 assert.equal(drafts.invalidate('chat',2),true);
 assert.equal(drafts.get('chat').text,'');assert.equal(drafts.get('chat').reply,null);assert.equal(drafts.get('chat').image,null);
 const restored=createDrafts({storage});restored.update('chat',{text:'A new draft'});
 assert.equal(restored.invalidate('chat',2),false);assert.equal(restored.invalidate('chat',1),false);
 assert.equal(restored.get('chat').text,'A new draft');
 assert.equal(createDrafts({storage}).get('chat').serverRevision,2);
 assert.equal(restored.invalidate('chat',3),true);assert.equal(restored.get('chat').text,'');
});
test('offline drafts are discarded at the next sync without clearing another chat',()=>{
 const drafts=createDrafts({storage:null});drafts.update('closed',{text:'Offline draft'});drafts.update('other',{text:'Keep me'});
 drafts.invalidate('closed',1);assert.equal(drafts.get('closed').text,'');assert.equal(drafts.get('other').text,'Keep me');
});
test('mute deadlines expire without treating permanent mute as a past timestamp',()=>{
 assert.equal(chatMuted({mute_until:-1},10000),true);
 assert.equal(chatMuted({mute_until:20},10000),true);assert.equal(chatMuted({mute_until:10},10000),false);
 assert.equal(chatMuted({mute_until:0},10000),false);assert.equal(chatMuted(null),false);
 assert.equal(muteLabel({mute_until:-1}),'Muted indefinitely');
});
