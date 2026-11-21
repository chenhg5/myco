/** Input planning only: no bridge, DOM or native configuration. */
export type RemoteInput = Record<string, unknown>;
export function textInput(text: string, unicode: boolean, held: string[] = []): RemoteInput[] | null {
  if (!text) return [];
  if ([...text].length > 256 || new TextEncoder().encode(text).length > 1024 || /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/.test(text)) return null;
  if (unicode && !held.length) {
    const out:RemoteInput[]=[];let chunk="";
    const flush=()=>{if(chunk){out.push({kind:"text",text:chunk});chunk="";}};
    for(const c of text.replace(/\r\n?/g,"\n")) {
      if(c==="\n"||c==="\t"){flush();const code=c==="\n"?"Enter":"Tab";out.push({kind:"key",code,down:true},{kind:"key",code,down:false});}else chunk+=c;
    }
    flush();return out;
  }
  // Old hosts support physical keys. Keep one burst below their input limit;
  // unsupported text is refused in full, never partially typed or discarded.
  if (text.length > 32) return null;
  const plain = "`1234567890-=[]\\;',./", shifted = '~!@#$%^&*()_+{}|:"<>?';
  const codes = ["Backquote",...Array.from({length:9},(_,i)=>`Digit${i+1}`),"Digit0","Minus","Equal","BracketLeft","BracketRight","Backslash","Semicolon","Quote","Comma","Period","Slash"];
  const out: RemoteInput[]=[];
  for(const c of text.replace(/\r\n?/g,"\n")) {
    let code="", shift=false;
    if(/^[a-zA-Z]$/.test(c)) {code=`Key${c.toUpperCase()}`;shift=c===c.toUpperCase();}
    else if(c===" ") code="Space";
    else if(c==="\n")code="Enter";else if(c==="\t")code="Tab";
    else {let i=plain.indexOf(c); if(i<0){i=shifted.indexOf(c);shift=true;} if(i<0)return null;code=codes[i];}
    const addShift=shift&&!held.includes("ShiftLeft");
    if(addShift)out.push({kind:"key",code:"ShiftLeft",down:true});
    out.push({kind:"key",code,down:true},{kind:"key",code,down:false});
    if(addShift)out.push({kind:"key",code:"ShiftLeft",down:false});
  }
  return out;
}

/** Relative touchpad: one finger moves, a tap clicks, two fingers scroll.
 * Dragging is explicit so panning a small phone never moves a remote window. */
export class Touchpad {
  point={x:.5,y:.5};
  dragging=false;
  private touches=new Map<number,{x:number;y:number;startX:number;startY:number;at:number;tap:boolean}>();
  private lastTap={at:0,x:0,y:0,count:0};
  constructor(private send:(e:RemoteInput)=>unknown,private moved:(p:{x:number;y:number})=>void=()=>{}) {}
  down(id:number,x:number,y:number,at=Date.now()) {
    this.touches.set(id,{x,y,startX:x,startY:y,at,tap:this.touches.size===0});
    if(this.touches.size>1)for(const t of this.touches.values())t.tap=false;
  }
  move(id:number,x:number,y:number,width:number,height:number) {
    const t=this.touches.get(id);if(!t)return;
    const dx=x-t.x,dy=y-t.y;t.x=x;t.y=y;
    if(Math.hypot(x-t.startX,y-t.startY)>7)t.tap=false;
    if(this.touches.size===2){this.send({kind:"scroll",delta:Math.max(-1200,Math.min(1200,-dy))});return;}
    if(this.touches.size!==1||width<=0||height<=0)return;
    this.point={x:Math.max(0,Math.min(1,this.point.x+dx/width)),y:Math.max(0,Math.min(1,this.point.y+dy/height))};
    this.send({kind:"move",...this.point});this.moved(this.point);
  }
  up(id:number,at=Date.now()) {
    const t=this.touches.get(id);this.touches.delete(id);if(!t||!t.tap||at-t.at>500||this.dragging)return;
    const same=at-this.lastTap.at<450&&Math.hypot(this.point.x-this.lastTap.x,this.point.y-this.lastTap.y)<.025;
    const clicks=same&&this.lastTap.count<3?this.lastTap.count+1:1;
    this.lastTap={...this.point,at,count:clicks};this.click(0,clicks);
  }
  click(button=0,clicks=1) {this.send({kind:"button",...this.point,button,clicks,down:true});this.send({kind:"button",...this.point,button,clicks,down:false});}
  drag(on:boolean) {this.dragging=on;this.send({kind:"button",...this.point,button:0,clicks:1,down:on});}
  reset(){this.touches.clear();this.dragging=false;this.send({kind:"release"});}
}
