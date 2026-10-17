// Fictional English data. No WhatsApp account, credentials or user data required.
export const DEMO_NOW = '2026-10-17T12:15:00Z';
export const DEMO_CHAT = 'homelab@g.us';
const stamp = time => Date.parse(`2026-10-17T${time}:00Z`) / 1000;
const message = (id,time,body,extra={}) => ({id,chat_id:DEMO_CHAT,sender_id:'mira',sender_name:'Mira',body,type:'chat',timestamp:stamp(time),from_me:0,reactions:[],...extra});
export const DEMO_MESSAGES = [
  message('demo-1','12:00','Anyone free this weekend to tidy up our homelab?',{
    reactions:[{actor_id:'sam',actor_name:'Sam',emoji:'❤️'},{actor_id:'lea',actor_name:'Lea',emoji:'❤️'}],reaction_version:1,
  }),
  message('demo-2','12:02',"Saturday at 2 pm works for me. I'll bring coffee.",{
    from_me:1,sender_id:'me',sender_name:'You',
    reply:{message_id:'demo-1',chat_id:DEMO_CHAT,sender_name:'Mira',body:'Anyone free this weekend to tidy up our homelab?',type:'chat'},
  }),
  message('demo-3','12:04',"Small change: let's start at 3 pm.",{
    sender_id:'jonas',sender_name:'Jonas',edited:true,edit_original_available:true,
    edit_history:[{body:"Let's start at 2 pm.",original:true,timestamp_ms:stamp('12:03')*1000}],
  }),
  message('demo-4','12:07',"Here's the new dashboard. What do you think?",{
    sender_id:'lea',sender_name:'Lea',type:'document',has_media:true,media_status:'ready',media_id:'demo-document',filename:'Homelab overview.pdf',
  }),
  message('demo-5','12:09','Looks great! Wisp and JumpKey are already up and running.',{from_me:1,sender_id:'me',sender_name:'You'}),
  message('demo-6','12:10','Perfect. Only the backups are left. Sam, could you take care of those?'),
  message('demo-7','12:11',"I'll bring the spare SSD."),
];
export const DEMO_CHATS = [
  {id:DEMO_CHAT,name:'Homelab & Coffee',type:'group',last_body:DEMO_MESSAGES.at(-1).body,last_sender_name:'Mira',last_type:'chat',last_message_at:stamp('12:11')},
  {id:'mira@lid',name:'Mira',type:'private',last_body:'Thanks, see you on Saturday!',last_type:'chat',last_message_at:stamp('12:08')},
  {id:'weekend@g.us',name:'Weekend plans',type:'group',last_body:"Who's bringing something for the barbecue?",last_sender_name:'Lea',last_type:'chat',last_message_at:stamp('12:07')},
  {id:'jonas@lid',name:'Jonas',type:'private',last_body:'The new version is running smoothly.',last_type:'chat',last_message_at:stamp('12:05')},
  {id:'neighbours@g.us',name:'Neighbours',type:'group',last_body:"I've got your parcel.",last_sender_name:'Sam',last_type:'chat',last_message_at:stamp('12:04')},
  {id:'lea@lid',name:'Lea',type:'private',last_body:'The photos look amazing!',last_type:'chat',last_message_at:stamp('12:03')},
];
