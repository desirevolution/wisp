import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
let enabled=false,calls=0,release;
const root=resolve('public'),streams=new Set(),errors=[];
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://local').pathname;
 const json=value=>{res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify(value))};
 if(path==='/api/events'){res.writeHead(200,{'content-type':'text/event-stream'});res.write(': ready\n\n');streams.add(res);req.on('close',()=>streams.delete(res));return}
 if(path==='/api/status')return json({whatsapp:{state:'connected'},send:{enabled:true},push:{test_enabled:enabled,subscriptions:2}});
 if(path==='/api/config')return json({sendEnabled:true});
 if(path==='/api/chats')return json([]);
 if(path==='/api/push/public-key')return json({publicKey:''});
 if(path==='/api/push/test'){assert.equal(req.method,'POST');calls++;await new Promise(done=>{release=done});return json({test_id:'test-browser',total:2,accepted:1,expired:1,failed:0,results:[{device:'12345678',provider:'push.example',result:'accepted',http_status:201}]})}
 try{const file=resolve(root,'.'+(path==='/'?'/index.html':path));if(!file.startsWith(root+'/'))throw Error();res.writeHead(200,{'content-type':({'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.webmanifest':'application/manifest+json'}[extname(file)]||'application/octet-stream')});res.end(await readFile(file))}catch{res.end()}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser;
try{
 browser=await chromium.launch({headless:true,executablePath:process.env.WISP_CHROME||undefined,args:['--no-sandbox']});
 for(const width of [1200,390]){
  enabled=false;const page=await browser.newPage({viewport:{width,height:844}});page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  await page.getByRole('button',{name:'Open settings'}).click();await page.getByRole('dialog',{name:'Settings',exact:true}).waitFor();
  assert.equal(await page.locator('.push-test-panel').count(),0);
  await page.getByRole('button',{name:'Close settings'}).click();enabled=true;
  await page.getByRole('button',{name:'Open settings'}).click();
  const button=page.getByRole('button',{name:'Send test to all devices'});await button.click();
  await page.waitForFunction(()=>document.querySelector('.push-test-panel button')?.disabled);
  assert.equal(calls,width===1200?1:2);release();
  await page.getByRole('status').filter({hasText:'1 of 2 accepted'}).waitFor();
  assert.ok(await page.locator('.push-test-results').textContent().then(t=>t.includes('does not confirm a visible notification')));
  const panel=await page.locator('.push-test-panel').boundingBox();assert.ok(panel.x>=0&&panel.x+panel.width<=width+1);
  assert.ok(await page.locator('.modal').evaluate(el=>el.scrollWidth<=el.clientWidth+1));
  await page.close();
 }
 assert.deepEqual(errors,[]);console.log('Push broadcast browser passed: desktop/mobile capability gate, busy state, provider results and layout.');
}finally{release?.();await browser?.close();for(const res of streams)res.end();server.closeAllConnections();await new Promise(done=>server.close(done))}
