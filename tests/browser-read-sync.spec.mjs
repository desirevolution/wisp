import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
let mode='wisp',reject=false;
const root=resolve('public'),streams=new Set(),errors=[];
const config=()=>({sendEnabled:true,readMode:mode,followNewMessages:false});
const emit=(name,data)=>{for(const res of streams)res.write(`event: ${name}\ndata: ${JSON.stringify(data)}\n\n`)};
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://local').pathname;
 const json=(value,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(value))};
 if(path==='/api/events'){res.writeHead(200,{'content-type':'text/event-stream'});res.write(': ready\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return}
 if(path==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true},push:{},chat_states:[]});
 if(path==='/api/config'){
  if(req.method==='POST'){
   let raw='';for await(const chunk of req)raw+=chunk;
   if(reject)return json({error:'Could not save read mode'},500);
   mode=JSON.parse(raw).readMode;emit('config',config());
  }
  return json(config());
 }
 if(path==='/api/chats')return json([]);
 if(path==='/api/push/public-key')return json({publicKey:''});
 if(path.startsWith('/api/'))return json([]);
 try{const file=resolve(root,'.'+(path==='/'?'/index.html':path));if(!file.startsWith(root+'/'))throw Error();const data=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.webmanifest':'application/manifest+json'}[extname(file)]||'application/octet-stream')});res.end(data)}catch{res.writeHead(404);res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 for(const width of [1200,390,320]){
  mode='wisp';reject=false;
  const page=await browser.newPage({viewport:{width,height:844}});page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  await page.getByRole('button',{name:'Open settings'}).click();
  const select=page.getByRole('combobox',{name:'When reading in Wisp'});
  await select.waitFor();await page.waitForFunction(()=>document.querySelector('wisp-settings select')?.value==='wisp');
  await select.selectOption('devices');
  await page.waitForFunction(()=>document.querySelector('wisp-settings')?.readMode==='devices'&&!document.querySelector('wisp-settings select')?.disabled);
  assert.equal(mode,'devices');
  assert.ok((await page.locator('.read-mode-hint').textContent()).includes('without sending a normal read receipt'));
  reject=true;await select.selectOption('receipts');
  await page.waitForFunction(()=>!document.querySelector('wisp-settings select')?.disabled&&document.querySelector('wisp-settings select')?.value==='devices');
  assert.equal(mode,'devices');reject=false;
  // A setting changed by another Wisp client appears without a page reload.
  mode='receipts';emit('config',config());
  await page.waitForFunction(()=>document.querySelector('wisp-settings select')?.value==='receipts');
  assert.ok(await page.locator('.modal').evaluate(el=>el.scrollWidth<=el.clientWidth+1));
  const box=await select.boundingBox();assert.ok(box.x>=0&&box.x+box.width<=width);
  await page.getByRole('button',{name:'Close settings'}).click();
  await page.reload();await page.getByRole('button',{name:'Open settings'}).click();
  await page.waitForFunction(()=>document.querySelector('wisp-settings select')?.value==='receipts');
  await page.close();
 }
 assert.deepEqual(errors,[]);console.log('Read sync settings browser passed: desktop/mobile modes, save failure, SSE updates, reload and narrow layout.');
}finally{await browser?.close();for(const res of streams)res.end();server.closeAllConnections();await new Promise(done=>server.close(done))}
