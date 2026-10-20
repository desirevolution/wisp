import {createOutbox} from '../core/outbox.js';
import {outboxStorage} from './outbox-storage.js';
export const sessionOutbox=createOutbox({storage:outboxStorage(),transport:async entry=>{
 let path='send',body,headers={'X-Wisp-Message-ID':entry.id};
 if(entry.image){path='send-image';body=new FormData();body.append('image',entry.image.file,entry.image.file.name||'image');body.append('caption',entry.text);body.append('reply_to',entry.reply?.message_id||'')}
 else{headers['content-type']='application/json';body=JSON.stringify({text:entry.text,reply_to:entry.reply?.message_id||''})}
 const response=await fetch(`/api/chats/${encodeURIComponent(entry.chatId)}/${path}`,{method:'POST',headers,body});
 const result=await response.json().catch(()=>null);
 if(!response.ok||result?.ok!==true)throw Error(result?.error||'Message could not be sent');
 return result;
}});
