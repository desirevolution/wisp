// Run after npm run build. Optional browser dependency: npm install --no-save playwright
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.WISP_PLAYWRIGHT_MODULE || 'playwright');
const root = resolve('public'), sent = [], reacted = [], imageSent = [];
let failSend = false, failReaction = false, mirror = false;
const chat = 'g@g.us', other = 'h@g.us';
const messages = [
 {id:'m1',chat_id:chat,body:'Treffen wir uns morgen um 18 Uhr?',sender_name:'Arthur',type:'chat',timestamp:1790760000,from_me:0,reactions:[],reaction_version:0},
 {id:'m2',chat_id:chat,body:'Ja, das passt!',type:'chat',timestamp:1790760030,from_me:1,reactions:[],reply:{message_id:'m1',chat_id:chat,sender_name:'Arthur',body:'Treffen wir uns morgen um 18 Uhr?',type:'chat'}},
 {id:'m3',chat_id:chat,body:'Antwort mit nicht verfügbarem Original',type:'chat',timestamp:1790760050,from_me:0,reply:{message_id:'missing',chat_id:chat,sender_name:'Bea',body:'Alte Frage',type:'chat'},reactions:[{actor_id:'me',from_me:true,actor_name:'You',emoji:'🦊',timestamp_ms:1},{actor_id:'bea',from_me:false,actor_name:'Bea',emoji:'🦊',timestamp_ms:1}],reaction_version:1},
];
const server = createServer(async (req,res) => {
 const url=new URL(req.url,'http://local');
 const json=(data,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(data))};
 if(url.pathname==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:!mirror}});
 if(url.pathname==='/api/config')return json({sendEnabled:!mirror,mirrorOnly:mirror,followNewMessages:false});
 if(url.pathname==='/api/chats')return json([{id:chat,name:'Gruppe',type:'group',last_message_at:1790760050,last_body:'Antwort'},{id:other,name:'Andere Gruppe',type:'group',last_message_at:1790760000,last_body:'Hallo'}]);
 const parts=decodeURIComponent(url.pathname).split('/');
 if(parts[1]==='api'&&parts[2]==='chats'){
  if(parts[4]==='send-image'){
   let raw='';for await(const data of req)raw+=data;imageSent.push(raw);
   return failSend?json({error:'Test image send failure'},502):json({ok:true,id:'sent-image'});
  }
  if(parts[4]==='send'){
   let raw='';for await(const data of req)raw+=data;sent.push({chat:parts[3],...JSON.parse(raw)});
   return failSend?json({error:'Test send failure'},502):json({ok:true,id:'sent'});
  }
  if(parts[4]==='edits'||parts[4]==='deletions')return json([]);
  if(parts[4]==='reactions')return json(messages.map(m=>({chat_id:chat,message_id:m.id,reactions:m.reactions,version:m.reaction_version||0})));
  if(parts[4]==='messages'&&parts[6]==='reactions'){
   let raw='';for await(const data of req)raw+=data;const {emoji}=JSON.parse(raw);reacted.push({id:parts[5],emoji});
   if(failReaction)return json({error:'Test reaction failure'},502);
   const m=messages.find(m=>m.id===parts[5]);m.reactions=m.reactions.filter(r=>!r.from_me);if(emoji)m.reactions.push({actor_id:'me',from_me:true,emoji,actor_name:'You'});m.reaction_version=(m.reaction_version||0)+1;
   return json({ok:true,chat_id:chat,message_id:m.id,reactions:m.reactions,version:m.reaction_version});
  }
  if(parts[4]==='messages'&&parts[6]==='context')return parts[5]==='missing'?json({error:'Message not found'},404):json(messages);
  if(parts[4]==='messages')return json(parts[3]===chat?messages:[{id:'h1',chat_id:other,body:'Hallo',type:'chat',timestamp:1790760000,from_me:0,reactions:[]}]);
 }
 try {const file=resolve(root,'.'+(url.pathname==='/'?'/index.html':url.pathname));if(!file.startsWith(root+'/'))throw new Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.json':'application/json','.webmanifest':'application/manifest+json','.svg':'image/svg+xml','.png':'image/png'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
const base=`http://127.0.0.1:${server.address().port}`;
async function pageFor(options){
 const context=await browser.newContext({serviceWorkers:'block',...options});
 await context.addInitScript(()=>{window.EventSource=class extends EventTarget{constructor(){super();window.testEvents=this;setTimeout(()=>this.dispatchEvent(new Event('open')),20)}close(){}}});
 const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(base);await page.locator(`[data-id="${chat}"]`).click();await page.locator('[data-message-id="m1"]').waitFor();return {context,page,errors};
}
try{
 browser=await chromium.launch({headless:true,...(process.env.WISP_CHROME?{executablePath:process.env.WISP_CHROME}:{})});
 const {context,page,errors}=await pageFor({viewport:{width:1200,height:850}});
 const row=id=>page.locator(`[data-message-id="${id}"]`).locator('..').locator('..');
 await row('m1').hover();await row('m1').getByRole('button',{name:'Reply to message',exact:true}).click();
 await page.locator('.reply-draft').waitFor();await page.locator('#composerInput').fill('Entwurf A');
 await page.locator(`[data-id="${other}"]`).click();await page.locator('#composerInput').fill('Entwurf B');
 await page.locator(`[data-id="${chat}"]`).click();assert.equal(await page.locator('#composerInput').inputValue(),'Entwurf A');assert.ok(await page.locator('.reply-draft').isVisible());
 await page.locator('#composerInput').press('Escape');assert.equal(await page.locator('.reply-draft').count(),0);assert.equal(await page.locator('#composerInput').inputValue(),'Entwurf A');
 await row('m1').hover();await row('m1').getByRole('button',{name:'Reply to message',exact:true}).click();
 failSend=true;await page.locator('#sendBtn').click();await page.getByText('Test send failure',{exact:true}).waitFor();assert.ok(await page.locator('.reply-draft').isVisible());assert.equal(await page.locator('#composerInput').inputValue(),'Entwurf A');
 failSend=false;await page.locator('#sendBtn').click();await page.waitForFunction(()=>document.querySelector('#composerInput').value==='');assert.equal(sent.at(-1).reply_to,'m1');assert.equal(await page.locator('.reply-draft').count(),0);
 await row('m1').hover();await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover:popover-open').waitFor();
 assert.equal(await page.locator('.message-popover .emoji-grid button').count(),32);
 await page.locator('.message-popover').getByRole('button',{name:'thumbsup',exact:true}).click();await row('m1').locator('.reaction-chips').waitFor();assert.equal(reacted.at(-1).emoji,'👍');
 await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover').getByRole('button',{name:'thumbsup',exact:true}).first().click();await page.waitForFunction(()=>!document.querySelector('[data-message-id="m1"]')?.parentElement.querySelector('.reaction-chips'));assert.equal(reacted.at(-1).emoji,'');
 await row('m3').locator('.reaction-chips button').click();await page.getByRole('button',{name:'Remove',exact:true}).click();assert.equal(reacted.at(-1).emoji,'');
 await row('m3').locator('.quoted-message').click();await page.getByText('Original message is not available.',{exact:true}).waitFor();assert.equal(await page.locator('#conversationTitle').textContent(),'Gruppe');
 await row('m2').locator('.quoted-message').click();assert.ok(await page.locator('[data-message-id="m1"]').evaluate(el=>el.classList.contains('highlight')));
 failReaction=true;await row('m1').hover();await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover').getByRole('button',{name:'heart',exact:true}).click();await page.getByText('Test reaction failure',{exact:true}).waitFor();assert.equal(await row('m1').locator('.reaction-chips').count(),0);failReaction=false;
 await page.locator('#composerInput').fill('discard on reload');await page.reload();await page.locator(`[data-id="${chat}"]`).click();await page.locator('#composerInput').waitFor();assert.equal(await page.locator('#composerInput').inputValue(),'');
 // Real layout checks: actions center on the bubble, not the reaction chips.
 for(const id of ['m1','m2','m3']){
  const bubble=await page.locator(`[data-message-id="${id}"]`).boundingBox();
  const actions=await row(id).locator('.message-actions').boundingBox();
  assert.ok(Math.abs(bubble.y+bubble.height/2-actions.y-actions.height/2)<1.5,`actions not centered on ${id}`);
 }
 const textBox=await page.locator('[data-message-id="m2"] .message-text').boundingBox();
 const timeBox=await page.locator('[data-message-id="m2"] .inline-time').boundingBox();
 const bubbleBox=await page.locator('[data-message-id="m2"]').boundingBox();
 assert.ok(timeBox.y>textBox.y,'time should be visually below text');
 assert.ok(timeBox.x>=textBox.x+textBox.width+5,'short message time must share the line');
 assert.ok(timeBox.y+timeBox.height<=bubbleBox.y+bubbleBox.height+.5,'time must stay inside bubble');
 // The actual browser handles multipart image drafts as well as JSON text sends.
 await row('m1').hover();await row('m1').getByRole('button',{name:'Reply to message',exact:true}).click();
 await page.locator('#composerInput').fill('My image caption');
 await page.locator('#imageFile').setInputFiles({name:'pixel.png',mimeType:'image/png',buffer:await readFile('tests/fixtures/pixel.png')});
 await page.locator('.image-draft').waitFor();
 failSend=true;await page.locator('#sendBtn').click();await page.getByText('Test image send failure',{exact:true}).waitFor();
 assert.ok(await page.locator('.image-draft').isVisible());assert.ok(await page.locator('.reply-draft').isVisible());
 assert.equal(await page.locator('#composerInput').inputValue(),'My image caption');
 failSend=false;await page.locator('#sendBtn').click();await page.waitForFunction(()=>!document.querySelector('.image-draft'));
 assert.ok(imageSent.at(-1).includes('My image caption'));assert.ok(imageSent.at(-1).includes('m1'));
 assert.equal(await page.locator('#composerInput').inputValue(),'');
 assert.deepEqual(errors,[]);await context.close();console.log('Desktop: reply, image drafts/send/failure, reactions, context and layout checks passed');
 const mobile=await pageFor({viewport:{width:390,height:844},isMobile:true,hasTouch:true,deviceScaleFactor:1});
 const mr=mobile.page.locator('[data-message-id="m3"]').locator('..').locator('..');
 assert.equal(await mr.locator('.message-actions').evaluate(el=>getComputedStyle(el).opacity),'1');
 await mr.getByRole('button',{name:'React to message',exact:true}).click();await mobile.page.locator('.message-popover:popover-open').waitFor();
 const box=await mobile.page.locator('.message-popover').boundingBox();assert.ok(box.x>=0&&box.y>=0&&box.x+box.width<=390&&box.y+box.height<=844,JSON.stringify(box));
 if(process.env.WISP_SCREENSHOT)await mobile.page.screenshot({path:process.env.WISP_SCREENSHOT});
 assert.deepEqual(mobile.errors,[]);await mobile.context.close();console.log('Mobile: persistent action buttons and bounded picker passed');
 mirror=true;const readonly=await pageFor({viewport:{width:1100,height:800}});assert.equal(await readonly.page.locator('.message-actions').count(),0);assert.equal(await readonly.page.locator('#composer').count(),0);await readonly.context.close();console.log('Mirror mode: send actions hidden');
}finally{await browser?.close();await new Promise(resolve=>server.close(resolve))}
