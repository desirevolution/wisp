import test from 'node:test';
import assert from 'node:assert/strict';
import {createOutbox} from '../frontend/src/core/outbox.js';
const tick=()=>new Promise(resolve=>setImmediate(resolve));
test('snapshot frees the next draft, queues sends per chat and retries with one ID',async()=>{
 let sequence=0,release;const sent=[];
 const outbox=createOutbox({newId:()=>`id-${++sequence}`,transport:async entry=>{sent.push({id:entry.id,text:entry.text,reply:entry.reply});if(sent.length===1)await new Promise(done=>release=done);if(sent.length===2)throw Error('offline');return {id:entry.id}}});
 const a=outbox.enqueue({chatId:'a',text:'first',reply:{message_id:'original'}}),b=outbox.enqueue({chatId:'a',text:'second'});
 await tick();assert.equal(sent.length,1);assert.equal(outbox.get(b.entry.id).status,'queued');release();assert.equal(await a.done,true);assert.equal(await b.done,false);
 assert.equal(outbox.get(b.entry.id).error,'offline');await outbox.retry(b.entry.id);assert.deepEqual(sent.map(s=>s.id),['id-1','id-2','id-2']);assert.equal(sent[0].reply.message_id,'original');
});
test('an early live echo survives a lost HTTP response and never produces a duplicate',async()=>{
 let reject;const outbox=createOutbox({newId:()=> 'stable',transport:()=>new Promise((_,no)=>reject=no)});
 const job=outbox.enqueue({chatId:'a',text:'one'});await tick();const echo={id:'stable',chat_id:'a',from_me:1,type:'chat',body:'one',timestamp:1};
 outbox.observe(echo);assert.equal(outbox.merge('a',[echo]).length,1);reject(Error('lost response'));assert.equal(await job.done,true);assert.equal(outbox.list('a').length,0);
});
test('restored sends require explicit retry and stay isolated from other chats',async()=>{
 const stored={id:'old',chatId:'a',text:'retained',reply:{message_id:'r'},createdAt:1000,status:'sending'};let calls=0;
 const outbox=createOutbox({storage:{load:async()=>[stored],put:async()=>{},remove:async()=>{}},transport:async()=>{calls++;return{id:'old'}}});await outbox.ready;
 assert.equal(calls,0);assert.equal(outbox.merge('b',[]).length,0);const row=outbox.merge('a',[])[0];assert.equal(row.outbox.status,'failed');assert.equal(row.reply.message_id,'r');await outbox.retry('old');assert.equal(calls,1);
});
test('discarding queued work prevents later transport and removes retained content',async()=>{
 let release;const calls=[];const outbox=createOutbox({transport:async e=>{calls.push(e.text);await new Promise(done=>release=done);return{id:e.id}}});
 const one=outbox.enqueue({chatId:'a',text:'one'}),two=outbox.enqueue({chatId:'a',text:'two'});await tick();outbox.discardChat('a');release();await one.done;await two.done;assert.deepEqual(calls,['one']);assert.equal(outbox.list().length,0);
});

test('a remote close revision also invalidates snapshots restored after startup',async()=>{
 let load;const removed=[];const outbox=createOutbox({storage:{load:()=>new Promise(done=>load=done),put:async()=>{},remove:async id=>removed.push(id)},transport:async()=>{throw Error('must not send')}});
 outbox.invalidate('a',2);load([{id:'old',chatId:'a',text:'discarded',serverRevision:1,status:'failed',createdAt:1}]);await outbox.ready;assert.equal(outbox.list().length,0);assert.deepEqual(removed,['old']);
});
