import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.WISP_PLAYWRIGHT_MODULE||'playwright');
let imageGate=null;
const root=resolve('public'),chat='outbox@g.us',other='other@g.us',rows=new Map(),attempts=[];let gate=null,fail=false;
const waitAttempts=async count=>{const deadline=Date.now()+10000;while(attempts.length<count){assert.ok(Date.now()<deadline,'Send request did not arrive');await new Promise(done=>setTimeout(done,5))}};
const initial=id=>[{id:'original-'+id,chat_id:id,body:'Original message',sender_name:'Bea',type:'chat',timestamp:1790760000,from_me:0,reactions:[]}];
const messages=id=>[...initial(id),...[...rows.values()].filter(m=>m.chat_id===id)];
const server=createServer(async(req,res)=>{
 const u=new URL(req.url,'http://fixture'),json=(data,status=200)=>{if(res.destroyed)return;res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(data))};
 if(u.pathname==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true}});
 if(u.pathname==='/api/config')return json({sendEnabled:true,mirrorOnly:false});
 if(u.pathname==='/api/chats')return json([{id:chat,name:'Outbox',type:'group'},{id:other,name:'Other',type:'group'}]);
 if(u.pathname.startsWith('/api/media/')){if(imageGate)await imageGate;res.writeHead(200,{'content-type':'image/png'});res.end(await readFile('tests/fixtures/pixel.png'));return}
 if(u.pathname.startsWith('/api/chats/')){
  const parts=decodeURIComponent(u.pathname).split('/'),id=parts[3];
  if(parts[4]==='send'||parts[4]==='send-image'){
   let raw='';for await(const chunk of req)raw+=chunk;
   const client=req.headers['x-wisp-message-id'],isImage=parts[4]==='send-image',payload=isImage?{text:raw.match(/name="caption"\r\n\r\n([^]*?)\r\n--/)?.[1]||'',reply_to:raw.match(/name="reply_to"\r\n\r\n([^]*?)\r\n--/)?.[1]||''}:JSON.parse(raw);
   attempts.push({client,chat:id,text:payload.text,image:isImage});
   if(fail)return json({error:'Provider unavailable'},502);
   const outgoing={id:client,chat_id:id,from_me:1,type:isImage?'image':'chat',body:payload.text,timestamp:Math.floor(Date.now()/1000),has_media:isImage?1:0,media_id:isImage?client:null,media_status:isImage?'ready':null,media_width:1,media_height:1,reply:payload.reply_to?{message_id:payload.reply_to,sender_name:'Bea',body:'Original message',type:'chat'}:null,reactions:[]};
   if(!isImage&&!rows.has(client))rows.set(client,outgoing);
   const waiting=gate;if(waiting)await waiting;
   if(!rows.has(client))rows.set(client,outgoing);
   return json({ok:true,id:client});
  }
  if(parts[4]==='messages')return json(messages(id));
 }
 if(u.pathname.startsWith('/api/'))return json([]);
 try{const path=resolve(root,'.'+(u.pathname==='/'?'/index.html':u.pathname));if(!path.startsWith(root+'/'))throw Error();res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml'}[extname(path)]||'application/octet-stream')});res.end(await readFile(path))}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));const base=`http://127.0.0.1:${server.address().port}`;let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 for(const width of [1200,390]){
  rows.clear();attempts.length=0;gate=null;fail=false;
  const context=await browser.newContext({viewport:{width,height:850},isMobile:width<701,hasTouch:width<701,serviceWorkers:'block'}),page=await context.newPage(),errors=[];
  page.setDefaultTimeout(10000);page.on('pageerror',e=>errors.push(e.message));await context.addInitScript(()=>{window.EventSource=class extends EventTarget{constructor(){super();window.testEvents=this}close(){}}});
  await page.goto(base);await page.locator(`#chatList [data-id="${chat}"]`).click();await page.locator('#composerInput').waitFor();await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('chat-preparing')&&!document.querySelector('#app').classList.contains('chat-revealing'));
  await page.locator(`[data-message-id="original-${chat}"]`).focus();await page.keyboard.press('Shift+F10');
  if(width<701)await page.getByRole('button',{name:'More message actions',exact:true}).click();await page.getByRole('button',{name:'Reply',exact:true}).click();
  let release;gate=new Promise(done=>release=done);
  await page.locator('#composerInput').fill('First optimistic message');
  if(width>700)await page.locator('#composerInput').press('Enter');else await page.locator('#sendBtn').tap();
  await page.locator('.outbox-status').waitFor();assert.equal(await page.locator('#composerInput').inputValue(),'');assert.equal(await page.locator('#composerInput').isDisabled(),false);assert.equal(await page.evaluate(()=>document.activeElement.id),'composerInput');assert.equal(await page.locator('.reply-draft').count(),0);
  await page.waitForFunction(()=>document.querySelector('.outbox-status').textContent.includes('Sending'));
  await waitAttempts(1);const first=attempts.at(-1);assert.match(first.client,/^3EB0[0-9A-F]{18}$/);
  assert.ok((await page.locator(`[data-message-id="${first.client}"] .quoted-message`).textContent()).includes('Original message'));
  await page.locator('#composerInput').fill('Second optimistic message');await page.locator('#sendBtn').click();await page.getByText('Waiting to send…',{exact:true}).waitFor();assert.equal(attempts.length,1);
  await page.locator('#composerInput').fill('New draft stays untouched');
  // Live echo before the first HTTP response must replace, not duplicate, its optimistic row.
  await page.evaluate(message=>window.testEvents.dispatchEvent(new MessageEvent('message',{data:JSON.stringify(message)})),rows.get(first.client));
  assert.equal(await page.locator(`[data-message-id="${first.client}"]`).count(),1);
  gate=null;release();await page.waitForFunction(()=>!document.querySelector('.outbox-status'));
  assert.equal(attempts.length,2);assert.equal(await page.locator('#composerInput').inputValue(),'New draft stays untouched');assert.equal(await page.locator(`[data-message-id="${first.client}"]`).count(),1);
  fail=true;await page.locator('#composerInput').fill('Will fail');await page.locator('#sendBtn').click();await page.getByText('Provider unavailable',{exact:true}).waitFor();await waitAttempts(3);const failedId=attempts.at(-1).client;
  await page.locator('#composerInput').fill('Keep this while retrying');fail=false;await page.getByRole('button',{name:'Retry send',exact:true}).click();await page.waitForFunction(()=>!document.querySelector('.outbox-status'));
  assert.equal(attempts.at(-1).client,failedId);assert.equal(await page.locator('#composerInput').inputValue(),'Keep this while retrying');assert.equal(await page.locator(`[data-message-id="${failedId}"]`).count(),1);
  // A delayed image moves from preview into its outgoing bubble immediately.
  gate=new Promise(done=>release=done);await page.locator('#imageFile').setInputFiles({name:'photo.png',mimeType:'image/png',buffer:await readFile('tests/fixtures/pixel.png')});await page.locator('#sendBtn').click();
  await page.locator('.outbox-status').waitFor();await page.waitForFunction(()=>document.querySelector('.outbox-status').textContent.includes('Sending'));
  await waitAttempts(5);const imageId=attempts.at(-1).client;assert.ok(attempts.at(-1).image);assert.equal(await page.locator('.image-draft').count(),0);assert.equal(await page.locator('#composerInput').inputValue(),'');
  const preview=page.locator(`[data-message-id="${imageId}"] img`);assert.ok((await preview.getAttribute('src')).startsWith('blob:'));
  await page.locator('#composerInput').fill('Draft during photo upload');
  // Reopening restores the persisted image snapshot without automatically resending it.
  await page.reload();await page.locator('#composerInput').waitFor();await page.getByRole('button',{name:'Retry send',exact:true}).waitFor();
  assert.equal(await page.locator('#composerInput').inputValue(),'Draft during photo upload');assert.equal(attempts.at(-1).client,imageId);
  assert.ok((await page.locator(`[data-message-id="${imageId}"] img`).getAttribute('src')).startsWith('blob:'));
  let releaseImage;imageGate=new Promise(done=>releaseImage=done);gate=null;release();await page.getByRole('button',{name:'Retry send',exact:true}).click();await page.waitForFunction(()=>!document.querySelector('.outbox-status'));
  assert.ok(await page.locator(`[data-message-id="${imageId}"] .outbox-image-preview`).isVisible(),'Local preview stays visible while the server image downloads');imageGate=null;releaseImage();await page.locator(`[data-message-id="${imageId}"] .inline-image.media-loaded`).waitFor();
  assert.equal(attempts.at(-1).client,imageId);assert.equal(await page.locator(`[data-message-id="${imageId}"]`).count(),1);assert.equal(await page.locator('#composerInput').inputValue(),'Draft during photo upload');
  assert.deepEqual(errors,[]);await context.close();
 }
 console.log('Optimistic outbox: desktop/mobile, immediate rows and free composer, replies, ordered queue, early live echo, retry IDs, untouched next draft and persisted image reload passed');
}finally{await browser?.close();server.closeAllConnections();await new Promise(done=>server.close(done))}
