import { useEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { myco } from "../_lib/myco";
import { boundedView, zoomView, pinchView, followPointer, type View } from "../_lib/remote-viewport";
import { Touchpad, textInput } from "../_lib/remote-input";
import {
  Monitor,
  ArrowUpRight,
  ArrowDownToLine,
  ShieldCheck,
  MousePointer2,
  Eye,
  RefreshCw,
  Search,
  Settings2,
  Unplug,
  Maximize2,
  ChevronLeft,
  Copy,
  Check,
  LoaderCircle,
  X,
  Keyboard,
  LockKeyhole,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { Kit, mount } from "../_lib/appkit";
import {
  DesktopSession,
  pointInVideo,
  remoteAvailable,
  remoteCall,
  type RemoteDevice,
  type RemoteStatus,
  type RemoteStats,
} from "../_lib/remote";
import "./remote.css";
const kit = new Kit([]);
const errorMessage = (e: unknown) => {
  const code = e instanceof Error ? e.message : String(e);
  const messages: Record<string, string> = {
    wrong_pairing_code: uiText(
      "配对码不对，请查看远端电脑上当前显示的配对码。",
    ),
    pairing_paused: uiText("配对尝试过多，请等一分钟再试。"),
    control_not_allowed: uiText(
      "远端尚未允许键鼠控制，或未授予辅助功能权限。可以改用仅查看。",
    ),
    computer_busy: uiText("这台电脑正在被连接，请稍后再试。"),
    video_unsupported: uiText(
      "当前 App 不支持 H.264 实时画面，请更新桌面 App。",
    ),
    remote_unavailable: uiText("请在支持远程桌面的新版 myco 桌面 App 中打开。"),
    connection_timeout: uiText(
      "连接超时。请确认两台电脑在同一局域网，或已通过 Tailscale 连通。",
    ),
    screen_capture_failed: uiText(
      "没有收到画面。请在远端授予屏幕录制权限，并确认桌面已解锁。",
    ),
    component_not_installed: uiText("先在这台电脑上安装可选远控组件。"),
    connection_closed: uiText("远端已结束共享。"),
    connection_lost: uiText("连接已断开，请确认远端仍在共享后重新连接。"),
  };
  if (/component download unavailable/.test(code))
    return uiText(
      "远控组件暂时无法下载，请稍后重试或使用已发布组件的新版 App。",
    );
  if (
    /checksum|unsafe component|unexpected component|archive.*incomplete/.test(
      code,
    )
  )
    return uiText("组件校验失败，安装已取消。请重试，不要手动运行下载文件。");
  if (/remote desktop is offline|session expired/.test(code))
    return uiText("这台电脑已停止共享，请刷新列表后重试。");
  if (/already has a remote session/.test(code)) return messages.computer_busy;
  return (
    messages[code] ||
    (/404|not_found|Not Found/.test(code)
      ? uiText("此工作区还不支持远程桌面，请更新工作区服务。")
      : /permission_denied|not allow|没有允许/.test(code)
        ? uiText("操作未获允许。需要时可以重新发起。")
        : /screen_capture/.test(code)
          ? messages.screen_capture_failed
          : code)
  );
};
function App() {
  const controllerOnly = !!myco?.features?.includes("remote-controller");
  const [devices, setDevices] = useState<RemoteDevice[]>([]),
    [status, setStatus] = useState<RemoteStatus | null>(null),
    [error, setError] = useState(""),
    [query, setQuery] = useState(""),
    [busy, setBusy] = useState(""),
    [loading, setLoading] = useState(true),
    [tab, setTab] = useState<"connect" | "share">("connect"),
    [settings, setSettings] = useState(false),
    [control, setControl] = useState(false),
    [selected, setSelected] = useState<RemoteDevice | null>(null),
    [code, setCode] = useState(""),
    [connecting, setConnecting] = useState(false),
    [active, setActive] = useState<RemoteDevice | null>(null),
    [state, setState] = useState(""),
    [stats, setStats] = useState<RemoteStats | null>(null),
    [copied, setCopied] = useState(false);
  const [view,setView]=useState<View>({scale:1,x:0,y:0});
  const viewRef=useRef(view); viewRef.current=view;
  const [expanded,setExpanded]=useState(false);
  const [toolsVisible,setToolsVisible]=useState(false);
  const touches=useRef(new Map<number,{x:number;y:number}>());
  const gesture=useRef<{distance:number;center:{x:number;y:number};lastY:number;view:View;at:number;mode:"pending"|"pinch"|"scroll"}|null>(null);
  const multiTouch=useRef(false);
  const updateView=(next:View)=>{viewRef.current=next;setView(next);};
  const zoomBy=(delta:number)=>{const box=stage.current?.getBoundingClientRect();if(box)updateView(zoomView(viewRef.current,viewRef.current.scale+delta,{x:box.width/2,y:box.height/2},box.width,box.height));};
  const [keyboardOpen,setKeyboardOpen]=useState(false),[draft,setDraft]=useState(""),[dragging,setDragging]=useState(false),[modifiers,setModifiers]=useState<string[]>([]),[,setCursor]=useState({x:.5,y:.5}),[showCursor,setShowCursor]=useState(false),[,setLayout]=useState(0);
  const [liveInput,setLiveInput]=useState(false),[scrollMode,setScrollMode]=useState(false),[inputError,setInputError]=useState("");
  const keyboard=useRef<HTMLTextAreaElement>(null),composing=useRef(false);
  const touchpad=useRef<Touchpad>();
  if(!touchpad.current)touchpad.current=new Touchpad(e=>session.current?.input(e),p=>setCursor({...p}));
  useEffect(()=>{const resize=()=>{document.documentElement.style.setProperty("--rd-viewport-height",`${window.visualViewport?.height??innerHeight}px`);document.documentElement.style.setProperty("--rd-viewport-top",`${window.visualViewport?.offsetTop??0}px`);setLayout(n=>n+1);};resize();window.addEventListener("resize",resize);window.visualViewport?.addEventListener("resize",resize);window.visualViewport?.addEventListener("scroll",resize);return()=>{window.removeEventListener("resize",resize);window.visualViewport?.removeEventListener("resize",resize);window.visualViewport?.removeEventListener("scroll",resize);};},[]);
  const motion = useRef<{ x: number; y: number } | null>(null),
    motionFrame = useRef(0);
  const clicks = useRef({ at: 0, x: 0, y: 0, button: 0, count: 0 });
  useEffect(() => {
    const el = stage.current;
    if (!active || !el) return;
    const wheel = (e: WheelEvent) => {
      if (!control) return;
      e.preventDefault();
      const delta =
        e.deltaY *
        (e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? el.clientHeight : 1);
      session.current?.input({
        kind: "scroll",
        delta: Math.max(-1200, Math.min(1200, delta)),
      });
    };
    el.addEventListener("wheel", wheel, { passive: false });
    return () => el.removeEventListener("wheel", wheel);
  }, [active, control]);
  const session = useRef<DesktopSession | null>(null),
    video = useRef<HTMLVideoElement>(null),
    stage = useRef<HTMLDivElement>(null),
    pendingStream = useRef<MediaStream | null>(null),
    mounted = useRef(true),
    attempt = useRef(0);
  const refresh = async () => {
    if (!remoteAvailable()) {
      setError(errorMessage(new Error("remote_unavailable")));
      setLoading(false);
      return;
    }
    try {
      const [s, list] = await Promise.all([
        remoteCall<RemoteStatus>("status"),
        remoteCall<RemoteDevice[]>("devices"),
      ]);
      if (mounted.current) {
        setStatus(s);
        setDevices(list);
        setSelected(current=>current?(list.find(d=>d.machine===current.machine)??current):null);
      }
    } catch (e) {
      if (mounted.current) setError(errorMessage(e));
    } finally {
      if (mounted.current) setLoading(false);
    }
  };
  useEffect(() => {
    mounted.current = true;
    void refresh();
    const t = setInterval(() => void refresh(), 5000);
    return () => {
      mounted.current = false;
      attempt.current++;
      clearInterval(t);
      cancelAnimationFrame(motionFrame.current);
      void session.current?.close();
    };
  }, []);
  useEffect(() => {
    if (active && video.current && pendingStream.current) {
      video.current.srcObject = pendingStream.current;
      void video.current.play().catch(() => {});
    }
  }, [active]);
  useEffect(() => {
    if (!selected && !settings) return;
    const before = document.activeElement as HTMLElement | null;
    const timer = setTimeout(
      () =>
        document
          .querySelector<HTMLElement>(".rd-dialog input,.rd-dialog button")
          ?.focus(),
      0,
    );
    const key = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") {
        void disconnect();
        setSelected(null);
        setSettings(false);
        e.preventDefault();
      }
      if (e.key === "Tab") {
        const items = [
          ...document.querySelectorAll<HTMLElement>(
            ".rd-dialog button:not(:disabled),.rd-dialog input:not(:disabled)",
          ),
        ];
        const first = items[0],
          last = items[items.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          last?.focus();
          e.preventDefault();
        } else if (!e.shiftKey && document.activeElement === last) {
          first?.focus();
          e.preventDefault();
        }
      }
    };
    document.addEventListener("keydown", key);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("keydown", key);
      before?.focus();
    };
  }, [!!selected, settings]);
  const run = async (action: string, args: unknown = {}) => {
    setBusy(action);
    setError("");
    try {
      await remoteCall(action, args);
      await refresh();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy("");
    }
  };
  const disconnect = async () => {
    attempt.current++;
    touchpad.current?.reset();if(touchpad.current)touchpad.current.point={x:.5,y:.5};setCursor({x:.5,y:.5});updateView({scale:1,x:0,y:0});setExpanded(false);setToolsVisible(false);touches.current.clear();gesture.current=null;multiTouch.current=false;setDragging(false);setModifiers([]);setKeyboardOpen(false);setDraft("");setInputError("");setLiveInput(false);setScrollMode(false);setShowCursor(false);
    const previous=session.current;
    session.current=null;pendingStream.current=null;
    setConnecting(false);setActive(null);setStats(null);setState("");
    await previous?.close();
  };

  const connect = async () => {
    if (!selected || connecting || !code.trim()) return;
    const id = ++attempt.current,
      device = selected;
    setConnecting(true);
    setError("");
    setState("connecting");
    const s = new DesktopSession();
    session.current = s;
    try {
      await s.connect(
        device,
        code,
        control && device.control,
        (stream) => {
          if (id !== attempt.current) return;
          pendingStream.current = stream;
          setActive(device);
          if(controllerOnly){setExpanded(true);setToolsVisible(false);}
          setSelected(null);
          setCode("");
        },
        (st) => {
          if (id !== attempt.current) return;
          setState(st);
          if (!["new", "connecting", "connected"].includes(st)) {
            setError(
              errorMessage(
                new Error(
                  st === "failed"
                    ? "screen_capture_failed"
                    : st === "closed" || st === "disconnected"
                      ? "connection_lost"
                      : st,
                ),
              ),
            );
            attempt.current++;setConnecting(false);setSelected(null);
            pendingStream.current=null;setStats(null);setActive(null);
          }
        },
        setStats,
      );
    } catch (e) {
      if (id === attempt.current) {
        setError(errorMessage(e));
        await s.close();
      }
    } finally {
      if (id === attempt.current) setConnecting(false);
    }
  };
  const input = (ev: Record<string, unknown>) => session.current?.input(ev);
  const releaseModifiers=()=>{for(const code of modifiers)input({kind:"key",code,down:false});setModifiers([]);};
  const specialKey=(code:string)=>{input({kind:"key",code,down:true});input({kind:"key",code,down:false});releaseModifiers();};
  const typed=(value:string)=>{
    if(composing.current)return false;
    const events=textInput(value,!!session.current?.supportsText,modifiers);
    if(events===null){setInputError(uiText(modifiers.length?"先松开组合键，再输入这段文字。":([...value].length>256?"一次最多输入256个字符，文字已保留。":"远端组件需要更新才能输入这段文字，文字已保留。")));return false;}
    if(!events.length)return true;
    for(const e of events)if(!input(e)){setInputError(uiText("文字未能全部送出，请检查连接后再确认远端内容。"));return false;}
    releaseModifiers();setDraft("");setInputError("");if(keyboard.current)keyboard.current.value="";return true;
  };
  useEffect(()=>{
    const el=keyboard.current;if(!keyboardOpen||!el||!liveInput)return;
    const before=(e:InputEvent)=>{
      if(e.isComposing||composing.current)return;
      if(e.inputType==="deleteContentBackward"&&!el.value){e.preventDefault();specialKey("Backspace");}
      if(e.inputType==="insertLineBreak"||e.inputType==="insertParagraph"){e.preventDefault();if(el.value&&!typed(el.value))return;specialKey("Enter");}
    };
    el.addEventListener("beforeinput",before);return()=>el.removeEventListener("beforeinput",before);
  },[keyboardOpen,liveInput,modifiers]);
  const touch=(e:React.PointerEvent,phase:"down"|"move"|"up")=>{
    if(e.pointerType!=="touch")return false;
    e.preventDefault();
    const pad=touchpad.current!,box=e.currentTarget.getBoundingClientRect(),points=touches.current;
    const point={x:e.clientX-box.left,y:e.clientY-box.top};
    if(phase==="down"){
      e.currentTarget.setPointerCapture(e.pointerId);points.set(e.pointerId,point);
      if(points.size===1){multiTouch.current=false;if(control)pad.down(e.pointerId,e.clientX,e.clientY);}
      else if(points.size===2){
        multiTouch.current=true;pad.reset();setDragging(false);setModifiers([]);
        // Do not infer scroll from the first moving finger: a slow pinch often
        // starts with parallel movement. Remote scrolling is an explicit mode.
        const [a,b]=[...points.values()];gesture.current={distance:Math.max(1,Math.hypot(a.x-b.x,a.y-b.y)),center:{x:(a.x+b.x)/2,y:(a.y+b.y)/2},lastY:(a.y+b.y)/2,view:{...viewRef.current},at:performance.now(),mode:control&&scrollMode?"scroll":"pending"};
      }
    }else if(phase==="move"){
      const previous=points.get(e.pointerId);if(!previous)return true;points.set(e.pointerId,point);
      if(points.size===2&&gesture.current){
        const g=gesture.current,[a,b]=[...points.values()],distance=Math.hypot(a.x-b.x,a.y-b.y),center={x:(a.x+b.x)/2,y:(a.y+b.y)/2};
        if(g.mode==="pending"){
          // Like Windows App: hold two still fingers before dragging to scroll.
          // Any early movement reserves this gesture for local zoom/pan, so a
          // slightly asymmetric pinch can never get locked into remote scroll.
          if(control&&performance.now()-g.at>=350)g.mode="scroll";
          else if(Math.abs(distance-g.distance)>4||Math.hypot(center.x-g.center.x,center.y-g.center.y)>4)g.mode="pinch";
        }
        if(g.mode==="pinch")updateView(pinchView(g.view,distance/g.distance,g.center,center,box.width,box.height));
        if(g.mode==="scroll")input({kind:"scroll",delta:Math.max(-1200,Math.min(1200,-(center.y-g.lastY)*2))});
        g.lastY=center.y;
      }else if(points.size===1&&!multiTouch.current){
        if(control){setShowCursor(true);pad.move(e.pointerId,e.clientX,e.clientY,box.width*viewRef.current.scale,box.height*viewRef.current.scale);
          const p=cursorPosition();if(typeof p.left==="number"&&typeof p.top==="number")updateView(followPointer(viewRef.current,{x:p.left,y:p.top},box.width,box.height));
        }else updateView(boundedView({...viewRef.current,x:viewRef.current.x+point.x-previous.x,y:viewRef.current.y+point.y-previous.y},box.width,box.height));
      }
    }else{
      points.delete(e.pointerId);if(control&&!multiTouch.current)pad.up(e.pointerId);
      if(points.size<2)gesture.current=null;
      if(!points.size)multiTouch.current=false;
    }
    return true;
  };
  const cursorPosition=()=>{
    const v=video.current;if(!v||!v.videoWidth)return {left:"50%",top:"50%"};
    const scale=Math.min(v.clientWidth/v.videoWidth,v.clientHeight/v.videoHeight),w=v.videoWidth*scale,h=v.videoHeight*scale;
    return {left:(v.clientWidth-w)/2+touchpad.current!.point.x*w,top:(v.clientHeight-h)/2+touchpad.current!.point.y*h};
  };
  const pointer = (e: React.PointerEvent) => {
    const v = video.current;
    if (!v) return null;
    return pointInVideo(
      e.clientX,
      e.clientY,
      v.getBoundingClientRect(),
      v.videoWidth,
      v.videoHeight,
    );
  };
  const filtered = devices.filter((d) =>
    (d.name + " " + d.machine).toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div className={`rd-app${controllerOnly?" is-controller":""}`}>
      <header className="rd-header">
        <div className="rd-brand">
          <span className="rd-mark">
            <Monitor size={22} />
          </span>
          <div>
            <h1>{uiText("远程桌面")}</h1>
            <p>{uiText("连接你的电脑，接着手头的工作。")}</p>
          </div>
        </div>
        {!controllerOnly && <button
          className="rd-icon"
          aria-label={uiText("组件设置")}
          onClick={() => setSettings(true)}
        >
          <Settings2 size={18} />
        </button>}
      </header>
      <main>
        {error && (
          <div role="alert" className="rd-error">
            <span>{error}</span>
            <button
              className="rd-icon"
              aria-label={uiText("关闭提示")}
              onClick={() => setError("")}
            >
              <X size={16} />
            </button>
          </div>
        )}
        {active ? (
          <section className={`rd-session${expanded?" is-expanded":""}${controllerOnly?" is-immersive":""}${toolsVisible?" show-tools":""}${keyboardOpen?" keyboard-open":""}`}>
            {controllerOnly&&<div className="rd-floating-tools">
              <button className="rd-icon" aria-label={uiText("工具")} aria-expanded={toolsVisible} onClick={()=>{setToolsVisible(!toolsVisible);if(toolsVisible){setKeyboardOpen(false);keyboard.current?.blur();releaseModifiers();}}}><Settings2 size={19}/></button>
              {control&&scrollMode&&<button className="rd-scroll-mode" aria-label={uiText("切回双指缩放")} onClick={()=>setScrollMode(false)}>{uiText("双指滚动 · 点此缩放")}</button>}
              {control&&<button className="rd-icon" aria-label={uiText("键盘")} onClick={()=>{setToolsVisible(true);flushSync(()=>setKeyboardOpen(!keyboardOpen));if(!keyboardOpen)keyboard.current?.focus();}}><Keyboard size={19}/></button>}
            </div>}
            <div className="rd-session-bar">
              <button
                className="rd-btn rd-quiet"
                onClick={() => void disconnect()}
              >
                <ChevronLeft size={16} />
                {uiText("返回")}
              </button>
              <strong>{active.name}</strong>
              <span className="rd-session-state">
                {state === "connected" ? uiText("已连接") : uiText("正在连接")}
              </span>
              <div className="rd-session-actions">
                <span>
                  {control?<Keyboard size={14}/>:<Eye size={14}/>}
                  {control ? uiText("可以操作") : uiText("仅查看")}
                </span>
                <button className="rd-icon" aria-label={uiText(expanded?"收起画面":"展开画面")} aria-pressed={expanded} onClick={()=>{setExpanded(!expanded);updateView({scale:1,x:0,y:0});}}>
                  {expanded?<X size={17}/>:<Maximize2 size={17}/>}
                </button>
                <button className="rd-btn" onClick={() => void disconnect()}>
                  <Unplug size={15} />
                  {uiText("断开")}
                </button>
              </div>
            </div>
            <div
              className="rd-screen"
              ref={stage}
              tabIndex={0}
              aria-label={uiText(control?"远程画面，点击后使用键盘和鼠标":"仅查看画面，可捏合缩放")}
              onContextMenu={(e) => e.preventDefault()}
              onPointerMove={(e) => {
                if(touch(e,"move"))return;
                if (!control) return;
                const p = pointer(e);
                if (p) {
                  motion.current = p;
                  if (!motionFrame.current)
                    motionFrame.current = requestAnimationFrame(() => {
                      motionFrame.current = 0;
                      if (motion.current)
                        input({ kind: "move", ...motion.current });
                    });
                }
              }}
              onPointerDown={(e) => {
                if(touch(e,"down"))return;
                if (!control) return;
                const p = pointer(e);
                if (p) {
                  e.currentTarget.focus();
                  e.currentTarget.setPointerCapture(e.pointerId);
                  const last = clicks.current,
                    now = Date.now();
                  const same =
                    now - last.at < 450 &&
                    Math.abs(e.clientX - last.x) < 5 &&
                    Math.abs(e.clientY - last.y) < 5 &&
                    e.button === last.button;
                  clicks.current = {
                    at: now,
                    x: e.clientX,
                    y: e.clientY,
                    button: e.button,
                    count: same && last.count < 3 ? last.count + 1 : 1,
                  };
                  input({
                    kind: "button",
                    ...p,
                    button: e.button,
                    down: true,
                    clicks: clicks.current.count,
                  });
                  e.preventDefault();
                }
              }}
              onPointerUp={(e) => {
                if(touch(e,"up"))return;
                if (!control) return;
                const p = pointer(e);
                if (p)
                  input({
                    kind: "button",
                    ...p,
                    button: e.button,
                    clicks: clicks.current.count,
                    down: false,
                  });
                else input({ kind: "release" });
              }}
              onPointerCancel={() => {touches.current.clear();gesture.current=null;multiTouch.current=false;touchpad.current?.reset();setDragging(false);setModifiers([]);}}
              onKeyDown={(e) => {
                if (control && video.current?.videoWidth) {
                  input({ kind: "key", code: e.code, down: true });
                  if (e.code !== "Escape") e.preventDefault();
                }
              }}
              onKeyUp={(e) => {
                if (control && video.current?.videoWidth) {
                  input({ kind: "key", code: e.code, down: false });
                  if (e.code !== "Escape") e.preventDefault();
                }
              }}
              onBlur={() => {touchpad.current?.reset();setDragging(false);setModifiers([]);}}
            >
              <video
                ref={video}
                style={{transform:`translate(${view.x}px, ${view.y}px) scale(${view.scale})`,transformOrigin:"0 0"}}
                autoPlay
                muted
                playsInline
                onLoadedMetadata={()=>setLayout(n=>n+1)}
                aria-label={uiText("远程桌面画面")}
              />
              {showCursor&&control&&<span className="rd-touch-cursor" aria-hidden="true" style={(()=>{const p=cursorPosition();return {left:typeof p.left==="number"?p.left*view.scale+view.x:p.left,top:typeof p.top==="number"?p.top*view.scale+view.y:p.top};})()}/>}
            </div>
            <div className={`rd-touch-controls${keyboardOpen?" has-keyboard":""}`}>
              <p>{uiText(control?(scrollMode?"双指滚动远端；要缩放请关闭滚动模式。":"双指捏合缩放；双指按住片刻再拖动，滚动远端。"):"仅查看：双指捏合缩放，放大后单指移动画面。")}</p>
              <div className="rd-touch-row">
                <button className="rd-btn" aria-label={uiText("缩小画面")} disabled={view.scale<=1} onClick={()=>zoomBy(-.5)}>−</button>
                <span className="rd-zoom" aria-label={uiText("画面缩放")}>{Math.round(view.scale*100)}%</span>
                <button className="rd-btn" aria-label={uiText("放大画面")} disabled={view.scale>=5} onClick={()=>zoomBy(.5)}>+</button>
                <button className="rd-btn" onClick={()=>updateView({scale:1,x:0,y:0})}>{uiText("适应画面")}</button>
              </div>
              {control&&<div className="rd-touch-row rd-remote-keys">
                <button className="rd-btn" aria-pressed={keyboardOpen} onClick={()=>{flushSync(()=>setKeyboardOpen(!keyboardOpen));if(!keyboardOpen)keyboard.current?.focus();}}><Keyboard size={16}/>{uiText("键盘")}</button>
                <button className="rd-btn" aria-pressed={scrollMode} onClick={()=>setScrollMode(!scrollMode)}>{uiText("双指滚动")}</button>
                <button className="rd-btn" onClick={()=>touchpad.current?.click(2)}>{uiText("右键")}</button>
                <button className="rd-btn" aria-pressed={dragging} onClick={()=>{touchpad.current?.drag(!dragging);setDragging(!dragging);}}>{dragging?uiText("结束拖动"):uiText("拖动")}</button>
                {["Escape","Tab","Enter","Backspace"].map(code=><button key={code} className="rd-btn" aria-label={code} onClick={()=>specialKey(code)}>{({Escape:"Esc",Tab:"Tab",Enter:"↵",Backspace:"⌫"})[code as "Enter"]}</button>)}
              </div>}
              {control&&keyboardOpen&&<div className="rd-keyboard">
                <div className="rd-composer-row">
                  <textarea ref={keyboard} aria-label={uiText("输入文字到远端")} placeholder={uiText(liveInput?"即时输入到远端":"先写好，再发送；也可用键盘麦克风听写")} value={draft} rows={1} autoCapitalize="off" autoCorrect={liveInput?"off":"on"} spellCheck={!liveInput}
                    onChange={e=>{setDraft(e.target.value);setInputError("");if(liveInput&&!(e.nativeEvent as InputEvent).isComposing)typed(e.target.value);}}
                    onCompositionStart={()=>{composing.current=true;}} onCompositionEnd={e=>{composing.current=false;if(liveInput)typed(e.currentTarget.value);}}
                    onKeyDown={e=>{if(!liveInput)return;if(e.key==="Enter"&&!composing.current&&!(e.nativeEvent as KeyboardEvent).isComposing){e.preventDefault();if(draft&&!typed(draft))return;specialKey("Enter");}if(e.key==="Backspace"&&!draft&&!composing.current){e.preventDefault();specialKey("Backspace");}}}
                  />
                  {!liveInput&&<button className="rd-btn rd-primary" disabled={!draft} onClick={()=>{if(typed(draft)){setKeyboardOpen(false);setToolsVisible(false);keyboard.current?.blur();}}}>{uiText("发送文字")}</button>}
                  <button className="rd-icon" aria-label={uiText("收起键盘")} onClick={()=>{setKeyboardOpen(false);setToolsVisible(false);keyboard.current?.blur();releaseModifiers();}}><ChevronLeft size={18}/></button>
                </div>
                <div className="rd-touch-row rd-keyboard-options">
                  <label className="rd-live-input"><input type="checkbox" checked={liveInput} onChange={e=>{setLiveInput(e.target.checked);releaseModifiers();setInputError("");}}/>{uiText("即时输入")}</label>
                  {liveInput&&[["ControlLeft","Ctrl"],["AltLeft","Alt"],["MetaLeft",active.os==="darwin"?"⌘":"Win"],["ShiftLeft","Shift"]].map(([code,label])=><button key={code} className="rd-btn" aria-pressed={modifiers.includes(code)} onClick={()=>{const down=!modifiers.includes(code);input({kind:"key",code,down});setModifiers(down?[...modifiers,code]:modifiers.filter(x=>x!==code));keyboard.current?.focus();}}>{label}</button>)}
                  {liveInput&&["Escape","Tab","Enter","Backspace"].map(code=><button key={code} className="rd-btn" aria-label={code} onClick={()=>{specialKey(code);keyboard.current?.focus();}}>{({Escape:"Esc",Tab:"Tab",Enter:"↵",Backspace:"⌫"})[code as "Enter"]}</button>)}
                  {!liveInput&&<span className="rd-input-hint">{uiText(session.current?.supportsText?"先点远端输入位置；发送文字不会额外按回车。":"远端组件较旧，只支持短英文；更新后可发送中文。") }</span>}
                </div>
                {inputError&&<p className="rd-input-error" role="alert">{inputError}</p>}
              </div>}
            </div>
            <footer className="rd-session-footer">
              <span>
                <LockKeyhole size={13} />
                {uiText("加密连接")}
              </span>
              <span>
                {stats?.width
                  ? `${stats.width} × ${stats.height} · ${Math.round(stats.fps)} fps · ${stats.mbps.toFixed(1)} Mbps · ${stats.latency} ms`
                  : uiText("正在接收画面")}
              </span>
              <span>{uiText("当前版本不传输声音")}</span>
            </footer>
          </section>
        ) : (
          <>
            <nav className="rd-tabs" aria-label={uiText("远程桌面功能")}>
              <button
                className={tab === "connect" ? "is-active" : ""}
                onClick={() => setTab("connect")}
              >
                <ArrowUpRight size={16} />
                {uiText("连接电脑")}
              </button>
              {!controllerOnly && <button
                className={tab === "share" ? "is-active" : ""}
                onClick={() => setTab("share")}
              >
                <Monitor size={16} />
                {uiText("共享这台电脑")}
                {status?.pairing_code && <i />}
              </button>}
            </nav>
            {tab === "connect" ? (
              <section className="rd-connect">
                <div className="rd-section-heading">
                  <div>
                    <h2>{uiText("你的电脑")}</h2>
                    <p>
                      {uiText(
                        "在另一台电脑中打开此应用，启用共享后就会出现在这里。",
                      )}
                    </p>
                  </div>
                  <button
                    className="rd-icon"
                    aria-label={uiText("刷新电脑列表")}
                    onClick={() => {
                      setLoading(true);
                      void refresh();
                    }}
                  >
                    <RefreshCw size={17} className={loading ? "rd-spin" : ""} />
                  </button>
                </div>
                <div className="rd-search">
                  <Search size={16} />
                  <input
                    aria-label={uiText("搜索电脑")}
                    placeholder={uiText("搜索电脑")}
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                </div>
                {loading && !devices.length ? (
                  <div className="rd-device-grid" aria-busy="true">
                    {[1, 2, 3].map((n) => (
                      <div className="rd-skeleton" key={n} />
                    ))}
                  </div>
                ) : filtered.length ? (
                  <div className="rd-device-grid">
                    {filtered.map((d) => (
                      <button
                        className="rd-device"
                        key={d.machine}
                        onClick={() => {
                          setSelected(d);
                          setCode("");
                          setControl(false);
                          setError("");
                        }}
                      >
                        <div className="rd-device-top">
                          <span className="rd-device-icon">
                            <Monitor size={28} />
                          </span>
                          <span className="rd-tag">
                            <i />
                            {uiText("可连接")}
                          </span>
                        </div>
                        <h3>{d.name || d.machine}</h3>
                        <p>
                          {d.os === "darwin"
                            ? "macOS"
                            : d.os === "windows"
                              ? "Windows"
                              : d.os}
                        </p>
                        <div className="rd-device-bottom">
                          <span>
                            {d.control ? (
                              <>
                                <MousePointer2 size={13} />
                                {uiText("支持键鼠控制")}
                              </>
                            ) : (
                              <>
                                <Eye size={14} />
                                {uiText("仅查看")}
                              </>
                            )}
                          </span>
                          <ArrowUpRight size={18} />
                        </div>
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="rd-empty">
                    <Monitor size={38} strokeWidth={1.1} />
                    <h3>
                      {query
                        ? uiText("没有匹配的电脑")
                        : uiText("还没有电脑在共享")}
                    </h3>
                    <p>
                      {query
                        ? uiText("换个关键词试试。")
                        : uiText(
                            "在需要连接的 Mac 或 Windows 上打开远程桌面，选择「共享这台电脑」。",
                          )}
                    </p>
                    {!query && !controllerOnly && (
                      <button
                        className="rd-btn"
                        onClick={() => setTab("share")}
                      >
                        {uiText("了解怎么共享")}
                        <ArrowUpRight size={14} />
                      </button>
                    )}
                  </div>
                )}
                <div className="rd-bottom-note">
                  <ShieldCheck size={18} />
                  <div>
                    <strong>{uiText("连接只由你开启")}</strong>
                    <p>
                      {uiText(
                        "需要远端当前的配对码。画面和键鼠通过独立加密通道传输，不经过聊天消息。",
                      )}
                    </p>
                  </div>
                </div>
              </section>
            ) : (
              <section className="rd-share">
                <div className="rd-share-intro">
                  <div className="rd-hero-icon">
                    <Monitor size={44} strokeWidth={1} />
                  </div>
                  <h2>
                    {status?.pairing_code
                      ? uiText("这台电脑正在共享")
                      : uiText("让你的另一台电脑连进来")}
                  </h2>
                  <p>
                    {uiText(
                      "在同一工作区打开远程桌面，输入这里的配对码即可连接。",
                    )}
                  </p>
                </div>
                {!status?.installed ? (
                  <div className="rd-install-card">
                    <div>
                      <h3>{uiText("安装可选远控组件")}</h3>
                      <p>
                        {uiText(
                          "仅在使用远控时运行，可随时卸载。不会自动开放这台电脑。",
                        )}
                      </p>
                    </div>
                    <button
                      className="rd-btn rd-primary"
                      disabled={!!busy || !status?.supported}
                      onClick={() => void run("install")}
                    >
                      {busy === "install" ? (
                        <LoaderCircle className="rd-spin" size={16} />
                      ) : (
                        <ArrowDownToLine size={16} />
                      )}
                      {busy === "install"
                        ? uiText("正在下载安装…")
                        : uiText("安装组件")}
                    </button>
                    {status && !status.supported && (
                      <p className="rd-muted">
                        {uiText(
                          "组件支持 Apple 芯片或 Intel Mac，以及 x64 Windows。",
                        )}
                      </p>
                    )}
                  </div>
                ) : status.pairing_code ? (
                  <div className="rd-pairing">
                    <span>{uiText("本次共享的配对码")}</span>
                    <div className="rd-code">{status.pairing_code}</div>
                    <p>
                      {uiText("请只交给你信任的人。每次重新开启共享都会更换。")}
                    </p>
                    <div className="rd-row">
                      <button
                        className="rd-btn"
                        onClick={() => {
                          void navigator.clipboard
                            .writeText(status.pairing_code!)
                            .then(() => {
                              setCopied(true);
                              setTimeout(() => setCopied(false), 1800);
                            })
                            .catch(() =>
                              setError(uiText("无法复制，请手动输入配对码。")),
                            );
                        }}
                      >
                        {copied ? <Check size={15} /> : <Copy size={15} />}
                        {copied ? uiText("已复制") : uiText("复制配对码")}
                      </button>
                      <button
                        className="rd-btn rd-primary"
                        disabled={!!busy}
                        onClick={() => void run("disable")}
                      >
                        <Unplug size={15} />
                        {uiText("停止共享")}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="rd-enable-card">
                    <label className="rd-check">
                      <input
                        type="checkbox"
                        role="switch"
                        checked={control}
                        onChange={(e) => setControl(e.target.checked)}
                      />
                      <span>
                        <strong>{uiText("允许键盘和鼠标操作")}</strong>
                        <small>
                          {uiText(
                            "默认仅查看。Mac 需要给远控组件授予辅助功能权限。",
                          )}
                        </small>
                      </span>
                    </label>
                    <button
                      className="rd-btn rd-primary"
                      disabled={!!busy || status.running}
                      onClick={() => void run("enable", { control })}
                    >
                      {busy === "enable" ? (
                        <LoaderCircle size={16} className="rd-spin" />
                      ) : (
                        <Monitor size={16} />
                      )}
                      {uiText("开启共享")}
                    </button>
                    {status.running && (
                      <p>
                        {uiText(
                          "另一个远程桌面应用正在使用本机组件。请先在那里停止共享。",
                        )}
                      </p>
                    )}
                  </div>
                )}
                <div className="rd-share-notes">
                  <div>
                    <span>01</span>
                    <p>
                      {uiText(
                        "两边保持 myco 和此应用打开，关闭应用即停止共享。",
                      )}
                    </p>
                  </div>
                  <div>
                    <span>02</span>
                    <p>
                      {uiText(
                        "先使用同一局域网或 Tailscale。当前版本不自动配置公网中继。",
                      )}
                    </p>
                  </div>
                  <div>
                    <span>03</span>
                    <p>
                      {uiText(
                        "Mac 首次使用要允许屏幕录制；锁屏、登录和管理员确认窗口暂不支持。",
                      )}
                    </p>
                  </div>
                </div>
              </section>
            )}
          </>
        )}
      </main>
      <footer className="rd-footer">
        <span>myco / {uiText("远程桌面")}</span>
        <span>{uiText(controllerOnly?"手机仅作为控制端，关闭页面即断开。":"按需安装 · 默认关闭 · 随时断开")}</span>
      </footer>
      {selected && (
        <div
          className="rd-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget && !connecting) setSelected(null);
          }}
        >
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="rd-connect-title"
            className="rd-dialog"
          >
            <button
              className="rd-icon rd-dialog-close"
              aria-label={uiText("关闭")}
              onClick={() => {
                void disconnect();
                setSelected(null);
              }}
            >
              <X size={18} />
            </button>
            <div className="rd-dialog-symbol">
              <Monitor size={30} />
            </div>
            <h2 id="rd-connect-title">{uiText("连接到 {0}", selected.name)}</h2>
            <p>{uiText("输入远端「共享这台电脑」页面上的配对码。")}</p>
            <label className="rd-field">
              {uiText("配对码")}
              <input
                autoFocus
                autoComplete="off"
                spellCheck={false}
                maxLength={8}
                value={code}
                placeholder="········"
                onChange={(e) =>
                  setCode(e.target.value.replace(/[^a-fA-F0-9]/g, ""))
                }
                onKeyDown={(e) => {
                  if (e.key === "Enter") void connect();
                }}
              />
            </label>
            <label className="rd-check">
              <input
                type="checkbox"
                role="switch"
                disabled={!selected.control || connecting}
                checked={control && selected.control}
                onChange={(e) => setControl(e.target.checked)}
              />
              <span>{uiText("操作键盘和鼠标")}</span>
            </label>
            {!selected.control&&<p className="rd-control-help" role="note">{uiText("这台电脑只允许查看。要操作键鼠，请在电脑上停止共享，勾选「允许键盘和鼠标操作」，再重新共享并连接。Mac 还需允许辅助功能权限。")}</p>}
            {error && (
              <p className="rd-dialog-error" role="alert">
                {error}
              </p>
            )}
            <button
              className="rd-btn rd-primary rd-full"
              disabled={code.length !== 8 || connecting}
              onClick={() => void connect()}
            >
              {connecting ? (
                <LoaderCircle size={16} className="rd-spin" />
              ) : (
                <ArrowUpRight size={16} />
              )}
              {connecting ? uiText("正在连接…") : uiText("连接")}
            </button>
            <small>{uiText("连接前 myco 会再次向你确认授权。")}</small>
          </section>
        </div>
      )}
      {settings && (
        <div
          className="rd-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setSettings(false);
          }}
        >
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="rd-settings-title"
            className="rd-dialog"
          >
            <button
              className="rd-icon rd-dialog-close"
              aria-label={uiText("关闭")}
              onClick={() => setSettings(false)}
            >
              <X size={18} />
            </button>
            <Settings2 size={27} />
            <h2 id="rd-settings-title">{uiText("远控组件")}</h2>
            <p>
              {uiText(
                "界面随 App 切换语言和深浅色。采集与传输在独立组件中运行。",
              )}
            </p>
            <dl>
              <div>
                <dt>{uiText("安装状态")}</dt>
                <dd>
                  {status?.installed ? uiText("已安装") : uiText("未安装")}
                </dd>
              </div>
              <div>
                <dt>{uiText("组件版本")}</dt>
                <dd>{status?.installed ? status.version : "—"}</dd>
              </div>
              <div>
                <dt>{uiText("启动方式")}</dt>
                <dd>{uiText("使用时启动")}</dd>
              </div>
              <div>
                <dt>{uiText("声音")}</dt>
                <dd>{uiText("当前版本不传输声音")}</dd>
              </div>
            </dl>
            <button
              className="rd-btn rd-full"
              disabled={!status?.installed || !!busy}
              onClick={() => void run("remove")}
            >
              {uiText("卸载组件")}
            </button>
          </section>
        </div>
      )}
    </div>
  );
}
mount(kit, App);
