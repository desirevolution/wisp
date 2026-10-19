// Two independent browser devices against a deterministic chat-state API/SSE fixture.
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.WISP_PLAYWRIGHT_MODULE || 'playwright');
const root=resolve('public'),chat='family@g.us',other='other@g.us',contact='49123@s.whatsapp.net';
const chats=new Map([[chat,{id:chat,name:'Family',type:'group',last_message_at:1790760000}],[other,{id:other,name:'Work',type:'group',last_message_at:1790760000}]]);
const prefs=new Map(),streams=new Set(),errors=[];
let failOptions=false,sequence=0,optionRequests=0;
const rows=new Map([[chat,[{id:'original',chat_id:chat,sender_name:'Bea',type:'chat',body:'Hello family',timestamp:1790760000,from_me:0,reactions:[]}]]]);
const preference=id=>prefs.get(id)||{chat_id:id,closed:false,mute_until:0,draft_revision:0,version:0};
function change(id,action,duration){
 const p={...preference(id),version:preference(id).version+1};
 if(action==='close'){p.closed=true;p.draft_revision++}
 if(action==='open'){p.closed=false}
 if(action==='mute')p.mute_until=duration==='forever'?-1:duration==='off'?0:Math.floor(Date.now()/1000)+(duration==='8h'?28800:604800);
 prefs.set(id,p);emit('chat-state',p);return p;
}
function emit(type,data){for(const res of streams)res.write(`event: ${type}\ndata: ${JSON.stringify(data)}\n\n`)}
async function incoming(){
 const p=preference(chat),muted=p.mute_until===-1||p.mute_until>Date.now()/1000;
 if(p.closed&&!muted)change(chat,'open');
 const m={id:`new-${++sequence}`,chat_id:chat,sender_name:'Bea',type:'chat',body:`New message ${sequence}`,timestamp:1790760000+sequence,from_me:0,reactions:[],silent:muted};
 rows.get(chat).push(m);Object.assign(chats.get(chat),{last_message_at:m.timestamp,last_body:m.body});emit('message',m);
}
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://local');const json=(data,code=200)=>{res.writeHead(code,{'content-type':'application/json'});res.end(JSON.stringify(data))};
 if(url.pathname==='/api/events'){res.writeHead(200,{'content-type':'text/event-stream','cache-control':'no-cache'});res.write(': connected\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return}
 if(url.pathname==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true},chat_states:[...prefs.values()]});
 if(url.pathname==='/api/config')return json({sendEnabled:true,mirrorOnly:false});
 if(url.pathname==='/api/push/public-key')return json({publicKey:''});
 if(url.pathname==='/api/chats')return json([...chats.values()].filter(c=>!preference(c.id).closed).map(c=>({...c,mute_until:preference(c.id).mute_until})));
 if(url.pathname==='/api/chat-options'){
  optionRequests++;
  if(failOptions){failOptions=false;return json({error:'Contact list unavailable'},503)}
  const q=(url.searchParams.get('q')||'').toLowerCase();
  return json([...chats.values()].map(c=>({...c,...preference(c.id)})).concat(chats.has(contact)?[]:[{id:contact,name:'Chris',type:'private',contact:true}]).filter(c=>c.name.toLowerCase().includes(q)));
 }
 if(url.pathname==='/api/search'){
  const q=(url.searchParams.get('q')||'').toLowerCase();return json({chats:[...chats.values()].filter(c=>!preference(c.id).closed&&c.name.toLowerCase().includes(q)),messages:[]});
 }
 const parts=decodeURIComponent(url.pathname).split('/');
 if(parts[1]==='api'&&parts[2]==='chats'){
  const id=parts[3];
  if(parts[4]==='state'){
   let raw='';for await(const b of req)raw+=b;const {action,mute_for}=JSON.parse(raw);
   if(id===contact&&!chats.has(id))chats.set(id,{id,name:'Chris',type:'private',last_message_at:0});
   return json(change(id,action,mute_for));
  }
  if(parts[4]==='messages')return json(rows.get(id)||[]);
  if(['reactions','edits','deletions','media','links'].includes(parts[4]))return json([]);
 }
 try{const file=resolve(root,'.'+(url.pathname==='/'?'/index.html':url.pathname));if(!file.startsWith(root+'/'))throw Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png','.webmanifest':'application/manifest+json'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
const base=`http://127.0.0.1:${server.address().port}`;
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 const desktop=await browser.newContext({viewport:{width:1200,height:850}}),mobile=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true});
 desktop.setDefaultTimeout(10000);mobile.setDefaultTimeout(10000);
 const a=await desktop.newPage();let b=await mobile.newPage();
 for(const page of [a,b]){page.on('pageerror',e=>errors.push(e.message));await page.goto(base);await page.locator(`#chatList [data-id="${chat}"]`).click();await page.locator('#composerInput').waitFor()}
 const menu=async(page)=>{await page.getByRole('button',{name:'Chat menu',exact:true}).click();await page.locator('dialog[open]').waitFor()};
 const closeChat=async(page)=>{await menu(page);await page.getByRole('button',{name:'Close chat…',exact:true}).click();await page.getByRole('button',{name:'Close chat',exact:true}).click();await page.waitForFunction(()=>!document.querySelector('dialog[open]'));await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('open'))};
 const choose=async(page,name)=>{await page.getByRole('button',{name:'New chat',exact:true}).click();await page.getByRole('searchbox',{name:'Search chats and contacts'}).fill(name);await page.locator('.chat-option').filter({hasText:name}).click();await page.locator('#composerInput').waitFor();await page.waitForFunction(()=>document.querySelector('#chatPicker').hidden)};
 // Desktop actions are anchored, non-modal and support keyboard and outside dismissal.
 await menu(a);
 const popup=await a.locator('dialog[open]').boundingBox(),anchor=await a.locator('#chatMenuBtn').boundingBox();
 assert.ok(Math.abs(popup.x+popup.width-anchor.x-anchor.width)<2&&popup.y>=anchor.y+anchor.height&&popup.y<anchor.y+anchor.height+12,JSON.stringify({popup,anchor}));
 assert.equal(await a.locator('dialog').evaluate(el=>el.matches(':modal')),false);
 assert.equal(await a.locator('#chatMenuBtn').getAttribute('aria-expanded'),'true');
 await a.waitForFunction(()=>document.activeElement?.matches('.chat-menu-actions button'));await a.keyboard.press('ArrowDown');assert.equal(await a.evaluate(()=>document.activeElement.textContent),'Close chat…');
 await a.keyboard.press('Home');assert.equal(await a.evaluate(()=>document.activeElement.textContent),'Mute chat…');
 await a.keyboard.press('Enter');await a.getByRole('button',{name:'8 hours',exact:true}).waitFor();
 assert.equal(await a.locator('dialog').evaluate(el=>el.matches(':modal')),false);
 await a.getByRole('button',{name:'Back to chat menu'}).click();await a.getByRole('button',{name:'Close chat…',exact:true}).click();
 assert.equal(await a.locator('dialog').evaluate(el=>el.matches(':modal')),true);
 await a.keyboard.press('Escape');await a.waitForFunction(()=>!document.querySelector('dialog[open]'));
 await menu(a);await a.locator('#composerInput').click();await a.waitForFunction(()=>!document.querySelector('dialog[open]'));
 assert.equal(await a.locator('#chatMenuBtn').getAttribute('aria-expanded'),'false');
 await menu(a);await a.setViewportSize({width:390,height:850});await a.waitForFunction(()=>document.querySelector('dialog')?.matches(':modal'));
 await a.setViewportSize({width:1200,height:850});await a.waitForFunction(()=>!document.querySelector('dialog')?.matches(':modal'));
 if(process.env.WISP_CHAT_QA)await a.screenshot({path:process.env.WISP_CHAT_QA.replace('.png','-menu.png')});
 await a.keyboard.press('Escape');await a.waitForFunction(()=>!document.querySelector('dialog[open]'));
 // Browser Back dismisses the dialog before leaving the chat.
 await menu(a);await a.goBack();await a.waitForFunction(()=>!document.querySelector('dialog[open]'));assert.equal(await a.locator('#conversationTitle').textContent(),'Family');
 await a.goForward();await a.locator('dialog[open]').waitFor();await a.reload();await a.locator('dialog[open]').waitFor();await a.goBack();await a.waitForFunction(()=>!document.querySelector('dialog[open]'));assert.equal(await a.locator('#conversationTitle').textContent(),'Family');
 // Search filters one loaded snapshot without removing the list or refetching.
 const beforeSearch=optionRequests;
 await a.getByRole('button',{name:'New chat',exact:true}).click();await a.locator('.chat-option').first().waitFor();
 assert.equal(await a.locator('dialog[open]').count(),0);assert.ok(await a.locator('.conversation').isVisible());assert.ok(await a.locator('#chatList').isHidden());
 const panelBox=await a.locator('#chatPicker').boundingBox(),sidebarBox=await a.locator('.sidebar').boundingBox();assert.ok(Math.abs(panelBox.x-sidebarBox.x)<1&&panelBox.width<=sidebarBox.width);
 await a.locator('#composerInput').fill('Right-hand chat stays usable');
 if(process.env.WISP_CHAT_QA)await a.screenshot({path:process.env.WISP_CHAT_QA.replace('.png','-desktop.png')});
 await a.locator('#composerInput').press('Control+f');await a.waitForFunction(()=>document.activeElement?.matches('#chatPicker input'));
 await a.evaluate(()=>{window.originalOptionsList=document.querySelector('.chat-options')});
 for(const query of ['F','Fa','Fam','Fami','Famil','Family','Work','Ch']){
  await a.getByRole('searchbox',{name:'Search chats and contacts'}).fill(query);
  const result=await a.evaluate(()=>({same:document.querySelector('.chat-options')===window.originalOptionsList,loading:document.querySelector('.chat-picker-results [role="status"]')?.textContent,count:document.querySelectorAll('.chat-option').length}));
  assert.ok(result.same&&!result.loading&&result.count>0,JSON.stringify({query,...result}));
 }
 await a.getByRole('searchbox',{name:'Search chats and contacts'}).fill('Nobody matches');await a.getByText('No matching chats or WhatsApp contacts.',{exact:true}).waitFor();
 await a.getByRole('searchbox',{name:'Search chats and contacts'}).fill('');assert.equal(await a.locator('.chat-option').count(),3);
 assert.equal(optionRequests,beforeSearch+1,'Typing must not send additional contact requests');
 await a.setViewportSize({width:390,height:850});assert.ok(await a.locator('.conversation').isHidden());assert.ok(await a.locator('#chatPicker').isVisible());
 const fullPage=await a.locator('#chatPicker').boundingBox();assert.ok(fullPage.x>=0&&fullPage.width<=390&&fullPage.height>800,JSON.stringify(fullPage));
 await a.setViewportSize({width:1200,height:850});assert.ok(await a.locator('.conversation').isVisible());
 await a.getByRole('searchbox',{name:'Search chats and contacts'}).press('Escape');await a.waitForFunction(()=>document.querySelector('#chatPicker').hidden);
 assert.equal(await a.locator('#composerInput').inputValue(),'Right-hand chat stays usable');

 // Mute duration and state are shared; UI remains usable on the other device.
 await menu(a);await a.getByRole('button',{name:'Mute chat…',exact:true}).click();await a.getByRole('button',{name:'Indefinitely',exact:true}).click();
 await a.waitForFunction(()=>!document.querySelector('dialog[open]'));await b.locator('#headerMuted:not([hidden])').waitFor();
 assert.equal(preference(chat).mute_until,-1);
 const headerIcon=await b.locator('#headerMuted').boundingBox(),headerName=await b.locator('#conversationTitle').boundingBox();
 assert.ok(headerIcon.x>=headerName.x+headerName.width&&headerIcon.x-headerName.x-headerName.width<=10,JSON.stringify({headerIcon,headerName}));

 await a.locator('#composerInput').fill('Desktop draft');await b.locator('#composerInput').fill('Mobile draft');
 await b.evaluate(()=>document.querySelector('#composer').setReply({id:'original',type:'chat',body:'Hello family',sender_name:'Bea'}));await b.locator('.reply-draft').waitFor();
 await b.evaluate(()=>{const bytes=Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aV00AAAAASUVORK5CYII='),c=>c.charCodeAt(0));document.querySelector('#composer')._attachImage(new File([bytes],'draft.png',{type:'image/png'}))});await b.locator('.image-draft').waitFor();
 await menu(a);await a.getByRole('button',{name:'Close chat…',exact:true}).click();await a.getByRole('button',{name:'Cancel',exact:true}).click();await a.waitForFunction(()=>!document.querySelector('dialog[open]'));
 assert.equal(await a.locator('#composerInput').inputValue(),'Desktop draft');assert.equal(preference(chat).closed,false);
 await closeChat(a);
 await b.waitForFunction(()=>document.querySelector('#composerInput').value==='');
 assert.equal(await b.locator('.reply-draft').count(),0);assert.equal(await b.locator('.image-draft').count(),0);assert.ok(await b.locator('#composerInput').isVisible());assert.equal(await b.locator('#conversationTitle').textContent(),'Family');
 assert.equal(await a.locator(`#chatList [data-id="${chat}"]`).count(),0);
 await a.locator('#search').fill('Family');await a.getByText('No results.',{exact:true}).waitFor();await a.locator('#search').fill('');
 await incoming();await b.getByText('New message 1',{exact:true}).waitFor();assert.equal(preference(chat).closed,true);assert.equal(await a.locator(`#chatList [data-id="${chat}"]`).count(),0);
 // Manual reopening keeps mute; a muted open chat still increments unread.
 await choose(a,'Family');assert.equal(preference(chat).closed,false);assert.equal(preference(chat).mute_until,-1);
 await b.goBack();await b.waitForFunction(()=>!document.querySelector('#app').classList.contains('open'));
 await incoming();await b.locator(`#chatList [data-id="${chat}"] .unread-count`).waitFor();assert.equal(await b.locator(`#chatList [data-id="${chat}"] .unread-count`).textContent(),'1');
 const listName=await b.locator(`#chatList [data-id="${chat}"] .chat-name-text`).boundingBox(),listIcon=await b.locator(`#chatList [data-id="${chat}"] .chat-muted`).boundingBox();
 assert.ok(listIcon.x>=listName.x+listName.width&&listIcon.x-listName.x-listName.width<=12,JSON.stringify({listName,listIcon}));

 // Unmuting a closed chat doesn't reopen it until the next message.
 await closeChat(a);
 const expired={...preference(chat),mute_until:Math.floor(Date.now()/1000)-1,version:preference(chat).version+1};prefs.set(chat,expired);emit('chat-state',expired);
 await a.waitForTimeout(80);assert.equal(await a.locator(`#chatList [data-id="${chat}"]`).count(),0);
 await incoming();await a.locator(`#chatList [data-id="${chat}"]`).waitFor();assert.equal(preference(chat).closed,false);
 // A browser that was offline during close loses its stale draft on next startup.
 await b.locator(`#chatList [data-id="${chat}"]`).click();await b.locator('#composerInput').fill('Offline stale draft');await b.close();
 await a.locator(`#chatList [data-id="${chat}"]`).click();await closeChat(a);
 b=await mobile.newPage();b.on('pageerror',e=>errors.push(e.message));await b.goto(base);await b.locator('#newChatBtn').waitFor();
 await choose(b,'Family');assert.equal(await b.locator('#composerInput').inputValue(),'');
 await b.locator('#composerInput').fill('Keep this fresh draft');await b.reload();await b.locator('#composerInput').waitFor();assert.equal(await b.locator('#composerInput').inputValue(),'Keep this fresh draft');
 // New contacts enter the chat list immediately without sending a message.
 failOptions=true;await a.getByRole('button',{name:'New chat',exact:true}).click();await a.getByText('Contact list unavailable',{exact:true}).waitFor();await a.getByRole('button',{name:'Try again',exact:true}).click();
 await a.getByRole('searchbox',{name:'Search chats and contacts'}).fill('Chris');await a.locator('.chat-option').filter({hasText:'Chris'}).click();await a.waitForFunction(()=>document.querySelector('#conversationTitle').textContent==='Chris'&&document.querySelector('#chatPicker').hidden);
 assert.ok(chats.has(contact));assert.equal(await a.locator(`#chatList [data-id="${contact}"]`).count(),1);assert.equal(await a.locator('[data-message-id]').count(),0);
 // Mobile chat menu, duration picker and close confirmation are bottom sheets.
 const sheet=async(height=844)=>{
  const box=await b.locator('dialog[open]').boundingBox();
  assert.ok(Math.abs(box.x)<1&&Math.abs(box.width-390)<1&&Math.abs(box.y+box.height-height)<2,JSON.stringify({height,box}));
 };
 await menu(b);await sheet();await b.getByRole('button',{name:'Mute chat…',exact:true}).click();await sheet();
 await b.locator('dialog[open]').press('Escape');await b.waitForFunction(()=>!document.querySelector('dialog[open]'));
 await menu(b);await b.getByRole('button',{name:'Close chat…',exact:true}).click();await sheet();
 await b.evaluate(()=>{const app=document.querySelector('#app');app.style.setProperty('--app-viewport-height','420px');app.style.setProperty('--app-viewport-top','0px')});
 await b.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));await sheet(420);
 if(process.env.WISP_CHAT_QA)await b.screenshot({path:process.env.WISP_CHAT_QA.replace('.png','-mobile.png')});
 await b.getByRole('button',{name:'Cancel',exact:true}).click();await b.waitForFunction(()=>!document.querySelector('dialog[open]'));
 await b.evaluate(()=>{document.querySelector('#app').style.removeProperty('--app-viewport-height')});
 // Mobile Back/Forward/reload restore a page, and selecting consumes its entry.
 await b.getByRole('button',{name:'Back to chats',exact:true}).click();await b.locator('#chatList').waitFor();
 await b.getByRole('button',{name:'New chat',exact:true}).click();await b.locator('#chatPicker').waitFor();assert.equal(await b.locator('dialog[open]').count(),0);
 await b.waitForFunction(()=>document.activeElement?.classList.contains('picker-back'));
 await b.goBack();await b.locator('#chatList').waitFor();assert.ok(await b.locator('#chatPicker').isHidden());
 await b.goForward();await b.locator('#chatPicker').waitFor();await b.reload();await b.locator('#chatPicker').waitFor();
 if(process.env.WISP_CHAT_QA){await b.locator('.chat-option').first().waitFor();await b.screenshot({path:process.env.WISP_CHAT_QA.replace('.png','-picker-mobile.png')})}
 await b.getByRole('searchbox',{name:'Search chats and contacts'}).fill('Family');await b.locator('.chat-option').filter({hasText:'Family'}).waitFor();await b.getByRole('searchbox',{name:'Search chats and contacts'}).press('Enter');await b.locator('#composerInput').waitFor();
 await b.goBack();await b.locator('#chatList').waitFor();assert.ok(await b.locator('#chatPicker').isHidden());
 // Desktop search supports arrows + Enter and preserves focus on cancellation.
 await a.getByRole('button',{name:'New chat',exact:true}).click();await a.getByRole('searchbox',{name:'Search chats and contacts'}).fill('Work');await a.locator('.chat-option').filter({hasText:'Work'}).waitFor();
 await a.getByRole('searchbox',{name:'Search chats and contacts'}).press('ArrowDown');await a.waitForFunction(()=>document.activeElement?.classList.contains('chat-option'));await a.keyboard.press('Enter');
 await a.waitForFunction(()=>document.querySelector('#conversationTitle').textContent==='Work'&&document.querySelector('#chatPicker').hidden);
 await a.goBack();await a.locator('#chatList').waitFor();assert.ok(await a.locator('#chatPicker').isHidden());
 await a.locator('#search').fill('Family');await a.locator('.search-chat').waitFor();
 await a.getByRole('button',{name:'New chat',exact:true}).click();await a.locator('.chat-option').first().waitFor();await a.getByRole('button',{name:'Back to chat list',exact:true}).click();
 await a.locator('#chatList').waitFor();assert.equal(await a.locator('#search').inputValue(),'Family');await a.waitForFunction(()=>document.activeElement?.id==='newChatBtn');
 await a.locator('#search').fill('');
 if(process.env.WISP_CHAT_QA){await a.getByRole('button',{name:'New chat',exact:true}).click();await a.locator('.chat-option').first().waitFor();await a.screenshot({path:process.env.WISP_CHAT_QA})}
 assert.deepEqual(errors,[]);
 await desktop.close();await mobile.close();
 console.log('Chat controls: two-device close/mute, confirmation, remote/offline draft deletion, unread counts, hidden search, expiration, chooser/new contact, nonmodal desktop picker/mobile page, keyboard selection, instant local search, mobile mute icons/bottom sheets, retry, Escape and Browser Back passed');
}finally{await browser?.close();for(const res of streams)res.end();await new Promise(done=>server.close(done))}
