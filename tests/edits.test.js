import test from 'node:test';
import assert from 'node:assert/strict';
import {applyEditState,learnMessageEdits,mergeEdited} from '../frontend/src/core/edits.js';

test('edits replace text without changing position and retain quoted snapshots',()=>{
 const chat='edit-test@g.us';
 const messages=[{id:'a',chat_id:chat,body:'old',timestamp:10},{id:'reply',chat_id:chat,body:'answer',timestamp:20,reply:{chat_id:chat,message_id:'a',body:'old'}}];
 const changed=applyEditState(messages,{chat_id:chat,message_id:'a',body:'new',edited_at:30000,edit_version:2,edit_history:[{body:'old',original:true}],original_available:true});
 assert.equal(changed[0].body,'new');assert.equal(changed[0].timestamp,10);assert.equal(changed[1].reply.body,'old');assert.equal(changed[1].reply.original_edited,true);
 const stale=applyEditState(changed,{chat_id:chat,message_id:'a',body:'intermediate',edited_at:25000,edit_version:1});assert.equal(stale[0].body,'new');
 assert.equal(mergeEdited(messages[0]).body,'new');
 const laterReply=mergeEdited({...messages[1],timestamp:40});assert.ok(!laterReply.reply.original_edited);
});
test('early edits survive arrival of an original and higher server revisions teach quote warnings',()=>{
 const chat='early-edit@g.us';
 applyEditState([],{chat_id:chat,message_id:'a',body:'corrected',edited_at:30000,edit_version:1,edit_history:[],original_available:false});
 const original={id:'a',chat_id:chat,body:'original',timestamp:10};assert.equal(mergeEdited(original).body,'corrected');
 learnMessageEdits([{...original,edited:true,body:'newer',edited_at:50000,edit_version:3,edit_history:[{body:'original'}],edit_original_available:true}]);
 const m=mergeEdited(original);assert.equal(m.body,'newer');assert.equal(m.edit_original_available,true);
 assert.equal(mergeEdited({...original,chat_id:'other'}).body,'original');
});
