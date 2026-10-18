// Run after npm run build. Optional browser dependency: npm install --no-save playwright
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import assert from 'node:assert/strict';
const { chromium } = await import(process.env.WISP_PLAYWRIGHT_MODULE || 'playwright');
const root = resolve('public'), sent = [], reacted = [], imageSent = [];
let failSend = false, failReaction = false, mirror = false, switchRows = false, mediaDemo = false;
const chat = 'g@g.us', other = 'h@g.us';
const messages = [
 {id:'m1',chat_id:chat,body:'Treffen wir uns morgen um 18 Uhr?',sender_name:'Arthur',type:'chat',timestamp:1790760000,from_me:0,reactions:[],reaction_version:0},
 {id:'m2',chat_id:chat,body:'Ja, das passt!',type:'chat',timestamp:1790760030,from_me:1,reactions:[],reply:{message_id:'m1',chat_id:chat,sender_name:'Arthur',body:'Treffen wir uns morgen um 18 Uhr?',type:'chat'}},
 {id:'m3',chat_id:chat,body:'Antwort mit nicht verfügbarem Original',type:'chat',timestamp:1790760050,from_me:0,reply:{message_id:'missing',chat_id:chat,sender_name:'Bea',body:'Alte Frage',type:'chat'},reactions:[{actor_id:'me',from_me:true,actor_name:'You',emoji:'🦊',timestamp_ms:1},{actor_id:'bea',from_me:false,actor_name:'Bea',emoji:'🦊',timestamp_ms:1}],reaction_version:1},
];
const server = createServer(async (req,res) => {
 const url=new URL(req.url,'http://local');
 const json=(data,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(data))};
 if(url.pathname==='/outside-wisp'){res.writeHead(200,{'content-type':'text/html'});res.end('<h1>Outside Wisp</h1>');return}
 if(url.pathname==='/api/media/demo-portrait'){res.writeHead(200,{'content-type':'image/svg+xml'});res.end('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="800"><rect width="200" height="800" fill="#238b88"/></svg>');return}
 if(url.pathname==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:!mirror}});
 if(url.pathname==='/api/config')return json({sendEnabled:!mirror,mirrorOnly:mirror,followNewMessages:false});
 if(url.pathname==='/api/search')return json({chats:[{id:chat,name:'Gruppe',type:'group'}],messages:[{...messages[0],chat_name:'Gruppe'}]});
 if(url.pathname.endsWith('/media')||url.pathname.endsWith('/links'))return json([]);
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
  if(parts[4]==='messages'&&mediaDemo&&parts[3]===other)return json(url.searchParams.has('beforeTs')?[]:[{id:'known-portrait',chat_id:other,type:'image',has_media:1,media_id:'known-portrait',media_status:'pending',media_width:600,media_height:1200,timestamp:1790763980,from_me:0,reactions:[]},{id:'known-landscape',chat_id:other,type:'image',has_media:1,media_id:'known-landscape',media_status:'pending',media_width:1600,media_height:900,timestamp:1790763990,from_me:0,reactions:[]},{id:'portrait-message',chat_id:other,sender_name:'Demo',sender_id:'demo',body:'Portrait image',type:'image',has_media:1,media_id:'demo-portrait',media_status:'ready',timestamp:1790764000,from_me:0,reactions:[]},{id:'after-portrait',chat_id:other,body:'This message must not move when the image finishes loading.',type:'chat',timestamp:1790764010,from_me:1,reactions:[]}]);
  if(parts[4]==='messages'&&switchRows)return json(url.searchParams.has('beforeTs')?[]:Array.from({length:60},(_,i)=>({id:`${parts[3]}-${i}`,chat_id:parts[3],sender_id:'demo',sender_name:'Demo',body:`Message ${i}: a longer conversation to check scrolling during chat switching.`,type:'chat',timestamp:1790760000+i*60,from_me:i%2,reactions:[]})));
  if(parts[4]==='messages')return json(parts[3]===chat?messages:[{id:'h1',chat_id:other,body:'Hallo',type:'chat',timestamp:1790760000,from_me:0,reactions:[]}]);
 }
 try {const file=resolve(root,'.'+(url.pathname==='/'?'/index.html':url.pathname));if(!file.startsWith(root+'/'))throw new Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.json':'application/json','.webmanifest':'application/manifest+json','.svg':'image/svg+xml','.png':'image/png'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
const base=`http://127.0.0.1:${server.address().port}`;
async function pageFor(options,{previous=false}={}){
 const context=await browser.newContext({serviceWorkers:'block',...options});
 await context.addInitScript(()=>{window.EventSource=class extends EventTarget{constructor(){super();window.testEvents=this;setTimeout(()=>this.dispatchEvent(new Event('open')),20)}close(){}}});
 const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 if(previous)await page.goto(base+'/outside-wisp');
 await page.goto(base);await page.evaluate(()=>window.startHistoryLength=history.length);await page.locator(`[data-id="${chat}"]`).click();await page.locator(switchRows?'[data-message-id="g@g.us-59"]':'[data-message-id="m1"]').waitFor();return {context,page,errors};
}
try{
 browser=await chromium.launch({headless:true,...(process.env.WISP_CHROME?{executablePath:process.env.WISP_CHROME}:{})});
 if(!process.env.WISP_SWITCH_ONLY){
 const {context,page,errors}=await pageFor({viewport:{width:1200,height:850}});
 const row=id=>page.locator(`[data-message-id="${id}"]`).locator('..').locator('..');
 await row('m1').hover();await row('m1').getByRole('button',{name:'Reply to message',exact:true}).click();
 await page.locator('.reply-draft').waitFor();await page.locator('#composerInput').fill('Entwurf A');
 await page.locator(`[data-id="${other}"]`).click();await page.locator('#composerInput').fill('Entwurf B');
 await page.locator(`[data-id="${chat}"]`).click();await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('chat-preparing'));assert.equal(await page.locator('#composerInput').inputValue(),'Entwurf A');assert.ok(await page.locator('.reply-draft').isVisible());
 await page.locator('#composerInput').press('Escape');assert.equal(await page.locator('.reply-draft').count(),0);assert.equal(await page.locator('#composerInput').inputValue(),'Entwurf A');
 await row('m1').hover();await row('m1').getByRole('button',{name:'Reply to message',exact:true}).click();
 failSend=true;await page.locator('#sendBtn').click();await page.getByText('Test send failure',{exact:true}).waitFor();assert.ok(await page.locator('.reply-draft').isVisible());assert.equal(await page.locator('#composerInput').inputValue(),'Entwurf A');
 failSend=false;await page.locator('#sendBtn').click();await page.waitForFunction(()=>document.querySelector('#composerInput').value==='');assert.equal(sent.at(-1).reply_to,'m1');assert.equal(await page.locator('.reply-draft').count(),0);
 await row('m1').hover();await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover:popover-open').waitFor();
 assert.equal(await page.locator('.message-popover .popover-close').isVisible(),false);
 assert.equal(await page.locator('.message-popover .emoji-grid button').count(),32);
 await page.locator('#conversationTitle').click();await page.waitForFunction(()=>!document.querySelector('.message-popover'));
 await row('m1').hover();await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover:popover-open').waitFor();
 await page.locator('.message-popover').getByRole('button',{name:'thumbsup',exact:true}).click();await row('m1').locator('.reaction-chips').waitFor();assert.equal(reacted.at(-1).emoji,'👍');
 await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover').getByRole('button',{name:'thumbsup',exact:true}).first().click();await page.waitForFunction(()=>!document.querySelector('[data-message-id="m1"]')?.parentElement.querySelector('.reaction-chips'));assert.equal(reacted.at(-1).emoji,'');
 await row('m3').locator('.reaction-chips button').click();await page.getByRole('button',{name:'Remove',exact:true}).click();assert.equal(reacted.at(-1).emoji,'');
 await row('m3').locator('.reaction-chips button').click();assert.equal(await page.locator('.message-popover .popover-close').isVisible(),false);await page.locator('.message-popover').press('Escape');await page.waitForFunction(()=>!document.querySelector('.message-popover'));
 await row('m3').locator('.quoted-message').click();await page.getByText('Original message is not available.',{exact:true}).waitFor();assert.equal(await page.locator('#conversationTitle').textContent(),'Gruppe');
 await row('m2').locator('.quoted-message').click();assert.ok(await page.locator('[data-message-id="m1"]').evaluate(el=>el.classList.contains('highlight')));
 failReaction=true;await row('m1').hover();await row('m1').getByRole('button',{name:'React to message',exact:true}).click();await page.locator('.message-popover').getByRole('button',{name:'heart',exact:true}).click();await page.getByText('Test reaction failure',{exact:true}).waitFor();assert.equal(await row('m1').locator('.reaction-chips').count(),0);failReaction=false;
 await page.locator('#composerInput').fill('persist on reload');await page.reload();await page.locator(`[data-id="${chat}"]`).click();await page.locator('#composerInput').waitFor();assert.equal(await page.locator('#composerInput').inputValue(),'persist on reload');
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

 // UI refresh regressions: focus and draft survive live shell updates.
 await page.locator('#composerInput').fill('Stable draft');
 await page.locator('#composerInput').focus();
 await page.locator('#composerInput').evaluate(el=>{window.savedComposer=el});
 await page.evaluate(()=>window.testEvents.dispatchEvent(new MessageEvent('connection',{data:JSON.stringify({state:'connected',syncing:true})})));
 assert.ok(await page.locator('#composerInput').evaluate(el=>el===window.savedComposer&&el===document.activeElement));
 assert.equal(await page.locator('#composerInput').inputValue(),'Stable draft');
 // Keyboard resize persists on reload.
 await page.locator('#sidebarResize').focus();await page.locator('#sidebarResize').press('ArrowRight');
 const sidebarWidth=await page.locator('.sidebar').evaluate(el=>Math.round(el.getBoundingClientRect().width));
 assert.equal(sidebarWidth,370);
 await page.reload();await page.locator('#sidebarResize').waitFor();
 assert.equal(await page.locator('.sidebar').evaluate(el=>Math.round(el.getBoundingClientRect().width)),sidebarWidth);
 const initialHistoryLength=await page.evaluate(()=>history.length);
 // Back always returns to the chat selection; chat switches share one history entry.
 await page.locator('#search').fill('Treffen');await page.locator('.search-hit').waitFor();
 await page.locator('.search-hit').click();await page.locator('[data-message-id="m1"]').waitFor();
 assert.equal(await page.locator('#search').inputValue(),'');
 await page.getByRole('button',{name:'Back to chats',exact:true}).click();
 await page.getByText('Select a chat',{exact:true}).waitFor();
 assert.equal(await page.locator('#search').inputValue(),'');assert.equal(await page.locator('.search-hit').count(),0);
 assert.equal(await page.evaluate(()=>history.length),initialHistoryLength);
 await page.locator('#settingsBtn').click();await page.getByRole('dialog',{name:'Settings',exact:true}).waitFor();
 await page.getByRole('button',{name:'Close settings',exact:true}).click();await page.waitForFunction(()=>document.querySelector('#settings').hidden);
 await page.locator(`[data-id="${chat}"]`).first().click();
 await page.locator('#mediaBtn').click();await page.locator('#gallery').waitFor();
 await page.getByRole('button',{name:'Back to chats',exact:true}).click();
 await page.getByText('Select a chat',{exact:true}).waitFor();assert.equal(await page.locator('#gallery').isVisible(),false);
 await page.locator(`[data-id="${chat}"]`).first().click();
 await page.waitForFunction(()=>document.activeElement?.id==='composerInput'&&!document.querySelector('.message-skeleton'));
 await page.locator('#composerInput').press('Escape');
 assert.ok(await page.locator('.chat.keyboard-selected').evaluate(el=>el===document.activeElement));
 await page.locator('.chat.keyboard-selected').press('Enter');
 await page.waitForFunction(()=>document.activeElement?.id==='composerInput');
 // Fill history with live messages, then deliberately read away from the bottom.
 await page.evaluate(({chat,timestamp})=>{for(let i=0;i<40;i++)window.testEvents.dispatchEvent(new MessageEvent('message',{data:JSON.stringify({id:`long-${i}`,chat_id:chat,body:`Reading anchor message ${i}`,sender_id:'reader',sender_name:'Arthur',type:'chat',timestamp:timestamp+i+100,from_me:0,reactions:[]})}))},{chat,timestamp:messages[0].timestamp});
 await page.locator('[data-message-id="long-39"]').waitFor();
 await page.locator('#messages').evaluate(el=>{el.dispatchEvent(new WheelEvent('wheel'));el.scrollTop=350;el.dispatchEvent(new Event('scroll'))});
 const readingTop=await page.locator('#messages').evaluate(el=>el.scrollTop);
 await page.evaluate(({chat,timestamp})=>window.testEvents.dispatchEvent(new MessageEvent('message',{data:JSON.stringify({id:'new-while-reading',chat_id:chat,body:'New incoming',type:'chat',timestamp,from_me:0,reactions:[]})})),{chat,timestamp:messages[0].timestamp+1000});
 await page.locator('[data-message-id="new-while-reading"]').waitFor();
 assert.ok(Math.abs(await page.locator('#messages').evaluate(el=>el.scrollTop)-readingTop)<2,JSON.stringify({readingTop,current:await page.locator('#messages').evaluate(el=>el.scrollTop)}));
 assert.ok((await page.locator('#newMessages').textContent()).includes('new message'));
 await page.evaluate(()=>window.testEvents.dispatchEvent(new MessageEvent('connection',{data:JSON.stringify({state:'disconnected'})})));
 await page.locator('#connectionView').waitFor();assert.equal(await page.locator('#composer').isVisible(),false);
 await page.evaluate(()=>window.testEvents.dispatchEvent(new MessageEvent('connection',{data:JSON.stringify({state:'connected'})})));
 await page.locator('#messages').waitFor();
 assert.ok(Math.abs(await page.locator('#messages').evaluate(el=>el.scrollTop)-readingTop)<2,JSON.stringify({readingTop,current:await page.locator('#messages').evaluate(el=>el.scrollTop)}));
 assert.equal(await page.locator('#composerInput').inputValue(),'Stable draft');
 await page.locator('#composerInput').fill('one\ntwo\nthree\nfour\nfive\nsix\nseven');
 assert.ok(await page.locator('#composerInput').evaluate(el=>el.clientHeight<=120&&el.scrollHeight>el.clientHeight));
 await page.locator('#composerInput').fill('Stable draft');
 if(process.env.WISP_DESKTOP_SCREENSHOT)await page.screenshot({path:process.env.WISP_DESKTOP_SCREENSHOT});
 assert.deepEqual(errors,[]);await context.close();console.log('Desktop: reply, image drafts/send/failure, reactions, context and layout checks passed');
 const mobile=await pageFor({viewport:{width:390,height:844},isMobile:true,hasTouch:true,deviceScaleFactor:1});
 const mr=mobile.page.locator('[data-message-id="m3"]').locator('..').locator('..');
 assert.equal(await mr.locator('.message-actions').evaluate(el=>getComputedStyle(el).opacity),'1');
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();await mobile.page.locator('.message-sheet:popover-open').waitFor();
 const box=await mobile.page.locator('.message-sheet-content').boundingBox();assert.ok(box.x>=0&&box.y>=0&&box.x+box.width<=390&&Math.abs(box.y+box.height-844)<2,JSON.stringify(box));
 assert.ok((await mobile.page.locator('.sheet-message-preview').textContent()).includes('Antwort mit nicht verfügbarem Original'));
 assert.equal(await mobile.page.getByRole('button',{name:'React',exact:true}).count(),0);
 if(process.env.WISP_SHEET_SCREENSHOT)await mobile.page.screenshot({path:process.env.WISP_SHEET_SCREENSHOT});
 // Reactions are one tap away; clicking the selected reaction again removes it.
 await mobile.page.locator('.message-sheet').getByRole('button',{name:'thumbsup',exact:true}).last().click();
 await mobile.page.waitForFunction(()=>!document.querySelector('.message-popover'));assert.equal(reacted.at(-1).emoji,'👍');
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();await mobile.page.locator('.message-sheet button[aria-pressed="true"]').last().click();
 await mobile.page.waitForFunction(()=>!document.querySelector('.message-popover'));assert.equal(reacted.at(-1).emoji,'');
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();
 await mobile.page.getByRole('button',{name:'Reply',exact:true}).click();
 await mobile.page.locator('.reply-draft').waitFor();assert.equal(await mobile.page.locator('.message-sheet').count(),0);
 await mobile.page.locator('#composerInput').press('Escape');
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();
 await mobile.page.locator('.message-sheet-backdrop').click({position:{x:5,y:5}});await mobile.page.waitForFunction(()=>!document.querySelector('.message-popover'));
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();

 await mobile.page.locator('.message-popover').press('Escape');
 await mobile.page.waitForFunction(()=>!document.querySelector('.message-popover'));
 await mobile.page.locator('#composerInput').fill('Mobile line');await mobile.page.locator('#composerInput').press('Enter');
 assert.equal(await mobile.page.locator('#composerInput').inputValue(),'Mobile line\n');
 // A touch with the editor focused must open the sheet on the first tap.
 // Simulate the viewport expansion/scroll that a software keyboard causes;
 // browser emulation does not provide an actual Android keyboard.
 await mobile.page.locator('#composerInput').focus();
 await mr.getByRole('button',{name:'Message actions',exact:true}).tap();
 await mobile.page.locator('.message-sheet:popover-open').waitFor();
 await mobile.page.evaluate(()=>{window.dispatchEvent(new Event('resize'));document.querySelector('#messages').dispatchEvent(new Event('scroll'))});
 await mobile.page.locator('.message-sheet:popover-open').waitFor();
 assert.equal(await mobile.page.locator('#composerInput').evaluate(el=>el===document.activeElement),false);
 await mobile.page.locator('.message-sheet').getByRole('button',{name:'Close',exact:true}).click();
 // Native camera selection uses the existing preview, never automatic sending.
 const camera=mobile.page.getByRole('button',{name:'Take photo',exact:true});
 assert.equal(await camera.isVisible(),true);
 const chooserPromise=mobile.page.waitForEvent('filechooser');await camera.click();
 const chooser=await chooserPromise;
 assert.equal(await chooser.element().getAttribute('capture'),'environment');
 const beforeCameraSend=imageSent.length;
 await chooser.setFiles({name:'camera.jpg',mimeType:'image/jpeg',buffer:Buffer.from([0xff,0xd8,0xff,0xd9])});
 await mobile.page.locator('.image-draft').waitFor();
 assert.equal(imageSent.length,beforeCameraSend);
 assert.equal(await mobile.page.locator('#composerInput').inputValue(),'Mobile line\n');
 await mobile.page.getByRole('button',{name:'Remove image',exact:true}).click();
 // Large native camera captures are resized before preview/upload. A held send
 // visibly stays in progress and cannot be submitted twice.
 const cameraPhoto=await mobile.page.evaluate(async()=>{
   const canvas=document.createElement('canvas');canvas.width=3200;canvas.height=2400;
   const ctx=canvas.getContext('2d'),gradient=ctx.createLinearGradient(0,0,3200,2400);
   gradient.addColorStop(0,'#036');gradient.addColorStop(1,'#fd6');ctx.fillStyle=gradient;ctx.fillRect(0,0,3200,2400);
   return canvas.toDataURL('image/jpeg',.98).split(',')[1];
 });
 const cameraBytes=Buffer.from(cameraPhoto,'base64');
 await mobile.page.locator('#cameraFile').setInputFiles({name:'large-camera.jpg',mimeType:'image/jpeg',buffer:cameraBytes});
 await mobile.page.waitForFunction(()=>{const el=document.querySelector('.image-draft img');return el?.complete&&el.naturalWidth===2048&&el.naturalHeight===1536});
 const optimizedSize=await mobile.page.locator('#composer').evaluate(el=>el.draft.image.file.size);
 assert.ok(optimizedSize<cameraBytes.length,'Camera optimization must reduce the upload size');
 const cameraPreview=await mobile.page.locator('.image-draft img').boundingBox();
 assert.ok(Math.abs(cameraPreview.width/cameraPreview.height-4/3)<.02,'Camera preview must use its actual aspect ratio');
 let releaseCameraSend;
 const cameraGate=new Promise(resolve=>{releaseCameraSend=resolve});
 await mobile.page.route('**/send-image',async route=>{await cameraGate;await route.continue()});
 await mobile.page.locator('#sendBtn').click();
 await mobile.page.locator('.send-progress').waitFor();
 assert.match(await mobile.page.locator('.send-progress').textContent(),/WhatsApp confirmation/);
 assert.equal(await mobile.page.locator('#sendBtn').isDisabled(),true);
 releaseCameraSend();await mobile.page.waitForFunction(()=>!document.querySelector('.image-draft'));
 await mobile.page.unroute('**/send-image');
 assert.equal(await mobile.page.locator('#composerInput').inputValue(),'');
 console.log(`Camera photo: ${cameraBytes.length} -> ${optimizedSize} bytes; 3200x2400 -> 2048x1536; held send remains visible`);
 await mobile.page.locator('#composerInput').fill('Mobile line\n');
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();await mobile.page.locator('.message-popover').waitFor();
 await mobile.page.locator('.message-sheet').getByRole('button',{name:'Close',exact:true}).click();await mobile.page.waitForFunction(()=>!document.querySelector('.message-popover'));
 assert.equal(await mobile.page.evaluate(()=>history.length),await mobile.page.evaluate(()=>window.startHistoryLength)+1);
 assert.equal(await mobile.page.evaluate(()=>history.state.view),'chat');
 assert.equal(await mobile.page.locator('#composerInput').inputValue(),'Mobile line\n');
 await mobile.page.locator('#composerInput').fill('');await mobile.page.locator('#composerInput').blur();
 if(process.env.WISP_SCREENSHOT)await mobile.page.screenshot({path:process.env.WISP_SCREENSHOT});
 await mobile.page.getByRole('button',{name:'Back to chats',exact:true}).click();await mobile.page.locator('.sidebar').waitFor();
 await mobile.page.locator(`[data-id="${chat}"]`).click();
 await mobile.page.setViewportSize({width:320,height:480});
 await mr.getByRole('button',{name:'Message actions',exact:true}).click();await mobile.page.locator('.message-sheet-content').waitFor();
 const smallBox=await mobile.page.locator('.message-sheet-content').boundingBox();assert.ok(smallBox.x>=0&&smallBox.y>=0&&smallBox.x+smallBox.width<=320&&smallBox.y+smallBox.height<=481);
 const replyBox=await mobile.page.getByRole('button',{name:'Reply',exact:true}).boundingBox();assert.ok(replyBox.y>=0&&replyBox.y+replyBox.height<=480);
 await mobile.page.locator('.message-sheet').press('Escape');
 assert.deepEqual(mobile.errors,[]);await mobile.context.close();console.log('Mobile: bottom sheet, preview, direct reactions/removal, reply, backdrop/Escape/Close, direct Back-to-chats and Enter passed');
 mirror=true;const readonly=await pageFor({viewport:{width:1100,height:800}});assert.equal(await readonly.page.locator('.message-actions').count(),0);assert.equal(await readonly.page.locator('#composer').isVisible(),false);await readonly.context.close();console.log('Mirror mode: send actions hidden');

 }
 // Browser/PWA navigation: overview -> chat -> image, without raw-media
 // navigation or accumulating an entry for every chat switch.
 switchRows=false;mirror=false;
 const nav=await pageFor({viewport:{width:1200,height:844},isMobile:true,hasTouch:true},{previous:true});
 const np=nav.page,navLength=await np.evaluate(()=>history.length);
 const navPhoto={id:'nav-photo',chat_id:chat,type:'image',body:'Navigation portrait',has_media:1,media_id:'demo-portrait',media_status:'ready',media_width:200,media_height:800,timestamp:1790760060,from_me:0,reactions:[]};
 await np.route('**/api/chats/g%40g.us/messages?limit=50',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify([...messages,navPhoto])}));
 await np.reload();await np.locator('[data-message-id="nav-photo"]').waitFor();
 assert.equal(await np.evaluate(()=>history.length),navLength,'Chat reload must not add another history entry');
 await np.locator(`[data-id="${other}"]`).click();await np.locator('[data-message-id="h1"]').waitFor();
 assert.equal(await np.evaluate(()=>history.length),navLength,'Chat switches replace the chat history entry');
 await np.setViewportSize({width:390,height:844});
 await np.goBack();await np.waitForFunction(()=>history.state.view==='overview'&&!document.querySelector('#app').classList.contains('open'));await np.locator('.sidebar').waitFor();
 await np.goForward();await np.locator('[data-message-id="h1"]').waitFor();
 await np.goBack();await np.waitForFunction(()=>history.state.view==='overview'&&!document.querySelector('#app').classList.contains('open'));await np.locator('.sidebar').waitFor();
 await np.locator(`[data-id="${chat}"]`).click();await np.locator('[data-message-id="nav-photo"]').waitFor();
 const navImage=np.locator('[data-message-id="nav-photo"] .media-frame');
 await navImage.click();await np.locator('#imageViewer[open]').waitFor();
 assert.equal(nav.context.pages().length,1,'An image must not open a new browser/PWA tab');
 assert.equal(np.url(),base+'/','Image viewing must stay inside Wisp');
 assert.equal(await np.evaluate(()=>history.state.view),'image');
 await np.reload();await np.locator('#imageViewer[open]').waitFor();
 await np.goBack();await np.waitForFunction(()=>!document.querySelector('#imageViewer').open);
 assert.equal(await np.locator('#conversationTitle').textContent(),'Gruppe');
 await np.goForward();await np.locator('#imageViewer[open]').waitFor();
 await np.getByRole('button',{name:'Back to chat',exact:true}).click();
 await np.waitForFunction(()=>!document.querySelector('#imageViewer').open);
 await navImage.click();await np.locator('#imageViewer[open]').waitFor();
 await np.locator('#imageViewer').press('Escape');await np.waitForFunction(()=>!document.querySelector('#imageViewer').open);
 await np.goBack();await np.waitForFunction(()=>history.state.view==='overview'&&!document.querySelector('#app').classList.contains('open'));await np.locator('.sidebar').waitFor();
 await np.goBack();await np.getByRole('heading',{name:'Outside Wisp'}).waitFor();
 assert.deepEqual(nav.errors,[]);await nav.context.close();
 console.log('Browser/PWA Back: image -> chat -> overview -> previous site; Forward, reload, close/Escape and single chat entry passed');

 // Cached chat refresh must preserve the position reached while the request is pending.
 mirror=false;switchRows=true;
 const switching=await pageFor({viewport:{width:1200,height:850}});
 const sp=switching.page;
 const waitBottom=()=>sp.waitForFunction(()=>{const el=document.querySelector('#messages');return el.scrollHeight-el.scrollTop-el.clientHeight<2});
 await waitBottom();
 await sp.locator('#messages').evaluate(el=>{el.dispatchEvent(new WheelEvent('wheel'));el.scrollTop=600;el.dispatchEvent(new Event('scroll'))});
 await sp.locator(`[data-id="${other}"]`).click();await sp.locator('[data-message-id="h@g.us-59"]').waitFor();await waitBottom();
 let finishRefresh;
 const pendingRefresh=new Promise(done=>{finishRefresh=done});
 await sp.route('**/api/chats/g%40g.us/messages?limit=50',async route=>{await pendingRefresh;await route.continue()});
 await sp.locator(`[data-id="${chat}"]`).click();await sp.locator('[data-message-id="g@g.us-59"]').waitFor();
 await sp.waitForFunction(()=>document.querySelector('#messages').scrollTop>500&&document.querySelector('#messages').scrollTop<700);
 // Change position after the cached page is shown, before the refresh completes.
 await sp.locator('#messages').evaluate(el=>{el.dispatchEvent(new WheelEvent('wheel'));el.scrollTop=1100;el.dispatchEvent(new Event('scroll'));window.switchFrames=[];window.recordSwitch=true;const record=()=>{if(!window.recordSwitch)return;window.switchFrames.push({top:el.scrollTop,skeleton:Boolean(el.querySelector('.message-skeleton'))});requestAnimationFrame(record)};requestAnimationFrame(record)});
 const loaded=sp.waitForResponse(response=>response.url().includes('/api/chats/g%40g.us/messages?limit=50'));
 finishRefresh();await loaded;
 await sp.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))));
 const frames=await sp.evaluate(()=>{window.recordSwitch=false;return window.switchFrames});
 assert.ok(frames.length>0);assert.ok(frames.every(frame=>!frame.skeleton&&Math.abs(frame.top-1100)<2),JSON.stringify(frames));
 assert.deepEqual(switching.errors,[]);await switching.context.close();console.log('Chat switching: cached refresh preserves the live reading position without loading frames');

 // A successful image POST must reveal its message even without an SSE event,
 // then keep following late media layout. Uploads completed in another chat
 // must never pull that chat away from its reading position.
 const uploaded=await pageFor({viewport:{width:1200,height:850}}),up=uploaded.page;
 const photo={id:'photo-scroll',chat_id:chat,from_me:1,type:'image',body:'Uploaded photo',has_media:1,media_id:'demo-portrait',media_status:'ready',media_width:200,media_height:800,timestamp:1790770000,reactions:[]};
 await up.route('**/api/status',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({version:'v1.2.3',whatsapp:{state:'connected'},send:{enabled:true},update:{current:'v1.2.3',latest:'v1.3.0',url:'https://github.com/desirevolution/wisp/releases/tag/v1.3.0',available:true,comparable:true,checkedAt:1790760000000}})}));
 await up.getByRole('button',{name:'Open settings',exact:true}).click();
 await up.getByText('Wisp v1.2.3',{exact:true}).waitFor();
 await up.locator('.app-version').getByRole('link',{name:'View release'}).waitFor();
 await up.getByRole('button',{name:'Close settings',exact:true}).click();
 await up.locator('#updateNotice').waitFor();
 await up.route('**/messages/photo-scroll/context',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify([photo])}));
 await up.route('**/send-image',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({ok:true,id:photo.id})}));
 let showPhoto;const photoGate=new Promise(resolve=>{showPhoto=resolve});
 await up.route('**/api/media/demo-portrait',async route=>{await photoGate;await route.continue()});
 await up.locator('#imageFile').setInputFiles({name:'photo.png',mimeType:'image/png',buffer:await readFile('tests/fixtures/pixel.png')});
 await up.locator('#messages').evaluate(el=>{el.dispatchEvent(new WheelEvent('wheel'));el.scrollTop=600;el.dispatchEvent(new Event('scroll'))});
 await up.locator('#sendBtn').click();
 await up.locator('[data-message-id="photo-scroll"]').waitFor();
 await up.waitForFunction(()=>{const el=document.querySelector('#messages');return el.scrollHeight-el.scrollTop-el.clientHeight<2});
 assert.equal(await up.locator('[data-message-id="g@g.us-0"]').count(),1,'Context merge must retain loaded history');
 showPhoto();await up.locator('[data-message-id="photo-scroll"] .media-loaded').waitFor();
 await up.waitForFunction(()=>{const el=document.querySelector('#messages');return el.scrollHeight-el.scrollTop-el.clientHeight<2});
 await up.unroute('**/send-image');
 let finishOther;const otherGate=new Promise(resolve=>{finishOther=resolve});
 await up.route('**/send-image',async route=>{await otherGate;await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({ok:true,id:'late-photo'})})});
 await up.locator('#imageFile').setInputFiles({name:'photo.png',mimeType:'image/png',buffer:await readFile('tests/fixtures/pixel.png')});
 await up.locator('#sendBtn').click();
 await up.locator(`[data-id="${other}"]`).click();await up.locator('[data-message-id="h@g.us-59"]').waitFor();
 await up.locator('#messages').evaluate(el=>{el.dispatchEvent(new WheelEvent('wheel'));el.scrollTop=600;el.dispatchEvent(new Event('scroll'))});
 const otherTop=await up.locator('#messages').evaluate(el=>el.scrollTop);
 const lateResponse=up.waitForResponse(response=>response.url().endsWith('/send-image'));
 finishOther();await lateResponse;await up.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
 assert.ok(Math.abs(await up.locator('#messages').evaluate(el=>el.scrollTop)-otherTop)<2);
 assert.deepEqual(uploaded.errors,[]);await uploaded.context.close();
 console.log('Release settings/update notice and successful image scroll: no SSE, late image, retained history and cross-chat guard passed');

 // Stage an uncached media chat: first text/layout, then independently loaded image.
 mediaDemo=true;
 const staged=await pageFor({viewport:{width:1200,height:850}}),mp=staged.page;
 let releaseBackend,releaseImage;
 const backendGate=new Promise(done=>{releaseBackend=done}),imageGate=new Promise(done=>{releaseImage=done});
 await mp.route('**/api/chats/h%40g.us/messages?limit=50',async route=>{await backendGate;await route.continue()});
 await mp.route('**/api/media/demo-portrait',async route=>{await imageGate;await route.continue()});
 await mp.evaluate(()=>{window.presentationFrames=[];window.recordPresentation=true;const sample=()=>{if(!window.recordPresentation)return;const list=document.querySelector('#messages'),loader=document.querySelector('#chatLoading');window.presentationFrames.push({time:performance.now(),title:document.querySelector('#conversationTitle').textContent,preparing:document.querySelector('#app').classList.contains('chat-preparing'),visibility:getComputedStyle(list).visibility,loading:!loader.hidden,disabled:document.querySelector('#composerInput').disabled,bottom:list.scrollHeight-list.scrollTop-list.clientHeight});requestAnimationFrame(sample)};requestAnimationFrame(sample)});
 await mp.locator(`[data-id="${other}"]`).click();await mp.locator('#chatLoading').waitFor();
 assert.equal(await mp.locator('#composerInput').isDisabled(),true);
 assert.equal(await mp.locator('#messages').evaluate(el=>getComputedStyle(el).visibility),'hidden');
 await mp.locator('#search').focus();
 releaseBackend();
 await mp.locator('[data-message-id="portrait-message"]').waitFor();
 await mp.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))));
 const presentation=await mp.evaluate(()=>{window.recordPresentation=false;return window.presentationFrames.filter(frame=>frame.title==='Andere Gruppe')});
 const waiting=presentation.filter(frame=>frame.preparing),revealed=presentation.find(frame=>!frame.preparing);
 assert.ok(waiting.length>0&&revealed,JSON.stringify(presentation));
 assert.ok(revealed.time-waiting[0].time>=145,JSON.stringify(presentation));
 assert.ok(waiting.every(frame=>frame.visibility==='hidden'&&frame.loading&&frame.disabled),JSON.stringify(presentation));
 assert.ok(presentation.filter(frame=>!frame.preparing).every(frame=>frame.visibility==='visible'&&!frame.loading&&frame.bottom<2));
 const image=mp.locator('[data-message-id="portrait-message"] img'),frame=mp.locator('[data-message-id="portrait-message"] .media-frame');
 assert.equal(await image.evaluate(el=>el.complete),false,'Chat must reveal without waiting for the image');
 assert.equal(await mp.evaluate(()=>document.activeElement.id),'search','Ready chat must not steal focus after the user returns to search');
 const knownPortrait=await mp.locator('[data-message-id="known-portrait"] .media-frame').boundingBox(),knownLandscape=await mp.locator('[data-message-id="known-landscape"] .media-frame').boundingBox();
 assert.ok(Math.abs(knownPortrait.width/knownPortrait.height-.5)<.01,JSON.stringify(knownPortrait));
 assert.ok(Math.abs(knownLandscape.width/knownLandscape.height-16/9)<.01,JSON.stringify(knownLandscape));
 assert.ok(knownPortrait.width<knownLandscape.width,'Initial pending frames must follow their own intrinsic sizes');
 const beforeImage=await frame.boundingBox(),beforeTail=await mp.locator('[data-message-id="after-portrait"]').boundingBox();
 releaseImage();await mp.locator('[data-message-id="portrait-message"] .media-loaded').waitFor();
 const afterImage=await frame.boundingBox(),afterTail=await mp.locator('[data-message-id="after-portrait"]').boundingBox();
 assert.ok(Math.abs(beforeImage.width/beforeImage.height-1.5)<.01,'Missing metadata starts with the temporary fallback');
 assert.ok(Math.abs(afterImage.width/afterImage.height-.25)<.01,'Unknown frame must resolve to the decoded portrait aspect ratio');
 assert.ok(Math.abs(afterTail.y-beforeTail.y)<2,'Resolving the frame must preserve the reading position at the bottom');
 assert.equal(await image.evaluate(el=>getComputedStyle(el).objectFit),'contain');
 if(process.env.WISP_PREPARED_SCREENSHOT)await mp.screenshot({path:process.env.WISP_PREPARED_SCREENSHOT});
 for(const status of ['pending','failed','ready']){
   await mp.evaluate(({other,status})=>window.testEvents.dispatchEvent(new MessageEvent('media',{data:JSON.stringify({chatId:other,messageId:'portrait-message',status})})),{other,status});
   if(status==='ready')await mp.locator('[data-message-id="portrait-message"] .media-loaded').waitFor();
   else await mp.locator('[data-message-id="portrait-message"] .media-placeholder').waitFor();
   const transitioned=await frame.boundingBox();
   assert.equal(transitioned.width,afterImage.width,`Width changed for media status ${status}`);
   assert.equal(transitioned.height,afterImage.height,`Height changed for media status ${status}`);
 }

 // Later visits use the learned portrait ratio immediately.
 await mp.locator(`[data-id="${chat}"]`).click();await mp.locator('[data-message-id="g@g.us-59"]').waitFor();
 await mp.locator(`[data-id="${other}"]`).click();await mp.locator('[data-message-id="portrait-message"]').waitFor();
 const learned=await frame.boundingBox();assert.ok(Math.abs(learned.width/learned.height-.25)<.01,JSON.stringify(learned));
 // Latest selection wins; an older preparation and response cannot reveal over it.
 await mp.evaluate(({chat,other})=>{document.querySelector(`[data-id="${chat}"]`).click();document.querySelector(`[data-id="${other}"]`).click();document.querySelector(`[data-id="${chat}"]`).click()},{chat,other});
 await mp.locator('[data-message-id="g@g.us-59"]').waitFor();
 assert.equal(await mp.locator('#conversationTitle').textContent(),'Gruppe');
 assert.equal(await mp.locator('[data-message-id="portrait-message"]').count(),0);
 assert.equal(await mp.locator('#chatLoading').isVisible(),false);

 // Back and fetch failures must not expose partial content or leave input active.
 await mp.getByRole('button',{name:'Back to chats',exact:true}).click();
 await mp.getByText('Select a chat',{exact:true}).waitFor();
 await mp.evaluate(({chat,other})=>{document.querySelector(`[data-id="${other}"]`).click();document.querySelector('#back').click()},{chat,other});
 await mp.evaluate(()=>new Promise(done=>setTimeout(done,200)));
 assert.equal(await mp.locator('#chatLoading').isVisible(),false);
 await mp.getByText('Select a chat',{exact:true}).waitFor();
 // An uncached failure has a stable retry surface rather than revealing an empty chat.
 const failureContext=await browser.newContext({viewport:{width:1000,height:800},serviceWorkers:'block'});
 const failurePage=await failureContext.newPage();
 await failureContext.addInitScript(()=>{window.EventSource=class extends EventTarget{close(){}}});
 let failNext=true;
 await failurePage.route('**/api/chats/h%40g.us/messages?limit=50',route=>{if(failNext){failNext=false;return route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({error:'Demo load failure'})})}return route.continue()});
 await failurePage.goto(base);await failurePage.locator(`[data-id="${other}"]`).click();
 await failurePage.getByRole('button',{name:'Try again',exact:true}).waitFor();
 assert.equal(await failurePage.locator('#composerInput').isDisabled(),true);
 assert.equal(await failurePage.locator('#messages').evaluate(el=>getComputedStyle(el).visibility),'hidden');
 await failurePage.getByRole('button',{name:'Try again',exact:true}).click();
 await failurePage.locator('[data-message-id="portrait-message"]').waitFor();
 assert.equal(await failurePage.locator('#composerInput').isDisabled(),false);
 await failureContext.close();
 // Resolving an unknown image above an older reading position preserves the
 // first visible message, including when browser scroll anchoring also runs.
 const anchorBefore=await mp.evaluate(async()=>{
   const list=document.createElement('wisp-message-list');list.id='anchor-regression-list';list.className='messages';
   list.style.cssText='position:fixed;inset:0 auto auto 0;width:400px;height:240px;overflow:auto;z-index:9999';
   list.chatId='anchor-test';list.hasSelection=true;
   const base={chat_id:'anchor-test',timestamp:1790764000,type:'chat',from_me:0,reactions:[]};
   list.messages=[{...base,id:'anchor-media',type:'image',has_media:1,media_id:'anchor-portrait',media_status:'pending'},...Array.from({length:25},(_,i)=>({...base,id:`anchor-${i}`,body:`Reading message ${i}`}))];
   document.body.append(list);await list.updateComplete;list.scrollTop=500;
   const top=list.getBoundingClientRect().top;
   const anchor=[...list.querySelectorAll('[data-message-id]')].find(el=>el.getBoundingClientRect().bottom>top);
   const before={id:anchor.dataset.messageId,top:anchor.getBoundingClientRect().top};
   list.messages=list.messages.map(m=>m.id==='anchor-media'?{...m,media_id:'demo-portrait',media_status:'ready'}:m);
   await list.updateComplete;list.querySelector('img').loading='eager';
   return before;
 });
 await mp.locator('#anchor-regression-list .media-loaded').waitFor();
 const anchorAfter=await mp.locator(`#anchor-regression-list [data-message-id="${anchorBefore.id}"]`).boundingBox();
 assert.ok(Math.abs(anchorAfter.y-anchorBefore.top)<2,JSON.stringify({anchorBefore,anchorAfter}));
 await mp.locator('#anchor-regression-list').evaluate(el=>el.remove());
 assert.deepEqual(staged.errors,[]);await staged.context.close();
 console.log('Prepared chat: 150ms quiet loading, hidden layout, disabled input, stable delayed image, learned ratio and latest selection passed');
}finally{await browser?.close();await new Promise(resolve=>server.close(resolve))}
