// Intrinsic sizes learned this session; never resize an already displayed frame.
const dimensions = new Map();
const keyFor = message => message.media_id || message.id;
function valid(width,height){return Number.isFinite(width)&&Number.isFinite(height)&&width>0&&height>0;}
export function rememberMediaSize(message,width,height){
  if(!valid(width,height))return;
  const key=keyFor(message);dimensions.delete(key);dimensions.set(key,{width,height});
  while(dimensions.size>256)dimensions.delete(dimensions.keys().next().value);
}
export function mediaFrameFor(message){
  const width=Number(message.media_width),height=Number(message.media_height);
  const size=valid(width,height)?{width,height}:dimensions.get(keyFor(message));
  const ratio=size?size.width/size.height:3/2;
  // Fit the original aspect ratio within a 420 × 360 preview; CSS can shrink it.
  const frameWidth=Math.min(420,360*ratio);
  return {ratio,width:frameWidth,known:Boolean(size)};
}
