import test from 'node:test';
import assert from 'node:assert/strict';
import {loadUnreadOpening} from '../frontend/src/services/chat-opening.js';
const messages=Array.from({length:305},(_,i)=>({id:`m${i}`,timestamp:i}));
test('first unread opening spans multiple pages without leaving gaps to the newest message',async()=>{
 const urls=[];
 const result=await loadUnreadOpening({chatId:'c',targetId:'m3',isCurrent:()=>true,apiJson:async url=>{
  urls.push(url);if(url.endsWith('/context'))return [messages[3]];
  const q=new URL(url,'http://test').searchParams;
  return messages.filter(m=>!q.has('beforeTs')||m.timestamp<Number(q.get('beforeTs'))).slice(-Number(q.get('limit')));
 }});
 assert.equal(result.targetId,'m3');assert.deepEqual(result.messages,messages);
 assert.equal(urls.length,4);assert.ok(urls.slice(2).every(url=>url.includes('localOnly=1')));
});
test('missing targets fall back to the newest page without paging all stored history',async()=>{
 let calls=0;
 const result=await loadUnreadOpening({chatId:'c',targetId:'gone',isCurrent:()=>true,apiJson:async url=>{calls++;if(url.endsWith('/context'))throw Object.assign(Error('gone'),{status:404});return messages.slice(-50)}});
 assert.equal(calls,2);assert.equal(result.targetId,null);assert.deepEqual(result.messages,messages.slice(-50));
});
test('a switched chat discards a late opening response and stops paging',async()=>{
 let current=true;
 const result=await loadUnreadOpening({chatId:'c',targetId:'m3',isCurrent:()=>current,apiJson:async url=>{current=false;return url.endsWith('/context')?[]:messages.slice(-50)}});
 assert.equal(result,null);
});
