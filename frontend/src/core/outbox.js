// Snapshots belong to the outbox, never to the next editable draft.
export function createOutbox({transport,storage=null,newId=()=>`3EB0${[...crypto.getRandomValues(new Uint8Array(9))].map(b=>b.toString(16).padStart(2,'0')).join('').toUpperCase()}`,now=()=>Date.now()}={}){
 const entries=new Map(),revisions=new Map(),listeners=new Set(),tails=new Map();let writes=Promise.resolve();
 const notify=(type,entry)=>{for(const fn of listeners)fn({type,entry})};
 const save=entry=>{if(storage)writes=writes.then(()=>entries.has(entry.id)?storage.put({...entry,image:entry.image?{file:entry.image.file,width:entry.image.width,height:entry.image.height}:null}):storage.remove(entry.id)).catch(()=>{});return writes};
 const ready=Promise.resolve(storage?.load?.()||[]).then(rows=>{for(const saved of rows){if(saved.status!=='confirmed'&&(saved.serverRevision||0)<(revisions.get(saved.chatId)||0)){void storage?.remove(saved.id);continue}if(entries.has(saved.id))continue;const entry={...saved,status:saved.status==='confirmed'?'confirmed':'failed',error:saved.status==='confirmed'?'':'Send status unknown after reopening. Retry to check or send again.',delivered:false};if(entry.image?.file)entry.image={...entry.image,url:URL.createObjectURL(entry.image.file)};entries.set(entry.id,entry)}notify('restored',null)}).catch(()=>{});
 function remove(id){const entry=entries.get(id);if(!entry)return;entries.delete(id);if(entry.image?.url)URL.revokeObjectURL(entry.image.url);void save(entry)}
 async function run(entry){
  await ready;if(!entries.has(entry.id))return false;
  entry.status='sending';entry.error='';notify('changed',entry);await save(entry);
  if(!entries.has(entry.id))return false;
  try{const result=await transport(entry);entry.serverId=result.id||entry.id;entry.status='confirmed';entry.error='';if(entries.has(entry.id)){void save(entry);notify('acknowledged',entry)}return true}
  catch(error){if(entry.delivered)return true;if(entries.has(entry.id)){entry.status='failed';entry.error=error.message||'Message could not be sent';void save(entry);notify('changed',entry)}return false}
 }
 function schedule(entry){const previous=tails.get(entry.chatId)||Promise.resolve();const pending=previous.then(()=>run(entry));tails.set(entry.chatId,pending);void pending.finally(()=>{if(tails.get(entry.chatId)===pending)tails.delete(entry.chatId)});return pending}
 function enqueue({chatId,text,reply=null,image=null,serverRevision=0}){
  const entry={id:newId(),chatId,text,reply,serverRevision,image:image?{...image,url:URL.createObjectURL(image.file)}:null,status:'queued',createdAt:now(),error:'',delivered:false};
  entries.set(entry.id,entry);void save(entry);notify('enqueued',entry);return {entry,done:schedule(entry)};
 }
 function retry(id){const entry=entries.get(id);if(!entry||entry.status!=='failed')return Promise.resolve(false);entry.status='queued';entry.error='';notify('changed',entry);void save(entry);return schedule(entry)}
 function observe(message){for(const entry of entries.values())if(entry.chatId===message.chat_id&&(entry.serverId||entry.id)===message.id&&message.from_me){entry.delivered=true;entry.status='confirmed';void save(entry)}}
 function merge(chatId,rows){
  const result=[...rows];
  for(const entry of [...entries.values()]){
   if(entry.chatId!==chatId)continue;
   const server=rows.find(m=>m.id===(entry.serverId||entry.id));
   if(server){entry.delivered=true;entry.status='confirmed';if(server.deleted||!entry.image){remove(entry.id);continue}}
   const local={...(server||{}),id:entry.serverId||entry.id,chat_id:chatId,from_me:1,sender_name:'You',type:entry.image?'image':'chat',body:entry.text,timestamp:Math.floor(entry.createdAt/1000),reply:entry.reply?{...entry.reply,from_me:entry.reply.sender_name==='You'}:null,has_media:entry.image?1:0,media_status:entry.image?'ready':null,media_id:server?.media_id||entry.serverId||entry.id,media_width:server?.media_width||entry.image?.width,media_height:server?.media_height||entry.image?.height,server_image_ready:Boolean(server?.media_status==='ready'),local_image_url:entry.image?.url,reactions:[],outbox:{id:entry.id,status:entry.status,error:entry.error}};
   if(server)result[result.indexOf(server)]=local;else result.push(local);
  }
  return result.sort((a,b)=>a.timestamp-b.timestamp);
 }
 function invalidate(chat,revision){if(!Number.isSafeInteger(revision)||revision<=(revisions.get(chat)||0))return;revisions.set(chat,revision);for(const e of [...entries.values()])if(e.chatId===chat&&e.status!=='confirmed'&&(e.serverRevision||0)<revision){remove(e.id);notify('changed',e)}}
 function completePreview(id){const entry=entries.get(id);if(entry?.image&&entry.delivered){remove(id);notify('changed',entry)}}
 return {ready,enqueue,retry,observe,merge,invalidate,completePreview,get:id=>entries.get(id),list:chat=>[...entries.values()].filter(e=>!chat||e.chatId===chat),discardChat:chat=>{for(const e of [...entries.values()])if(e.chatId===chat&&e.status!=='confirmed'){remove(e.id);notify('changed',e)}},subscribe:fn=>{listeners.add(fn);return()=>listeners.delete(fn)}};
}
