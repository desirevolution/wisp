// Keep the range from the first unread message through the latest contiguous:
// using only a search context would leave a gap when more than 50 arrived.
export async function loadUnreadOpening({chatId,targetId,apiJson,isCurrent}){
 const path=`/api/chats/${encodeURIComponent(chatId)}/messages`;
 const latest=await apiJson(`${path}?limit=50`);
 if(!isCurrent())return null;
 const rows=new Map(latest.map(m=>[m.id,m]));
 if(targetId&&!rows.has(targetId)){
  try{await apiJson(`/api/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(targetId)}/context`)}
  catch(error){if(error.status===404)return {messages:latest,targetId:null};throw error}
 }
 let page=latest;
 while(isCurrent()&&targetId&&!rows.has(targetId)&&page.length){
  const first=page[0];
  page=await apiJson(`${path}?limit=200&localOnly=1&beforeTs=${encodeURIComponent(first.timestamp)}&beforeId=${encodeURIComponent(first.id)}`);
  let fresh=0;for(const m of page)if(!rows.has(m.id)){rows.set(m.id,m);fresh++}
  if(!fresh)break;
 }
 if(!isCurrent())return null;
 // A pruned/missing target should fall back to the latest page, not load an
 // entire old history into the view or highlight an unrelated message.
 if(!rows.has(targetId))return {messages:latest,targetId:null};
 return {messages:[...rows.values()].sort((a,b)=>a.timestamp-b.timestamp||a.id.localeCompare(b.id)),targetId};
}
