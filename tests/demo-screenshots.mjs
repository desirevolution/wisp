// Build first, or run `npm run screenshots`. This is an offline visual smoke test.
import {createServer} from 'node:http';
import {readFile,mkdir,stat} from 'node:fs/promises';
import {resolve,relative,isAbsolute,extname,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
import {DEMO_CHAT,DEMO_CHATS,DEMO_MESSAGES,DEMO_NOW} from './fixtures/demo-data.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const root=resolve(project,'public');
const args=process.argv.slice(2);
if(args.includes('--help')){
  console.log('Usage: npm run screenshots -- [--output-dir DIRECTORY]\nDefault: screenshots/wisp-desktop.png and screenshots/wisp-mobile.png\nOptional: WISP_CHROME points to an existing Chromium executable.');
  process.exit(0);
}
if(args.length && !(args.length===2 && args[0]==='--output-dir'))throw Error('Expected --output-dir DIRECTORY. Use --help.');
const output=resolve(project,args[1]||'screenshots');
try{await stat(resolve(root,'index.html'))}catch{throw Error('Frontend build missing. Run npm run screenshots to build and capture.');}
const {chromium}=await import(process.env.WISP_PLAYWRIGHT_MODULE||'playwright');
const mime={'.html':'text/html','.js':'application/javascript','.css':'text/css','.webmanifest':'application/manifest+json','.svg':'image/svg+xml','.png':'image/png'};
const server=createServer(async(req,res)=>{
  const url=new URL(req.url,'http://demo');
  const json=(value,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(value))};
  if(req.method!=='GET')return json({error:'Demo is read-only'},405);
  if(url.pathname==='/api/status')return json({whatsapp:{state:'connected',account:'Demo account'},send:{enabled:true}});
  if(url.pathname==='/api/config')return json({sendEnabled:true,mirrorOnly:false,followNewMessages:false});
  if(url.pathname==='/api/chats')return json(DEMO_CHATS);
  const parts=decodeURIComponent(url.pathname).split('/');
  if(parts[1]==='api'&&parts[2]==='chats'&&parts[3]===DEMO_CHAT){
    if(parts[4]==='messages')return json(url.searchParams.has('beforeTs')?[]:DEMO_MESSAGES);
    if(parts[4]==='reactions')return json(DEMO_MESSAGES.map(m=>({chat_id:DEMO_CHAT,message_id:m.id,reactions:m.reactions,version:m.reaction_version||0})));
    if(['edits','deletions','media','links'].includes(parts[4]))return json([]);
  }
  if(url.pathname.startsWith('/api/'))return json({error:'Unknown demo endpoint'},404);
  try{
    const file=resolve(root,'.'+decodeURIComponent(url.pathname==='/'?'/index.html':url.pathname));
    const rel=relative(root,file);
    if(rel.startsWith('..')||isAbsolute(rel))return json({error:'Not found'},404);
    const data=await readFile(file);res.writeHead(200,{'content-type':mime[extname(file)]||'application/octet-stream','cache-control':'no-store'});res.end(data);
  }catch{json({error:'Not found'},404)}
});
let browser;
try{
  await mkdir(output,{recursive:true});
  await new Promise((done,fail)=>{server.once('error',fail);server.listen(0,'127.0.0.1',done)});
  const base=`http://127.0.0.1:${server.address().port}`;
  browser=await chromium.launch({headless:true,...(process.env.WISP_CHROME?{executablePath:process.env.WISP_CHROME}:{})});
  for(const profile of [
    {name:'desktop',viewport:{width:1440,height:1000},deviceScaleFactor:1},
    {name:'mobile',viewport:{width:430,height:932},deviceScaleFactor:2,isMobile:true,hasTouch:true},
  ]){
    const {name,...device}=profile;
    const context=await browser.newContext({...device,locale:'en-GB',timezoneId:'UTC',colorScheme:'dark',reducedMotion:'reduce',serviceWorkers:'block'});
    try{
      // Isolated contexts prevent persisted drafts/widths/emoji from affecting captures.
      await context.route('**/*',route=>new URL(route.request().url()).origin===base?route.continue():route.abort());
      await context.addInitScript(()=>{
        window.EventSource=class extends EventTarget{
          constructor(){super();queueMicrotask(()=>this.dispatchEvent(new Event('open')))}
          close(){}
        };
      });
      const page=await context.newPage(),errors=[],httpErrors=[];
      page.on('pageerror',error=>errors.push(error.message));
      page.on('response',response=>{if(response.status()>=400)httpErrors.push(`${response.status()} ${response.url()}`)});
      await page.clock.setFixedTime(new Date(DEMO_NOW));
      await page.goto(base);
      await page.locator(`[data-id="${DEMO_CHAT}"]`).click();
      await page.locator('[data-message-id="demo-7"]').waitFor();
      await page.locator('#messages .reaction-chips').waitFor();
      await page.evaluate(()=>document.fonts.ready);
      // Wait for the real initial scroll; do not add arbitrary sleep delays.
      await page.waitForFunction(()=>{
        const el=document.querySelector('#messages');
        return el&&el.scrollHeight-el.scrollTop-el.clientHeight<2;
      });
      await page.mouse.move(1,1);
      await page.locator('#composerInput').blur();
      assert.equal(await page.locator('#conversationTitle').textContent(),'Homelab & Coffee');
      assert.equal(await page.locator('#messages [data-message-id]').count(),DEMO_MESSAGES.length);
      assert.ok(await page.locator('#composerInput').isVisible());
      assert.ok(await page.locator('[data-message-id="demo-7"]').isVisible());
      assert.equal(await page.locator('#messages .grouped').count(),1);
      if(name==='mobile')assert.notEqual(await page.evaluate(()=>document.activeElement?.id),'composerInput');
      const file=resolve(output,`wisp-${name}.png`);
      await page.screenshot({path:file,fullPage:false,animations:'disabled',caret:'hide'});
      assert.deepEqual(errors,[],'Browser errors');assert.deepEqual(httpErrors,[],'Failed demo requests');
      const png=await readFile(file);
      assert.ok(png.length>10000,'Screenshot should not be empty');
      assert.equal(png.readUInt32BE(16),device.viewport.width*device.deviceScaleFactor,'PNG width');
      assert.equal(png.readUInt32BE(20),device.viewport.height*device.deviceScaleFactor,'PNG height');
      console.log(`${name}: ${file}`);
    }finally{await context.close()}
  }
  console.log('Demo screenshot smoke test passed (English, fixed time, no WhatsApp).');
}finally{
  await browser?.close();
  if(server.listening)await new Promise(done=>server.close(done));
}
