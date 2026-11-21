import { useEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { myco } from "../_lib/myco";
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
  const [keyboardOpen,setKeyboardOpen]=useState(false),[draft,setDraft]=useState(""),[dragging,setDragging]=useState(false),[modifiers,setModifiers]=useState<string[]>([]),[cursor,setCursor]=useState({x:.5,y:.5}),[showCursor,setShowCursor]=useState(false),[,setLayout]=useState(0);
  const keyboard=useRef<HTMLTextAreaElement>(null),composing=useRef(false);
  const touchpad=useRef<Touchpad>();
  if(!touchpad.current)touchpad.current=new Touchpad(e=>session.current?.input(e),p=>setCursor({...p}));
  useEffect(()=>{const resize=()=>setLayout(n=>n+1);window.addEventListener("resize",resize);window.visualViewport?.addEventListener("resize",resize);return()=>{window.removeEventListener("resize",resize);window.visualViewport?.removeEventListener("resize",resize);};},[]);
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
    touchpad.current?.reset();setDragging(false);setModifiers([]);setKeyboardOpen(false);setDraft("");setShowCursor(false);
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
    if(composing.current)return;
    const events=textInput(value,!!session.current?.supportsText,modifiers);
    if(events===null){setError(uiText("远端组件需要更新才能输入这段文字，文字已保留。"));return;}
    if(!events.length)return;
    for(const e of events)if(!input(e))return;
    releaseModifiers();setDraft("");if(keyboard.current)keyboard.current.value="";
  };
  const touch=(e:React.PointerEvent,phase:"down"|"move"|"up")=>{
    if(e.pointerType!=="touch")return false;
    if(!control)return true;
    e.preventDefault();setShowCursor(true);
    const pad=touchpad.current!,box=e.currentTarget.getBoundingClientRect();
    if(phase==="down"){e.currentTarget.setPointerCapture(e.pointerId);pad.down(e.pointerId,e.clientX,e.clientY);}
    else if(phase==="move")pad.move(e.pointerId,e.clientX,e.clientY,box.width,box.height);
    else pad.up(e.pointerId);
    return true;
  };
  const cursorPosition=()=>{
    const v=video.current;if(!v||!v.videoWidth)return {left:"50%",top:"50%"};
    const scale=Math.min(v.clientWidth/v.videoWidth,v.clientHeight/v.videoHeight),w=v.videoWidth*scale,h=v.videoHeight*scale;
    return {left:(v.clientWidth-w)/2+cursor.x*w,top:(v.clientHeight-h)/2+cursor.y*h};
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
          <section className="rd-session">
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
                  <Keyboard size={14} />
                  {control ? uiText("可以操作") : uiText("仅查看")}
                </span>
                <button
                  className="rd-icon"
                  aria-label={uiText("全屏")}
                  onClick={() =>
                    void stage.current?.requestFullscreen?.().catch(() => {})
                  }
                >
                  <Maximize2 size={17} />
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
              aria-label={uiText("远程画面，点击后使用键盘和鼠标")}
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
              onPointerCancel={() => {touchpad.current?.reset();setDragging(false);setModifiers([]);}}
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
                autoPlay
                muted
                playsInline
                onLoadedMetadata={()=>setLayout(n=>n+1)}
                aria-label={uiText("远程桌面画面")}
              />
              {showCursor&&control&&<span className="rd-touch-cursor" aria-hidden="true" style={cursorPosition()}/>}
            </div>
            {control&&<div className="rd-touch-controls">
              <p>{uiText("触控板：单指移动，轻点单击，双指滚动。")}</p>
              <div className="rd-touch-row">
                <button className="rd-btn" aria-pressed={keyboardOpen} onClick={()=>{flushSync(()=>setKeyboardOpen(!keyboardOpen));if(!keyboardOpen)keyboard.current?.focus();}}><Keyboard size={16}/>{uiText("键盘")}</button>
                <button className="rd-btn" onClick={()=>touchpad.current?.click(2)}>{uiText("右键")}</button>
                <button className="rd-btn" aria-pressed={dragging} onClick={()=>{touchpad.current?.drag(!dragging);setDragging(!dragging);}}>{dragging?uiText("结束拖动"):uiText("拖动")}</button>
                {["Escape","Tab","Enter","Backspace"].map(code=><button key={code} className="rd-btn" aria-label={code} onClick={()=>specialKey(code)}>{({Escape:"Esc",Tab:"Tab",Enter:"↵",Backspace:"⌫"})[code as "Enter"]}</button>)}
              </div>
              {keyboardOpen&&<div className="rd-keyboard">
                <div className="rd-touch-row">{[["ControlLeft","Ctrl"],["AltLeft","Alt"],["MetaLeft",active.os==="darwin"?"⌘":"Win"],["ShiftLeft","Shift"]].map(([code,label])=><button key={code} className="rd-btn" aria-pressed={modifiers.includes(code)} onClick={()=>{const down=!modifiers.includes(code);input({kind:"key",code,down});setModifiers(down?[...modifiers,code]:modifiers.filter(x=>x!==code));keyboard.current?.focus();}}>{label}</button>)}</div>
                <textarea ref={keyboard} aria-label={uiText("输入文字到远端")} placeholder={uiText("键入或粘贴文字")} value={draft} rows={1} autoCapitalize="off" autoCorrect="off" spellCheck={false}
                  onChange={e=>{setDraft(e.target.value);if(!(e.nativeEvent as InputEvent).isComposing)typed(e.target.value);}}
                  onCompositionStart={()=>{composing.current=true;}} onCompositionEnd={e=>{composing.current=false;typed(e.currentTarget.value);}}
                  onKeyDown={e=>{if(e.key==="Enter"&&!composing.current&&!(e.nativeEvent as KeyboardEvent).isComposing){e.preventDefault();if(draft)typed(draft);specialKey("Enter");}if(e.key==="Backspace"&&!draft&&!composing.current){e.preventDefault();specialKey("Backspace");}}}
                />
              </div>}
            </div>}
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
                    {!query && (
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
        <span>{uiText("按需安装 · 默认关闭 · 随时断开")}</span>
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
                disabled={!selected.control || connecting}
                checked={control && selected.control}
                onChange={(e) => setControl(e.target.checked)}
              />
              <span>{uiText("操作键盘和鼠标")}</span>
            </label>
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
