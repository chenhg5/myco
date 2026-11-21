// Capture is a source adapter; the editor only receives ordinary local Files.
// No video bytes or local paths are written into workspace records.
import { useEffect, useRef, useState } from "react";
import { Circle, Square, X, Loader2 } from "lucide-react";
import { myco, save, type OSJob } from "../_lib/myco";
import { t } from "../_lib/i18n";
import "./recorder.css";

type Recording = {path:string; bytes:number};
export function Recorder({onRecorded, request = 0}:{request?:number; onRecorded:(file:File)=>Promise<boolean>}) {
 const [open,setOpen]=useState(false), [state,setState]=useState<"idle"|"selecting"|"recording"|"importing">("idle");
 const [platform,setPlatform]=useState(""), [native,setNative]=useState(false), [mic,setMic]=useState(false), [interactive,setInteractive]=useState(true);
 const [error,setError]=useState(""), [path,setPath]=useState(""), [elapsed,setElapsed]=useState(0);
 const [retained,setRetained]=useState<File|null>(null);
 const job=useRef<OSJob<Recording>|null>(null), stream=useRef<MediaStream|null>(null), recorder=useRef<MediaRecorder|null>(null);
 const starting=useRef(false), alive=useRef(true);
 const active=state!=="idle";
 useEffect(()=>{
  if(!open)return;
  const previous=document.activeElement as HTMLElement|null;
  const el=document.querySelector<HTMLElement>(".rec-dialog");
  el?.querySelector<HTMLElement>("button")?.focus();
  const key=(e:KeyboardEvent)=>{
   if(e.key==="Escape"){e.preventDefault();setOpen(false);}
   if(e.key!=="Tab"||!el)return;
   const items=Array.from(el.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),[tabindex="0"]'));
   const index=items.indexOf(document.activeElement as HTMLElement);
   if(e.shiftKey&&index<=0){e.preventDefault();items.at(-1)?.focus();}
   else if(!e.shiftKey&&index===items.length-1){e.preventDefault();items[0]?.focus();}
  };
  window.addEventListener("keydown",key);return()=>{window.removeEventListener("keydown",key);previous?.focus();};
 },[open]);
 useEffect(()=>{if(request)setOpen(true);},[request]);
 useEffect(()=>{alive.current=true;void myco?.os?.list().then(v=>{setPlatform(v.os);setNative(v.caps.some(c=>c.name==="screen.record"&&c.available));}).catch(()=>{});const stop=()=>{void job.current?.cancel();stream.current?.getTracks().forEach(t=>t.stop());};window.addEventListener("pagehide",stop);return()=>{alive.current=false;stop();window.removeEventListener("pagehide",stop);};},[]);
 useEffect(()=>{if(state!=="recording")return;const at=Date.now();const id=setInterval(()=>setElapsed(Math.floor((Date.now()-at)/1000)),500);return()=>clearInterval(id);},[state]);
 useEffect(()=>{if(!active)return;const f=(e:BeforeUnloadEvent)=>{e.preventDefault();e.returnValue="";};window.addEventListener("beforeunload",f);return()=>window.removeEventListener("beforeunload",f);},[active]);
 async function importRecording(file:File) {
  if(!alive.current)return;
  setState("importing");setRetained(file);
  if(await onRecorded(file)){setRetained(null);setOpen(false);}else setError(t("导入没完成，原片已保留，可以重试。"));
 }
 async function start() {
  if(starting.current)return;starting.current=true;setError("");setPath("");setElapsed(0);setState("selecting");
  try {
   if(native && myco?.os) {
    const j=myco.os.screen.record({interactive:platform==="darwin"&&interactive,microphone:platform==="darwin"&&mic,clicks:true,max_seconds:600});job.current=j;
    j.on("started",(data:{path:string})=>{if(alive.current){setPath(data.path);setState("recording");}});
    const result=await j.done;job.current=null;
    if(!alive.current)return;setPath(result.path);setState("importing");
    if(result.bytes>512*1024*1024)throw new Error(t("原片超过 512 MB，已保留在本机；请导入较短的视频。"));
    const response=await fetch(myco.os.mediaURL(result.path));if(!response.ok)throw new Error(t("原片已保存在本机，但还没导入。"));
    const blob=await response.blob();await importRecording(new File([blob],`Recording-${new Date().toISOString().replace(/:/g,"-")}.mov`,{type:"video/quicktime"}));
   } else {
    if(!navigator.mediaDevices?.getDisplayMedia || !window.MediaRecorder)throw new Error(t("当前窗口不支持录屏。请在新版 Mac App 中使用，或用 Chrome / Edge 打开后录制；也可以导入已有视频。"));
    const s=await navigator.mediaDevices.getDisplayMedia({video:{frameRate:30},audio:true});
    if(!alive.current){s.getTracks().forEach(t=>t.stop());return;}stream.current=s;
    const type=["video/webm;codecs=vp9,opus","video/webm;codecs=vp8,opus","video/mp4"].find(x=>MediaRecorder.isTypeSupported(x));
    const m=new MediaRecorder(s,type?{mimeType:type}:undefined);recorder.current=m;
    const chunks:Blob[]=[];let bytes=0;let failed=false;
    m.ondataavailable=e=>{if(e.data.size){chunks.push(e.data);bytes+=e.data.size;if(bytes>512*1024*1024&&m.state!=="inactive")m.stop();}};
    m.onerror=()=>{failed=true;setError(t("录制中断，已保留收到的片段。"));};
    m.onstop=async()=>{
     clearTimeout(limit);s.getTracks().forEach(t=>t.stop());stream.current=null;recorder.current=null;
     try{if(!chunks.length)throw new Error(t("没有录到画面，请重新选择屏幕。"));const mime=m.mimeType||chunks[0].type;await importRecording(new File(chunks,`Recording-${Date.now()}.${mime.includes("mp4")?"mp4":"webm"}`,{type:mime}));if(failed)setOpen(true);}catch(e){if(alive.current)setError((e as Error).message);}finally{starting.current=false;if(alive.current)setState("idle");}
    };
    s.getVideoTracks()[0].onended=()=>{if(m.state!=="inactive")m.stop();};
    m.start(1000);setState("recording");
    const limit=setTimeout(()=>{if(m.state!=="inactive")m.stop();},600000);
    return;
   }
  }catch(e){if(alive.current){setOpen(true);setError((e as Error).name==="NotAllowedError"?t("录屏未获允许或已取消，请重新选择。"):(e as Error).message);}}
  finally{if(!recorder.current){starting.current=false;if(alive.current)setState("idle");}}
 }
 function stop(){if(job.current)void job.current.write("stop").catch(e=>setError(String(e.message)));const m=recorder.current;if(m&&m.state!=="inactive")m.stop();}
 return <>
 <button className="btn" onClick={()=>setOpen(true)}><Circle className="ico"/>{active?`${t("录制中")} ${Math.floor(elapsed/60)}:${String(elapsed%60).padStart(2,"0")}`:t("开始录屏")}</button>
 {state==="recording"&&<button className="btn btn-primary" onClick={stop}><Square className="ico"/>{t("停止并剪辑")}</button>}
 {open&&<div className="rec-backdrop"><section className="rec-dialog" role="dialog" aria-modal="true" aria-labelledby="rec-title">
 <header><div><span className="rec-kicker">{t("录制 → 剪辑 → 导出")}</span><h2 id="rec-title">{t("录下你的下一次演示")}</h2></div><button className="btn" aria-label={t("关闭")} onClick={()=>setOpen(false)}><X className="ico"/></button></header>
 <p>{t("视频留在这台电脑。停止录制后会自动放进时间线，可以裁剪、加文字和重点放大。")}</p>
 {state==="idle"&&<>
 {native&&platform==="darwin"?<><label className="rec-option"><input type="checkbox" checked={interactive} onChange={e=>setInteractive(e.target.checked)}/><span>{t("由系统选择录制区域")}<small>{t("取消选择则录制主屏幕；系统工具栏也可以停止录制。")}</small></span></label><label className="rec-option"><input type="checkbox" checked={mic} onChange={e=>setMic(e.target.checked)}/><span>{t("同时录制麦克风")}<small>{t("默认不录声音；此版本不采集电脑内部声音。")}</small></span></label></>:native?<p className="rec-info">{t("此版本录制 Windows 整个桌面，不采集声音。")}</p>:<p className="rec-info">{t("由浏览器选择屏幕或窗口；声音是否可用取决于系统和你选择的共享选项。")}</p>}
 <p className="rec-info">{t("单次最多 10 分钟，浏览器录制和自动导入最多 512 MB。请避免展示密码和私人信息。")}</p></>}
 {error&&<div className="rec-error" role="alert">{error}</div>}
 {path&&<button className="btn" onClick={()=>void myco?.os?.fs.reveal({path})}>{t("查看本机原片")}</button>}
 {retained&&state==="idle"&&<button className="btn" onClick={()=>void save(retained.name,retained,retained.type).then(()=>{setRetained(null);setError("");}).catch(e=>setError(String(e.message)))}>{t("保存原片")}</button>}
 {retained&&state==="idle"&&<button className="btn" onClick={()=>void importRecording(retained).finally(()=>setState("idle"))}>{t("重新导入原片")}</button>}
 <footer>{state==="idle"?<button className="btn btn-primary" disabled={!!retained} onClick={()=>void start()}><Circle className="ico"/>{t("选择画面并开始")}</button>:state==="importing"?<span><Loader2 className="ico spin"/>{t("正在导入原片…")}</span>:<><span role="status">{state==="selecting"?t("请在系统窗口中选择画面"):t("正在录制，可以切到其他窗口")}</span><button className="btn btn-primary" onClick={stop}><Square className="ico"/>{t("停止并剪辑")}</button></>}</footer>
 </section></div>}
 </>;
}
