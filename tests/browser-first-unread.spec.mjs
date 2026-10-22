import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
const root=resolve('public'),streams=new Set(),errors=[];
let pref,rows;
const emit=(type,data)=>{for(const res of streams)res.write(`event: ${type}\ndata: ${JSON.stringify(data)}\n\n`)};
function incoming(){
 const wasRead=pref.unread_sequence===pref.read_sequence;
 const id=`msg-${++pref.unread_sequence}`;pref.last_incoming_id=id;pref.version++;if(wasRead)pref.first_unread_id=id;
 rows.push({id,chat_id:'c',type:'chat',body:`Incoming message ${pref.unread_sequence}: text to read in the conversation.`,timestamp:pref.unread_sequence,from_me:0,reactions:[]});
}
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://local'),path=url.pathname;
 const json=(x,code=200)=>{res.writeHead(code,{'content-type':'application/json'});res.end(JSON.stringify(x))};
 if(path==='/api/events'){res.writeHead(200,{'content-type':'text/event-stream'});res.write(': ready\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return}
 if(path==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true},chat_states:[pref]});
 if(path==='/api/config')return json({sendEnabled:true});
 if(path==='/api/push/public-key')return json({publicKey:''});
 if(path==='/api/chats')return json([{id:'c',name:'Family',type:'group',last_message_at:pref.unread_sequence}]);
 if(path==='/api/chats/c/messages'){
  const before=url.searchParams.get('beforeTs');return json(rows.filter(m=>before===null||m.timestamp<Number(before)).slice(-Number(url.searchParams.get('limit')||50)));
 }
 if(path.endsWith('/context')){const id=decodeURIComponent(path.split('/')[5]);return rows.some(m=>m.id===id)?json(rows.filter(m=>m.id===id)):json({error:'missing'},404)}
 if(path==='/api/chats/c/read'){
  let body='';for await(const b of req)body+=b;const through=JSON.parse(body).through_sequence;
  pref.read_sequence=Math.max(pref.read_sequence,through);pref.first_unread_id=rows.find(m=>m.timestamp>pref.read_sequence)?.id||'';pref.version++;emit('unread',{...pref});return json(pref);
 }
 if(path.startsWith('/api/'))return json([]);
 try{const file=resolve(root,'.'+(path==='/'?'/index.html':path));if(!file.startsWith(root+'/'))throw Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.webmanifest':'application/manifest+json'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 for(const [width,reduced] of [[1200,false],[390,false],[390,true]]){
  pref={chat_id:'c',closed:false,mute_until:0,draft_revision:0,version:0,unread_sequence:0,read_sequence:0,last_incoming_id:'',first_unread_id:''};rows=[];
  for(let i=0;i<75;i++)incoming();
  const context=await browser.newContext({viewport:{width,height:844},isMobile:width<700,hasTouch:width<700,reducedMotion:reduced?'reduce':'no-preference'});context.setDefaultTimeout(10000);
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await page.goto(`http://127.0.0.1:${server.address().port}`);
  await page.locator('.unread-count').filter({hasText:'75'}).waitFor();
  const open=()=>page.locator('#chatList [data-id="c"]').click();
  const highlighted=async id=>{
   const target=page.locator(`[data-message-id="${id}"].highlight`);await target.waitFor();
   const box=await target.boundingBox(),pane=await page.locator('#messages').boundingBox();
   assert.ok(box.y+box.height>pane.y&&box.y<pane.y+pane.height,JSON.stringify({id,box,pane}));
   if(reduced)assert.equal(await target.evaluate(el=>getComputedStyle(el).animationName),'none');
  };
  await open();await highlighted('msg-1');await page.waitForFunction(()=>!document.querySelector('.unread-count'));
  assert.equal(await page.locator('#messages [data-message-id]').count(),75);assert.equal(pref.read_sequence,75);
  await page.waitForFunction(()=>!document.querySelector('.highlight'));
  await page.goBack();await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('open'));
  for(let i=0;i<74;i++)incoming();emit('unread',{...pref});
  await page.locator('.unread-count').filter({hasText:'74'}).waitFor();await open();await highlighted('msg-76');
  await page.waitForFunction(()=>!document.querySelector('.unread-count'));assert.equal(pref.read_sequence,149);
  // A stale anchor (e.g. pruned message) falls back to a usable latest page.
  await page.goBack();await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('open'));
  incoming();pref.first_unread_id='pruned';emit('unread',{...pref});await page.locator('.unread-count').waitFor();await open();
  await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('chat-preparing')&&!document.querySelector('.unread-count'));
  assert.equal(await page.locator('#messages [data-message-id]').count(),50);assert.equal(await page.locator('.highlight').count(),0);
  await context.close();
 }
 assert.deepEqual(errors,[]);console.log('First unread browser passed: desktop/mobile and reduced motion, 75-message opening, contiguous range, cached reopen, highlight removal and missing target fallback.');
}finally{await browser?.close();for(const res of streams)res.end();server.closeAllConnections();await new Promise(done=>server.close(done))}
