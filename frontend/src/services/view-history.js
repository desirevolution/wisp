// Exactly one chat entry above the overview; switching chats replaces that entry.
// The browser owns Back from the overview, so the app never traps navigation out.
export function createViewHistory({ win = window, onChange }) {
  const marker='wisp-view-v1';
  const overview=()=>({wispView:marker,view:'overview'});
  const valid=s=>s?.wispView===marker&&['overview','chat','gallery','image'].includes(s.view);
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
    replaceOverlay(mode){win.history.replaceState({...current(),overlay:mode},'')},
    closeOverlay(){if(current().overlay){win.history.back();return true}return false},
    openChat(chatId,{preserveImage=false}={}){
      const previous=current();
      if(preserveImage&&previous.overlay&&previous.chatId===chatId)return;
      if(previous.view==='image'||previous.view==='gallery'){
        if(preserveImage&&previous.chatId===chatId)return;
        pendingChat=chatId;win.history.back();return;
      }
      const next={wispView:marker,view:'chat',chatId};
      if(previous.view==='overview')win.history.pushState(next,'');
      else win.history.replaceState(next,'');
    },
    openGallery(chatId,kind='media'){
      const next={wispView:marker,view:'gallery',chatId,kind};
      if(current().view==='gallery')win.history.replaceState(next,'');
      else win.history.pushState(next,'');
    },
    closeGallery(){if(current().view==='gallery'){win.history.back();return true}return false},
    openImage(image,chatId){
      const next={wispView:marker,view:'image',chatId,image};
      if(current().view==='image')win.history.replaceState(next,'');
      else win.history.pushState(next,'');
    },
    closeImage(){if(current().view==='image'){win.history.back();return true}return false},
    toOverview(){
      const view=current().view;
      if(view==='overview')return false;
      win.history.go(['image','gallery'].includes(view)?-2:-1);return true;
    },
    destroy(){win.removeEventListener('popstate',pop)}
  };
}
