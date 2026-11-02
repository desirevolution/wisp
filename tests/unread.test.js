import test from 'node:test';
import assert from 'node:assert/strict';
import {createReadTracker,unreadCount,closeReadNotifications,unreadTabTitle} from '../frontend/src/services/unread.js';

test('read tracker requires a visible prepared chat and its latest incoming row',async()=>{
 const state={selectedChatId:'c',messages:[],chatStates:new Map([['c',{chat_id:'c',unread_sequence:2,read_sequence:0,last_incoming_id:'last'}]])};
 let visible=false,calls=0,finish;
 const tracker=createReadTracker({getState:()=>state,canRead:()=>visible,onState:p=>state.chatStates.set(p.chat_id,p),apiJson:async(url,opt)=>{calls++;assert.equal(url,'/api/chats/c/read');assert.equal(JSON.parse(opt.body).through_sequence,2);return new Promise(resolve=>finish=resolve)}});
 await tracker.markVisible();assert.equal(calls,0);
 visible=true;await tracker.markVisible();assert.equal(calls,0);
 state.messages=[{id:'last'}];const pending=tracker.markVisible();await tracker.markVisible();assert.equal(calls,1);
 finish({...state.chatStates.get('c'),read_sequence:2,version:1});await pending;
 assert.equal(unreadCount(state.chatStates.get('c')),0);
});

test('a failed read retains counts and a subsequent open can retry',async()=>{
 const state={selectedChatId:'c',messages:[{id:'last'}],chatStates:new Map([['c',{chat_id:'c',unread_sequence:1,read_sequence:0,last_incoming_id:'last'}]])};
 let calls=0;
 const tracker=createReadTracker({getState:()=>state,canRead:()=>true,onState:p=>state.chatStates.set('c',p),apiJson:async()=>{if(++calls===1)throw Error('offline');return {...state.chatStates.get('c'),read_sequence:1,version:1}}});
 await tracker.markVisible();assert.equal(unreadCount(state.chatStates.get('c')),1);
 await tracker.markVisible();assert.equal(unreadCount(state.chatStates.get('c')),0);
 assert.equal(unreadCount({closed:true,unread_sequence:10}),0);
});

test('an authentication redirect or invalid success body cannot read or cause a request loop',async()=>{
 const state={selectedChatId:'c',messages:[{id:'last'}],chatStates:new Map([['c',{chat_id:'c',version:1,unread_sequence:1,read_sequence:0,last_incoming_id:'last'}]])};
 let calls=0,applied=0;
 const tracker=createReadTracker({getState:()=>state,canRead:()=>true,onState:()=>applied++,apiJson:async()=>{calls++;return null}});
 await tracker.markVisible();assert.equal(calls,1);assert.equal(applied,0);assert.equal(unreadCount(state.chatStates.get('c')),1);
});

test('read cleanup removes only covered message notifications, preserving tests and other chats',async()=>{
 const closed=[];
 const notifications=[{chatId:'c',messageId:'read'},{chatId:'c',messageId:'new'},{chatId:'other',messageId:'read'},{chatId:'c',messageId:'read',testId:'test'}].map((data,i)=>({data,close:()=>closed.push(i)}));
 const previous=Object.getOwnPropertyDescriptor(globalThis,'navigator');
 Object.defineProperty(globalThis,'navigator',{configurable:true,value:{serviceWorker:{getRegistration:async()=>({getNotifications:async()=>notifications})}}});
 try{await closeReadNotifications('c',new Set(['read']));assert.deepEqual(closed,[0])}finally{if(previous)Object.defineProperty(globalThis,'navigator',previous);else delete globalThis.navigator}
});


test('tab title sums unread messages across chats while excluding mute and closed states',()=>{
 const now=100000;
 const state={unread:new Map([['a',2],['b',1],['muted',8],['timed',3],['closed',4]]),chatStates:new Map([['muted',{mute_until:-1}],['timed',{mute_until:101}],['closed',{closed:true}]]),knownChats:new Map()};
 assert.equal(unreadTabTitle(state,now),'(3) Wisp');
 assert.equal(unreadTabTitle(state,102000),'(6) Wisp');
 state.unread.delete('a');state.unread.delete('b');state.unread.delete('timed');
 assert.equal(unreadTabTitle(state,now),'Wisp');
 // Shared preferences win over a stale list snapshot; fallback covers old APIs.
 state.knownChats.set('muted',{mute_until:0});
 assert.equal(unreadTabTitle(state,now),'Wisp');
 state.chatStates.set('muted',{mute_until:0});
 assert.equal(unreadTabTitle(state,now),'(8) Wisp');
 state.chatStates.delete('muted');state.knownChats.set('muted',{mute_until:-1});
 assert.equal(unreadTabTitle(state,now),'Wisp');
});


test('device sync acknowledges loaded own/history boundaries even with no local unread count',async()=>{
 const state={selectedChatId:'c',readMode:'devices',messages:[{id:'history',timestamp:1},{id:'mine',timestamp:2,from_me:1}],chatStates:new Map(),chats:[{id:'c',last_message_id:'mine'}]};
 let visible=false,rendered=false,calls=[];
 const tracker=createReadTracker({getState:()=>state,canRead:()=>visible,isRendered:()=>rendered,onState:p=>state.chatStates.set('c',p),apiJson:async(url,options)=>{calls.push(JSON.parse(options.body));return {chat_id:'c',unread_sequence:0,read_sequence:0,version:0}}});
 await tracker.markVisible();visible=true;await tracker.markVisible();assert.equal(calls.length,0);
 rendered=true;await tracker.markVisible();assert.deepEqual(calls,[{through_sequence:0,last_message_id:'mine'}]);
 await tracker.markVisible();assert.equal(calls.length,1,'no request loop on already-read state');
 state.messages.push({id:'new-own',timestamp:3,from_me:1});state.chats[0].last_message_id='new-own';
 await tracker.markVisible();assert.equal(calls.length,2);assert.equal(calls[1].last_message_id,'new-own');
 tracker.reset('c');await tracker.markVisible();assert.equal(calls.length,3,'explicit reopening repairs native unread state');
 state.readMode='wisp';await tracker.markVisible();assert.equal(calls.length,3);
});
test('device sync excludes unloaded latest rows and retries failed zero-unread synchronization',async()=>{
 const state={selectedChatId:'c',readMode:'devices',messages:[{id:'old',timestamp:1}],chatStates:new Map(),chats:[{id:'c',last_message_id:'latest'}]};
 let calls=0;
 const tracker=createReadTracker({getState:()=>state,canRead:()=>true,onState:p=>state.chatStates.set('c',p),apiJson:async()=>{calls++;if(calls===1)throw Error('offline');return {chat_id:'c',unread_sequence:0,read_sequence:0,version:0}}});
 await tracker.markVisible();assert.equal(calls,0);
 state.messages.push({id:'latest',timestamp:2});await tracker.markVisible();assert.equal(calls,1);
 await tracker.markVisible();assert.equal(calls,2);
 await tracker.markVisible();assert.equal(calls,2);
});
