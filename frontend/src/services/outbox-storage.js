// IndexedDB retains image Blobs as well as text/reply snapshots across reloads.
// When unavailable, retain text/reply snapshots in the same device's local storage.
export function outboxStorage(){
 let database;
 const open=()=>database??=new Promise((resolve,reject)=>{
  if(!globalThis.indexedDB){reject(Error('No IndexedDB'));return}
  const request=indexedDB.open('wisp-outbox-v1',1);
  request.onupgradeneeded=()=>request.result.createObjectStore('messages',{keyPath:'id'});
  request.onsuccess=()=>resolve(request.result);request.onerror=()=>reject(request.error);request.onblocked=()=>reject(Error('Outbox storage blocked'));
 });
 const fallback=()=>{try{return JSON.parse(localStorage.getItem('wisp.outbox.v1')||'[]')}catch{return []}};
 const fallbackWrite=rows=>{try{localStorage.setItem('wisp.outbox.v1',JSON.stringify(rows))}catch{/* memory remains usable */}};
 async function request(mode,action){const db=await open();return new Promise((resolve,reject)=>{const tx=db.transaction('messages',mode),req=action(tx.objectStore('messages'));tx.oncomplete=()=>resolve(req.result);tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error)})}
 return {
  load:async()=>{try{return [...fallback(),...await request('readonly',s=>s.getAll())]}catch{return fallback()}},
  put:async entry=>{const copy={...entry,delivered:false};if(!entry.image)fallbackWrite([...fallback().filter(e=>e.id!==entry.id),copy]);try{await request('readwrite',s=>s.put(copy))}catch{/* fallback or memory */}},
  remove:async id=>{fallbackWrite(fallback().filter(e=>e.id!==id));try{await request('readwrite',s=>s.delete(id))}catch{/* fallback already removed */}}
 };
}
