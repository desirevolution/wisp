import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
const root=resolve('public'),streams=new Set(),errors=[];
let mode='devices',readCalls=[],rows,states;
const emit=(type,data)=>{for(const s of streams)s.write(`event: ${type}\ndata: ${JSON.stringify(data)}\n\n`)};
function reset(){readCalls=[];rows={own:[{id:'mine',chat_id:'own',type:'chat',body:'Sent using Wisp',timestamp:20,from_me:1,reactions:[]}],history:[{id:'historical',chat_id:'history',type:'chat',body:'Already read in Wisp',timestamp:10,from_me:0,reactions:[]}]};states={history:{chat_id:'history',version:1,read_sequence:1,unread_sequence:1,last_incoming_id:'historical'}}}
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://test').pathname;
 const json=x=>{res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify(x))};
 if(path==='/api/events'){res.writeHead(200,{'content-type':'text/event-stream'});res.write(': ready\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return}
 if(path==='/api/config')return json({sendEnabled:true,readMode:mode});
 if(path==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true},push:{},chat_states:Object.values(states)});
 if(path==='/api/chats')return json(Object.entries(rows).map(([id,list])=>({id,name:id==='own'?'Own messages':'History',type:'private',last_message_id:list.at(-1).id,last_message_at:list.at(-1).timestamp})));
 if(path==='/api/push/public-key')return json({publicKey:''});
 const match=path.match(/^\/api\/chats\/(own|history)\/(messages|read)$/);
 if(match){const [,id,action]=match;if(action==='messages')return json(rows[id]);
  let body='';for await(const part of req)body+=part;const input=JSON.parse(body);readCalls.push({chat:id,...input});
  const p=states[id]||{chat_id:id,version:0,unread_sequence:0,read_sequence:0,last_incoming_id:''};
  p.read_sequence=Math.max(p.read_sequence,input.through_sequence);states[id]=p;return json(p);
 }
 if(path.startsWith('/api/'))return json([]);
 try{const file=resolve(root,'.'+(path==='/'?'/index.html':path));if(!file.startsWith(root+'/'))throw Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.webmanifest':'application/manifest+json'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 for(const width of [1200,390]){
  reset();mode='devices';const page=await browser.newPage({viewport:{width,height:844}});page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  const open=async id=>{if(width<701&&await page.locator('#app').evaluate(el=>el.classList.contains('open')))await page.locator('#back').click();await page.locator(`#chatList [data-id="${id}"]`).click()};
  await open('own');
  await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('chat-preparing'));
  await page.waitForTimeout(150);
  assert.deepEqual(readCalls,[{chat:'own',through_sequence:0,last_message_id:'mine'}]);
  const next={id:'mine-next',chat_id:'own',type:'chat',body:'Another sent message',timestamp:21,from_me:1,reactions:[]};rows.own.push(next);emit('message',next);
  await page.locator('[data-message-id="mine-next"]').waitFor();
  for(let i=0;i<100&&readCalls.length<2;i++)await new Promise(done=>setTimeout(done,10));
  assert.deepEqual(readCalls.at(-1),{chat:'own',through_sequence:0,last_message_id:'mine-next'});
  await open('history');
  await page.locator('[data-message-id="historical"]').waitFor();
  for(let i=0;i<100&&!readCalls.some(c=>c.chat==='history');i++)await new Promise(done=>setTimeout(done,10));
  assert.deepEqual(readCalls.find(c=>c.chat==='history'),{chat:'history',through_sequence:1,last_message_id:'historical'});
  // Reopening explicitly requests the same boundary, rather than leaving a
  // formerly broken native unread state stuck for the lifetime of the page.
  await open('own');
  await page.locator('[data-message-id="mine-next"]').waitFor();
  for(let i=0;i<100&&readCalls.filter(c=>c.chat==='own').length<3;i++)await new Promise(done=>setTimeout(done,10));
  assert.equal(readCalls.filter(c=>c.chat==='own').length,3);
  await page.close();
 }
 assert.deepEqual(errors,[]);console.log('Visible read boundaries browser passed: desktop/mobile own-only chats, already-read history, later own messages and explicit reopen.');
}finally{await browser?.close();for(const s of streams)s.end();server.closeAllConnections();await new Promise(done=>server.close(done))}
