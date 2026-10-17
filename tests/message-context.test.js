import test from 'node:test';
import assert from 'node:assert/strict';
import { createDrafts } from '../frontend/src/core/drafts.js';
import { applyReactionState } from '../frontend/src/core/reactions.js';
import { ALL_EMOJIS } from '../frontend/src/lib/emoji.js';

test('in-memory drafts are isolated per chat and reset without a storage adapter',()=>{
 const drafts=createDrafts();drafts.update('a',{text:'answer',reply:{message_id:'m'}});
 drafts.update('b',{text:'other'});
 assert.equal(drafts.get('a').text,'answer');assert.equal(drafts.get('b').reply,null);
 drafts.update('a',{reply:null});assert.equal(drafts.get('a').text,'answer');
 assert.equal(createDrafts().get('a').text,'');
});
test('reaction snapshots reject late responses and only update their target',()=>{
 const messages=[{id:'m',chat_id:'a',reactions:[{emoji:'👍'}],reaction_version:3},{id:'n',chat_id:'a'}];
 const stale=applyReactionState(messages,{chat_id:'a',message_id:'m',reactions:[],version:2});
 assert.equal(stale[0],messages[0]);
 const fresh=applyReactionState(messages,{chat_id:'a',message_id:'m',reactions:[],version:4});
 assert.deepEqual(fresh[0].reactions,[]);assert.equal(fresh[1],messages[1]);
 assert.equal(applyReactionState(messages,{chat_id:'other',message_id:'m',reactions:[],version:5})[0],messages[0]);
});
test('picker exposes the agreed 32 emoji subset',()=>{assert.equal(new Set(ALL_EMOJIS).size,32);assert.ok(ALL_EMOJIS.includes('❤️'));assert.ok(!ALL_EMOJIS.includes('🦊'))});
