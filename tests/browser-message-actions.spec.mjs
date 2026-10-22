import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.WISP_PLAYWRIGHT_MODULE||'playwright');
const root=resolve('public'),chat='actions@g.us';let base,mutations=0;
const rows=()=>[
 {id:'in',chat_id:chat,sender_name:'Bea',body:`Hello <world>\nSecond line ${base}/outside`,type:'chat',timestamp:1790760000,from_me:0,reactions:[]},
 {id:'out',chat_id:chat,body:'Outgoing message',type:'chat',timestamp:1790760010,from_me:1,reactions:[]},
 ...['caption','photo'].map(id=>({id,chat_id:chat,body:id==='caption'?'A photo caption':'',type:'image',has_media:1,media_id:'sample',media_status:'ready',media_width:240,media_height:120,timestamp:1790760020,from_me:0,reactions:[]}))
];
const server=createServer(async(req,res)=>{
 const u=new URL(req.url,'http://fixture'),json=data=>{res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify(data))};
 if(req.method==='POST'){mutations++;return json({})}
 if(u.pathname==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true}});
 if(u.pathname==='/api/config')return json({sendEnabled:true,mirrorOnly:false});
 if(u.pathname==='/api/chats')return json([{id:chat,name:'Actions test',type:'group'}]);
 if(u.pathname.includes('/messages'))return json(rows());
 if(u.pathname==='/api/media/sample'){res.writeHead(200,{'content-type':'image/svg+xml'});res.end('<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120"><rect width="240" height="120" fill="#268b77"/></svg>');return}
 if(u.pathname.startsWith('/api/'))return json([]);
 try{const path=resolve(root,'.'+(u.pathname==='/'?'/index.html':u.pathname));if(!path.startsWith(root+'/'))throw Error();res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml'}[extname(path)]||'application/octet-stream')});res.end(await readFile(path))}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));base=`http://127.0.0.1:${server.address().port}`;
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 const pageFor=async(width,mirror=false)=>{
  const context=await browser.newContext({viewport:{width,height:850},isMobile:width<701,hasTouch:width<701,serviceWorkers:'block'}),page=await context.newPage(),errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await context.addInitScript(()=>{
   window.EventSource=class extends EventTarget{close(){}};
   window.writes=[];window.rejectCopy=false;
   Object.defineProperty(navigator,'clipboard',{configurable:true,value:{
    writeText:async text=>{if(window.rejectCopy)throw Error('denied');window.writes.push({text})},
    write:async items=>{if(window.rejectCopy)throw Error('denied');const blob=await items[0].getType('image/png');window.writes.push({type:blob.type,header:[...new Uint8Array(await blob.arrayBuffer()).slice(0,8)],size:blob.size})}
   }});
  });
  if(mirror){await context.route('**/api/status',r=>r.fulfill({json:{whatsapp:{state:'connected'},send:{enabled:false}}}));await context.route('**/api/config',r=>r.fulfill({json:{sendEnabled:false,mirrorOnly:true}}))}
  page.setDefaultTimeout(8000);await page.goto(base);await page.locator(`#chatList [data-id="${chat}"]`).click();await page.locator('[data-message-id="photo"]').waitFor();
  await page.waitForFunction(()=>!document.querySelector('#app').classList.contains('chat-revealing'));
  return {context,page,errors};
 };
 const desktop=await pageFor(1200),p=desktop.page;
 const bubble=(page,id)=>page.locator(`[data-message-id="${id}"]`),row=(page,id)=>bubble(page,id).locator('..').locator('..');
 const more=async(page,id)=>{await row(page,id).hover();await row(page,id).getByRole('button',{name:'Message actions',exact:true}).click();await page.locator('.message-popover:popover-open').waitFor()};
 for(const id of ['in','out']){
  await p.locator('.sidebar-top').hover();await p.locator('#composerInput').focus();
  await p.waitForFunction(id=>getComputedStyle(document.querySelector(`[data-message-id="${id}"]`).closest('.message-row').querySelector('.message-actions')).opacity==='0',id);
  assert.equal(await bubble(p,id).getByRole('button',{name:'Message actions',exact:true}).isVisible(),false);
  const before=await bubble(p,id).boundingBox();await row(p,id).hover();
  const actions=row(p,id),react=await actions.getByRole('button',{name:'React to message',exact:true}).boundingBox(),menu=await bubble(p,id).getByRole('button',{name:'Message actions',exact:true}).boundingBox(),after=await bubble(p,id).boundingBox();
  assert.deepEqual(after,before,'Hover must not resize or move the message');
  assert.ok(menu.x+menu.width<=after.x+after.width&&menu.x+menu.width>=after.x+after.width-6&&menu.y>=after.y&&menu.y<after.y+6,JSON.stringify({menu,after}));
  assert.ok(id==='in'?react.x>=after.x+after.width:react.x+react.width<=after.x);
  assert.equal(await actions.locator('.message-actions button').count(),1);assert.equal(await actions.getByRole('button',{name:'Reply to message',exact:true}).count(),0);
  await p.locator('.sidebar-top').hover();await bubble(p,id).focus();await bubble(p,id).getByRole('button',{name:'Message actions',exact:true}).waitFor();
 }
 await more(p,'out');const dropdown=await p.locator('.message-options').boundingBox(),conversation=await p.locator('.conversation').boundingBox();assert.ok(dropdown.x>=conversation.x&&dropdown.x+dropdown.width<=conversation.x+conversation.width);assert.equal((await p.locator('.message-copy-actions button').allTextContents())[0].trim(),'Reply');
 await p.getByRole('button',{name:'Reply',exact:true}).click();await p.locator('.reply-draft').waitFor();assert.ok((await p.locator('.reply-draft').textContent()).includes('Outgoing message'));await p.locator('#composerInput').press('Escape');
 await more(p,'in');await p.getByRole('button',{name:'Copy text',exact:true}).click();await p.waitForFunction(()=>!document.querySelector('.message-popover'));
 assert.equal(await p.evaluate(()=>window.writes[0].text),rows()[0].body);assert.equal(await p.locator('#appToast').textContent(),'Text copied');
 await more(p,'caption');assert.ok(await p.getByRole('button',{name:'Copy text',exact:true}).isVisible());await p.getByRole('button',{name:'Copy image',exact:true}).click();await p.waitForFunction(()=>!document.querySelector('.message-popover'));
 const copied=await p.evaluate(()=>window.writes.at(-1));assert.equal(copied.type,'image/png');assert.deepEqual(copied.header,[137,80,78,71,13,10,26,10]);assert.ok(copied.size>20);
 await more(p,'photo');assert.equal(await p.getByRole('button',{name:'Copy text',exact:true}).count(),0);await p.keyboard.press('Escape');assert.equal(await p.evaluate(()=>document.activeElement.getAttribute('aria-label')),'Message actions');
 await p.evaluate(()=>window.rejectCopy=true);await more(p,'caption');await p.getByRole('button',{name:'Copy image',exact:true}).click();await p.getByRole('alert').waitFor();
 const download=p.waitForEvent('download');await p.getByRole('button',{name:'Download image',exact:true}).click();assert.ok((await download).suggestedFilename().startsWith('wisp-image-'));
 await p.evaluate(()=>{window.rejectCopy=false;navigator.clipboard.write=undefined});await more(p,'photo');await p.getByRole('button',{name:'Copy image',exact:true}).click();await p.getByRole('button',{name:'Download image',exact:true}).waitFor();await p.keyboard.press('Escape');
 await bubble(p,'in').focus();await p.keyboard.press('Shift+F10');await p.getByRole('button',{name:'Copy text',exact:true}).waitFor();await p.keyboard.press('Escape');
 if(process.env.WISP_ACTIONS_QA){await more(p,'caption');await p.screenshot({path:process.env.WISP_ACTIONS_QA+'-desktop.png'});await p.keyboard.press('Escape')}
 // Desktop keeps pointer hover feedback in the full reaction picker.
 await row(p,'in').hover();await row(p,'in').getByRole('button',{name:'React to message',exact:true}).click();
 await p.locator('.emoji-grid button').first().hover();
 assert.equal(await p.locator('.emoji-grid button').first().evaluate(el=>getComputedStyle(el).backgroundColor),'rgb(42, 57, 66)');
 await p.keyboard.press('Escape');
 assert.deepEqual(desktop.errors,[]);await desktop.context.close();
 // Verify the browser's actual clipboard API as well as injected failure paths.
 const clipboardContext=await browser.newContext({viewport:{width:1200,height:850},permissions:['clipboard-read','clipboard-write'],serviceWorkers:'block'});
 await clipboardContext.addInitScript(()=>{window.EventSource=class extends EventTarget{close(){}}});
 const clipboardPage=await clipboardContext.newPage();await clipboardPage.goto(base);await clipboardPage.locator(`#chatList [data-id="${chat}"]`).click();await clipboardPage.locator('[data-message-id="photo"]').waitFor();
 await more(clipboardPage,'in');await clipboardPage.getByRole('button',{name:'Copy text',exact:true}).click();await clipboardPage.waitForFunction(()=>!document.querySelector('.message-popover'));
 assert.equal(await clipboardPage.evaluate(()=>navigator.clipboard.readText()),rows()[0].body);
 await more(clipboardPage,'caption');await clipboardPage.getByRole('button',{name:'Copy image',exact:true}).click();await clipboardPage.waitForFunction(()=>!document.querySelector('.message-popover'));
 assert.equal(await clipboardPage.evaluate(async()=>{const items=await navigator.clipboard.read();return (await items[0].getType('image/png')).type}),'image/png');await clipboardContext.close();
 const mobile=await pageFor(390),m=mobile.page,touch=await mobile.context.newCDPSession(m);
 const press=async(id,{move=false,short=false,link=false}={})=>{
  const target=link?bubble(m,id).locator('a').first():bubble(m,id).locator('.message-text,img').first();await target.scrollIntoViewIfNeeded();const b=await target.boundingBox(),x=b.x+b.width/2,y=b.y+b.height/2;
  await touch.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x,y}]});
  if(move)await touch.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x,y:y-35}]});
  await m.waitForTimeout(short?80:520);await touch.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
 };
 assert.equal(await row(m,'in').locator('.message-actions').isVisible(),false);
 // Put the message exactly beneath the future first emoji, not merely slide a
 // touch from elsewhere. Android may synthesize a hover at the release point.
 await m.evaluate(()=>localStorage.removeItem('recentEmojis'));
 await bubble(m,'in').focus();await m.keyboard.press('Shift+F10');
 const overlapBox=await m.locator('.quick-reactions button').first().boundingBox();
 const overlap={x:overlapBox.x+overlapBox.width/2,y:overlapBox.y+overlapBox.height/2};
 await m.keyboard.press('Escape');
 await bubble(m,'in').evaluate((el,point)=>{
  const text=el.querySelector('.message-text').getBoundingClientRect();
  el.style.transform=`translate(${point.x-text.x-12}px,${point.y-text.y-8}px)`;
 },overlap);
 assert.equal(await m.evaluate(point=>Boolean(document.elementFromPoint(point.x,point.y)?.closest('[data-message-id="in"]')),overlap),true);
 await touch.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[overlap]});
 await m.locator('.message-sheet:popover-open').waitFor();
 const openedBox=await m.locator('.quick-reactions button').first().boundingBox();
 assert.ok(overlap.x>=openedBox.x&&overlap.x<=openedBox.x+openedBox.width&&overlap.y>=openedBox.y&&overlap.y<=openedBox.y+openedBox.height,'Opening touch must overlap the emoji');
 await touch.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
 await m.waitForFunction(()=>document.querySelector('.message-sheet')&&!document.querySelector('.message-sheet').inert);
 // Compatibility mouse movement can leave :hover sticky on touch-only browsers.
 await touch.send('Input.dispatchMouseEvent',{type:'mouseMoved',x:overlap.x,y:overlap.y});
 const overlapEmoji=m.locator('.quick-reactions button').first();
 assert.equal(await overlapEmoji.getAttribute('aria-pressed'),'false');
 assert.equal(await overlapEmoji.evaluate(el=>getComputedStyle(el).backgroundColor),'rgba(0, 0, 0, 0)','A release-generated hover must not look like a selected reaction on mobile');
 assert.equal(await m.evaluate(()=>window.getSelection().toString()),'');
 // A real existing reaction retains its selection appearance on mobile.
 await overlapEmoji.evaluate(el=>el.setAttribute('aria-pressed','true'));
 assert.equal(await overlapEmoji.evaluate(el=>getComputedStyle(el).backgroundColor),'rgb(18, 93, 76)');
 await m.keyboard.press('Escape');await bubble(m,'in').evaluate(el=>el.style.removeProperty('transform'));

 // Keep a real touch down while the sheet appears, then slide over its emoji.
 // Release must not activate, select, or dismiss newly inserted content.
 await m.evaluate(()=>{
  window.holdReactions=[];
  window.holdReactionHandler=e=>{window.holdReactions.push(e.detail.emoji);e.stopImmediatePropagation()};
  document.querySelector('#messages').addEventListener('reaction-set',window.holdReactionHandler,true);
 });
 for(const target of ['.quick-reactions button','.sheet-message-preview span','.message-sheet-backdrop']){
  await bubble(m,'in').scrollIntoViewIfNeeded();
  const start=await bubble(m,'in').locator('.message-text').boundingBox();
  await touch.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:start.x+20,y:start.y+8}]});
  await m.locator('.message-sheet:popover-open').waitFor();
  assert.equal(await m.locator('.message-sheet').evaluate(el=>el.inert),true,'The original touch must not interact with the new sheet');
  assert.equal(await m.locator('.sheet-message-preview span').evaluate(el=>getComputedStyle(el).userSelect),'none');
  const destination=await m.locator(target).first().boundingBox();
  const x=target.includes('backdrop')?10:destination.x+destination.width/2,y=target.includes('backdrop')?30:destination.y+destination.height/2;
  await touch.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x,y}]});
  await touch.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
  await m.waitForFunction(()=>document.querySelector('.message-sheet')&&!document.querySelector('.message-sheet').inert);
  // Exercise a delayed compatibility click after touchend as well. It can be
  // delivered to a different top-layer target by mobile browser versions.
  assert.equal(await m.locator(target).first().evaluate(el=>el.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true,detail:1}))),false);
  assert.equal(await m.locator('.message-sheet').count(),1);
  assert.equal(await m.evaluate(()=>window.getSelection().toString()),'');
  assert.deepEqual(await m.evaluate(()=>window.holdReactions),[]);
  assert.equal(await m.locator('.reply-draft').count(),0);
  // A separate fresh tap is accepted immediately, without an arbitrary delay.
  await m.locator('.quick-reactions button').first().tap();
  await m.waitForFunction(()=>!document.querySelector('.message-sheet'));
  assert.equal(await m.evaluate(()=>window.holdReactions.length),1);
  await m.evaluate(()=>window.holdReactions=[]);
 }
 await m.evaluate(()=>document.querySelector('#messages').removeEventListener('reaction-set',window.holdReactionHandler,true));

 await m.evaluate(()=>localStorage.removeItem('recentEmojis'));await press('in');await m.locator('.message-sheet:popover-open').waitFor();
 assert.deepEqual(await m.locator('.quick-reactions button').allTextContents(),['👍','❤️','😂','😮','😢','🙏']);assert.equal(await m.getByRole('button',{name:'Reply',exact:true}).count(),0);
 await m.getByRole('button',{name:'More emojis',exact:true}).click();assert.ok(await m.locator('.sheet-reactions wisp-emoji-picker').isVisible());
 const fullEmoji=await m.locator('.sheet-reactions .emoji-grid button').first().boundingBox();
 await touch.send('Input.dispatchMouseEvent',{type:'mouseMoved',x:fullEmoji.x+fullEmoji.width/2,y:fullEmoji.y+fullEmoji.height/2});
 assert.equal(await m.locator('.sheet-reactions .emoji-grid button').first().evaluate(el=>getComputedStyle(el).backgroundColor),'rgba(0, 0, 0, 0)');
 await m.getByRole('button',{name:'Back to reactions',exact:true}).click();
 await m.getByRole('button',{name:'More message actions',exact:true}).click();await m.getByRole('button',{name:'Copy text',exact:true}).click();await m.waitForFunction(()=>!document.querySelector('.message-popover'));assert.equal(await m.evaluate(()=>window.writes.at(-1).text),rows()[0].body);
 await m.evaluate(()=>localStorage.setItem('recentEmojis',JSON.stringify(['🔥','😂','🔥'])));await press('caption');await m.locator('.message-sheet').waitFor();assert.deepEqual(await m.locator('.quick-reactions button').allTextContents(),['🔥','😂','👍','❤️','😮','😢']);
 assert.ok(await m.locator('#imageViewer').isHidden());await m.getByRole('button',{name:'More message actions',exact:true}).click();assert.ok(await m.getByRole('button',{name:'Copy image',exact:true}).isVisible());
 await m.getByRole('button',{name:'Back to reactions',exact:true}).click();if(process.env.WISP_ACTIONS_QA)await m.screenshot({path:process.env.WISP_ACTIONS_QA+'-mobile.png'});await m.keyboard.press('Escape');
 await press('caption',{short:true});await m.locator('#imageViewer:not([hidden])').waitFor();await m.getByRole('button',{name:'Back to chat',exact:true}).click();await m.waitForFunction(()=>document.querySelector('#imageViewer').hidden);
 await press('in',{link:true});await m.locator('.message-sheet').waitFor();assert.equal(m.url(),base+'/');assert.equal(mobile.context.pages().length,1);await m.keyboard.press('Escape');
 await press('in',{move:true});assert.equal(await m.locator('.message-sheet').count(),0);
 await press('in');await m.getByRole('button',{name:'More message actions',exact:true}).click();await m.getByRole('button',{name:'Reply',exact:true}).click();await m.locator('.reply-draft').waitFor();assert.equal(await m.locator('.message-sheet').count(),0);
 assert.deepEqual(mobile.errors,[]);await mobile.context.close();
 for(const width of [1200,390]){
  const readonly=await pageFor(width,true),r=readonly.page;
  assert.equal(await r.locator('#composer').isVisible(),false);assert.equal(await r.locator('[aria-label="Reply to message"],[aria-label="React to message"]').count(),0);
  if(width>700)await more(r,'caption');else{await bubble(r,'caption').focus();await r.keyboard.press('Shift+F10')}
  assert.ok(await r.getByRole('button',{name:'Copy text',exact:true}).isVisible());assert.ok(await r.getByRole('button',{name:'Copy image',exact:true}).isVisible());assert.equal(await r.getByRole('button',{name:'Reply',exact:true}).count(),0);
  await r.getByRole('button',{name:'Copy text',exact:true}).click();await r.waitForFunction(()=>!document.querySelector('.message-popover'));assert.equal(await r.evaluate(()=>window.writes.at(-1).text),'A photo caption');
  assert.deepEqual(readonly.errors,[]);await readonly.context.close();
 }
 assert.equal(mutations,0,'Copy actions must not mutate WhatsApp or send messages');
 console.log('Message actions: desktop reaction plus top-right bubble menu, Reply first; text and PNG clipboard; download fallback; real touch hold/release over emoji/text/backdrop, immediate next tap, scroll/link; compact/recent/full/options sheets; keyboard and mirror mode passed');
}finally{await browser?.close();server.closeAllConnections();await new Promise(done=>server.close(done))}
