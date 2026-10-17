import test from 'node:test';
import assert from 'node:assert/strict';
import { createDrafts } from '../frontend/src/core/drafts.js';
import { sameMessageGroup } from '../frontend/src/core/message-groups.js';

test('text/reply survive new sessions while transient state is not restored', () => {
  const values = new Map();
  const storage = { getItem: key => values.get(key), setItem: (key,value) => values.set(key,value) };
  const first = createDrafts({storage});
  first.update('a',{text:'Answer',reply:{message_id:'m1',body:'Question'},sending:true,error:'Failed'});
  first.update('b',{text:'Other'});
  const second = createDrafts({storage});
  assert.equal(second.get('a').text,'Answer');
  assert.equal(second.get('a').reply.message_id,'m1');
  assert.equal(second.get('a').sending,false);assert.equal(second.get('a').error,'');assert.equal(second.get('a').image,null);
  assert.equal(second.get('b').text,'Other');
  second.update('a',{text:'',reply:null});
  assert.equal(createDrafts({storage}).get('a').text,'');
});
test('blocked or corrupt storage does not break drafts', () => {
  for(const storage of [{getItem(){throw Error()},setItem(){throw Error()}},{getItem:()=>'{broken',setItem(){}}]) {
    const drafts=createDrafts({storage});drafts.update('a',{text:'Kept in memory'});assert.equal(drafts.get('a').text,'Kept in memory');
  }
});
test('message groups stop at sender, direction, date, deletion and five-minute boundaries', () => {
  const message={id:'a',chat_id:'chat',sender_id:'person',timestamp:1790760000,from_me:false};
  assert.ok(sameMessageGroup(message,{...message,id:'b',timestamp:message.timestamp+20}));
  for(const patch of [{sender_id:'other'},{from_me:true},{chat_id:'other'},{deleted:true},{timestamp:message.timestamp+301},{timestamp:message.timestamp-1}])assert.equal(sameMessageGroup(message,{...message,...patch}),false);
  assert.equal(sameMessageGroup(null,message),false);
});
