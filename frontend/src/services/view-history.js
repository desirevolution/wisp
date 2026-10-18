// Exactly one chat entry above the overview; switching chats replaces that entry.
// The browser owns Back from the overview, so the app never traps navigation out.
export function createViewHistory({ win = window, onChange }) {
  const marker='wisp-view-v1';
  const overview=()=>({wispView:marker,view:'overview'});
  const valid=s=>s?.wispView===marker&&['overview','chat','image'].includes(s.view);
  const current=()=>valid(win.history.state)?win.history.state:overview();
  if(!valid(win.history.state))win.history.replaceState(overview(),'');
  let pendingChat=null;
  const pop=()=>{
    if(pendingChat!==null){win.history.replaceState({wispView:marker,view:'chat',chatId:pendingChat},'');pendingChat=null}
    onChange(current());
  };
  win.addEventListener('popstate',pop);
  return {
    current,
    openOverlay(mode){if(!current().overlay)win.history.pushState({...current(),overlay:mode},'')},
    closeOverlay(){if(current().overlay){win.history.back();return true}return false},
    openChat(chatId,{preserveImage=false}={}){
      const previous=current();
      if(preserveImage&&previous.overlay&&previous.chatId===chatId)return;
      if(previous.view==='image'){
        if(preserveImage&&previous.chatId===chatId)return;
        pendingChat=chatId;win.history.back();return;
      }
      const next={wispView:marker,view:'chat',chatId};
      if(previous.view==='overview')win.history.pushState(next,'');
      else win.history.replaceState(next,'');
    },
    openImage(image,chatId){
      const next={wispView:marker,view:'image',chatId,image};
      if(current().view==='image')win.history.replaceState(next,'');
      else win.history.pushState(next,'');
    },
    closeImage(){if(current().view==='image'){win.history.back();return true}return false},
    toOverview(){
      const view=current().view;
      if(view==='overview')return false;
      win.history.go(view==='image'?-2:-1);return true;
    },
    destroy(){win.removeEventListener('popstate',pop)}
  };
}
