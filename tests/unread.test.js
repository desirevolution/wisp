import test from 'node:test';
import assert from 'node:assert/strict';
import {createReadTracker,unreadCount,closeReadNotifications} from '../frontend/src/services/unread.js';

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
