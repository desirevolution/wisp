import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
const root=resolve('public'),streams=new Set(),errors=[];
let pref,rows,readCalls=0,holdRead=false,releaseRead;
const emit=(type,data)=>{for(const res of streams)res.write(`event: ${type}\ndata: ${JSON.stringify(data)}\n\n`)};
function incoming({live=true}={}){
 const id=`msg-${++pref.unread_sequence}`;pref.last_incoming_id=id;pref.version++;
 const m={id,chat_id:'c',type:'chat',body:`Incoming ${id}`,timestamp:pref.unread_sequence,from_me:0,reactions:[]};rows.push(m);
 if(live){emit('message',m);emit('unread',{...pref})}
}
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://local').pathname;
 const json=(x,code=200)=>{res.writeHead(code,{'content-type':'application/json'});res.end(JSON.stringify(x))};
 if(path==='/api/events'){res.writeHead(200,{'content-type':'text/event-stream'});res.write(': ready\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return}
 if(path==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true},chat_states:[pref]});
 if(path==='/api/config')return json({sendEnabled:true});
 if(path==='/api/push/public-key')return json({publicKey:''});
 if(path==='/api/chats')return json([{id:'c',name:'Family',type:'group',last_message_at:pref.unread_sequence,last_body:'A message'}]);
 if(path==='/api/chats/c/messages')return json(rows);
 if(path==='/api/chats/c/read'){
  readCalls++;let body='';for await(const b of req)body+=b;const through=JSON.parse(body).through_sequence;
  if(holdRead){holdRead=false;await new Promise(done=>releaseRead=done)}
  pref.read_sequence=Math.max(pref.read_sequence,through);pref.version++;emit('unread',{...pref});return json(pref);
 }
 if(path.startsWith('/api/'))return json([]);
 try{const file=resolve(root,'.'+(path==='/'?'/index.html':path));if(!file.startsWith(root+'/'))throw Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.webmanifest':'application/manifest+json'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 for(const width of [1200,390]){
  pref={chat_id:'c',closed:false,mute_until:0,draft_revision:0,version:0,unread_sequence:0,read_sequence:0,last_incoming_id:''};rows=[];readCalls=0;
  incoming({live:false});incoming({live:false});
  const context=await browser.newContext({viewport:{width,height:844},isMobile:width<700,hasTouch:width<700});context.setDefaultTimeout(10000);
  await context.addInitScript(()=>Object.defineProperty(document,'visibilityState',{get:()=>window.qaHidden?'hidden':'visible'}));
  const a=await context.newPage(),b=await context.newPage();
  for(const page of [a,b]){page.on('pageerror',e=>errors.push(e.message));await page.goto(`http://127.0.0.1:${server.address().port}`);await page.locator('.unread-count').filter({hasText:'2'}).waitFor();assert.equal(await page.title(),'(2) Wisp')}
  await a.reload();await a.locator('.unread-count').filter({hasText:'2'}).waitFor();assert.equal(readCalls,0);assert.equal(await a.title(),'(2) Wisp');
  // A delayed read acknowledges only its snapshot, while a new arrival survives.
  holdRead=true;releaseRead=null;await a.locator('#chatList [data-id="c"]').click();
  await a.waitForFunction(()=>!document.querySelector('#app').classList.contains('chat-preparing'));
  for(let i=0;i<100&&!releaseRead;i++)await new Promise(done=>setTimeout(done,10));assert.ok(releaseRead);
  await a.evaluate(()=>{window.qaHidden=true});incoming();releaseRead();releaseRead=null;
  await b.locator('.unread-count').filter({hasText:'1'}).waitFor();assert.equal(pref.read_sequence,2);assert.equal(await b.title(),'(1) Wisp');
  // Resume refreshes a missed message before reading it, then synchronizes device B.
  incoming({live:false});
  await a.evaluate(()=>{window.qaHidden=false;document.dispatchEvent(new Event('visibilitychange'))});
  await a.locator('.message-text').filter({hasText:'Incoming msg-4'}).waitFor();
  await b.waitForFunction(()=>!document.querySelector('.unread-count'));assert.equal(pref.read_sequence,4);assert.equal(await b.title(),'Wisp');assert.equal(await a.title(),'Wisp');
  await b.reload();await b.locator('#chatList [data-id="c"]').waitFor();assert.equal(await b.locator('.unread-count').count(),0);
  // A background chat updates the title, while mute keeps only the list badge.
  await a.evaluate(()=>{window.qaHidden=true});incoming();
  await b.waitForFunction(()=>document.title==='(1) Wisp');
  await a.waitForFunction(()=>document.title==='(1) Wisp');
  pref.mute_until=-1;pref.version++;emit('chat-state',{...pref});
  await b.waitForFunction(()=>document.title==='Wisp');
  await b.locator('.unread-count').filter({hasText:'1'}).waitFor();
  // Expiration must refresh the title without another incoming message.
  pref.mute_until=Math.floor(Date.now()/1000)+2;pref.version++;emit('chat-state',{...pref});
  await b.waitForFunction(()=>document.title==='(1) Wisp');
  pref.closed=true;pref.version++;emit('chat-state',{...pref});
  await b.waitForFunction(()=>document.title==='Wisp');
  await a.waitForFunction(()=>document.title==='Wisp');
  await context.close();
 }
 assert.deepEqual(errors,[]);console.log('Unread browser passed: desktop/mobile overview reload, shared device state, delayed read, background arrival and missed-event resume, tab title counts, background updates, mute expiration and remote close.');
}finally{releaseRead?.();await browser?.close();for(const res of streams)res.end();server.closeAllConnections();await new Promise(done=>server.close(done))}
