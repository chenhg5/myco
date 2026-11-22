// The NES core only receives locally selected bytes. No ROM, save or filename
// crosses the page bridge. Keyboard and touch feed the same controller ports.
import { useEffect, useRef, useState } from "react";
import { Controller, NES, type ButtonKey } from "jsnes";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Maximize, Minimize, Pause, Play, RotateCcw, Upload, Volume2, VolumeX } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { NESAudio, nesSampleRate } from "./nes-audio";
import { MAX_NES_ROM, validNESROM } from "./nes-rom";

const width = 256, height = 240, frameTime = 1000 / 60;
const button = Controller;
const bindings: Record<string, ButtonKey> = {
  ArrowUp: button.BUTTON_UP, ArrowDown: button.BUTTON_DOWN, ArrowLeft: button.BUTTON_LEFT, ArrowRight: button.BUTTON_RIGHT,
  KeyX: button.BUTTON_A, KeyZ: button.BUTTON_B, ShiftRight: button.BUTTON_SELECT, Enter: button.BUTTON_START,
};

export function NESPanel() {
  const panel = useRef<HTMLElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const core = useRef<NES>();
  const cartridge = useRef<Uint8Array>();
  const audio = useRef<NESAudio>();
  const keys = useRef(new Map<string, ButtonKey>());
  const pressedSinceFrame = useRef(new Set<ButtonKey>());
  const pendingRelease = useRef(new Set<ButtonKey>());
  const swap = useRef(false);
  const load = useRef(0);
  const [fullscreen, setFullscreen] = useState(false);
  const [swapped, setSwapped] = useState(false);
  const [filename, setFilename] = useState("");
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(true);
  const [held, setHeld] = useState<ButtonKey[]>([]);
  const [error, setError] = useState("");

  function mapped(code: ButtonKey): ButtonKey {
    if (swap.current && code === button.BUTTON_A) return button.BUTTON_B;
    if (swap.current && code === button.BUTTON_B) return button.BUTTON_A;
    return code;
  }

  function input(source: string, code: ButtonKey, down: boolean, cancel = false) {
    if (!core.current) return;
    if (down) {
      if (keys.current.has(source)) return;
      if (![...keys.current.values()].includes(code) && !pendingRelease.current.delete(code)) {
        core.current.buttonDown(1, mapped(code));
        pressedSinceFrame.current.add(code);
      }
      keys.current.set(source, code);
    } else {
      if (!keys.current.delete(source)) return;
      if (![...keys.current.values()].includes(code)) {
        if (!cancel && pressedSinceFrame.current.has(code)) pendingRelease.current.add(code);
        else core.current.buttonUp(1, mapped(code));
      }
    }
    setHeld([...keys.current.values()]);
  }

  function release() {
    for (const code of new Set([...keys.current.values(), ...pendingRelease.current])) {
      core.current?.buttonUp(1, mapped(code));
    }
    keys.current.clear();
    pendingRelease.current.clear();
    pressedSinceFrame.current.clear();
    setHeld([]);
  }

  function toggleFullscreen() {
    const element = panel.current;
    if (!element) return;
    if (fullscreen) {
      if (document.fullscreenElement === element) void document.exitFullscreen().catch(() => undefined);
      else setFullscreen(false);
    } else {
      setFullscreen(true);
      // iPhone's embedded web view can use the page-sized layout even
      // when the host cannot grant element fullscreen.
      if (document.fullscreenEnabled && element.requestFullscreen) {
        void element.requestFullscreen().catch(() => undefined);
      }
    }
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  }

  useEffect(() => {
    const down = (event: KeyboardEvent) => {
      if (!core.current || event.ctrlKey || event.altKey || event.metaKey ||
          (event.target instanceof HTMLElement && event.target.closest("input, select, textarea, button, a"))) return;
      const code = bindings[event.code];
      if (code === undefined) return;
      event.preventDefault();
      input(event.code, code, true);
    };
    const up = (event: KeyboardEvent) => {
      const code = bindings[event.code];
      if (code !== undefined) input(event.code, code, false);
    };
    const hidden = () => { if (document.hidden) { release(); setPlaying(false); } };
    const element = panel.current;
    let native = false;
    const syncFullscreen = () => {
      if (document.fullscreenElement === element) native = true;
      else if (native) { native = false; setFullscreen(false); }
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && document.fullscreenElement !== element) setFullscreen(false);
    };
    document.addEventListener("fullscreenchange", syncFullscreen);
    window.addEventListener("keydown", escape);
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    window.addEventListener("blur", release);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      ++load.current;
      document.removeEventListener("fullscreenchange", syncFullscreen);
      window.removeEventListener("keydown", escape);
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
      window.removeEventListener("blur", release);
      document.removeEventListener("visibilitychange", hidden);
      audio.current?.close();
    };
  }, []);

  useEffect(() => {
    if (!playing) return;
    let previous = performance.now(), elapsed = 0, request = 0;
    function tick(now: number) {
      elapsed += Math.min(now - previous, frameTime * 3);
      previous = now;
      try {
        while (elapsed >= frameTime) {
          elapsed -= frameTime;
          core.current?.frame();
          // A quick tap can start and end between two emulated frames.
          for (const code of pendingRelease.current) core.current?.buttonUp(1, mapped(code));
          pendingRelease.current.clear();
          pressedSinceFrame.current.clear();
          audio.current?.flush();
        }
        request = requestAnimationFrame(tick);
      } catch {
        setError(uiText("NES 程序已停止。请尝试重新载入 ROM。"));
        setPlaying(false);
      }
    }
    request = requestAnimationFrame(tick);
    return () => { cancelAnimationFrame(request); audio.current?.clear(); };
  }, [playing]);

  function start(bytes: Uint8Array) {
    const context = canvas.current?.getContext("2d");
    if (!context) throw new Error("Canvas is unavailable");
    const image = context.createImageData(width, height);
    const pixels = new Uint32Array(image.data.buffer);
    const next = new NES({
      sampleRate: nesSampleRate,
      onFrame: frame => {
        for (let i = 0; i < pixels.length; i++) pixels[i] = 0xff000000 | frame[i];
        context.putImageData(image, 0, 0);
      },
      onAudioSample: (left, right) => audio.current?.sample(left, right),
    });
    next.loadROM(bytes);
    release();
    audio.current?.clear();
    core.current = next;
    cartridge.current = bytes;
    setError("");
    setPlaying(true);
  }

  async function importFile(file?: File) {
    if (!file) return;
    const attempt = ++load.current;
    try {
      if (file.size < 16 || file.size > MAX_NES_ROM) throw new Error("size");
      const bytes = new Uint8Array(await file.arrayBuffer());
      if (attempt !== load.current) return;
      if (!validNESROM(bytes)) throw new Error("invalid");
      try { start(bytes); } catch (failure) {
        setError(uiText("这个 NES ROM 无法启动：{0}", failure instanceof Error ? failure.message : ""));
        return;
      }
      setFilename(file.name);
      if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
    } catch {
      if (attempt === load.current) setError(uiText("请选择完整的 NES 卡带文件（.nes，最大 16 MB）。"));
    }
  }

  async function sound() {
    if (audio.current) { audio.current.close(); audio.current = undefined; setMuted(true); return; }
    let next: NESAudio | undefined;
    try {
      next = new NESAudio();
      audio.current = next;
      await next.context.resume();
      if (audio.current === next) setMuted(false);
    } catch {
      if (audio.current === next) { next?.close(); audio.current = undefined; }
      setError(uiText("音频无法启动。"));
    }
  }

  function touch(code: ButtonKey, label: string, content: React.ReactNode, name?: string) {
    return <button type="button" className={name} aria-label={label} aria-pressed={held.includes(code)} disabled={!filename}
      onContextMenu={event => event.preventDefault()}
      onPointerDown={event => {
        if (event.button !== 0) return;
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        input("touch-" + event.pointerId, code, true);
      }}
      onPointerUp={event => input("touch-" + event.pointerId, code, false)}
      onPointerCancel={event => input("touch-" + event.pointerId, code, false, true)}
      onLostPointerCapture={event => input("touch-" + event.pointerId, code, false, true)}
      onKeyDown={event => { if (event.key === " " || event.key === "Enter") { event.preventDefault(); input("focus-" + code, code, true); } }}
      onKeyUp={event => { if (event.key === " " || event.key === "Enter") input("focus-" + code, code, false); }}
      onBlur={() => input("focus-" + code, code, false, true)}>{content}</button>;
  }

  return <main ref={panel} className={fullscreen ? "arc-layout arc-nes arc-nes-full" : "arc-layout arc-nes"}>
    <section className="arc-cabinet">
      <div className="arc-title"><h2>{filename || uiText("NES 卡带")}</h2>{filename && <span role="status">{playing ? uiText("运行中") : uiText("已暂停")}</span>}</div>
      <div className="arc-screen">
        <canvas ref={canvas} width={width} height={height} role="img" aria-label={uiText("游戏画面")}/>
        {!filename && <p>{uiText("导入 .nes 卡带即可游玩")}</p>}
      </div>
      <div className="arc-tools">
        <input ref={picker} hidden type="file" accept=".nes,application/octet-stream" onChange={event => {
          void importFile(event.target.files?.[0]); event.target.value = "";
        }}/>
        <button type="button" className="btn btn-primary arc-nes-import" onClick={() => picker.current?.click()}>
          <Upload size={15}/>{uiText("导入 .nes")}
        </button>
        <button type="button" className="btn" disabled={!filename} onClick={() => {
          if (playing) release();
          setPlaying(!playing);
          if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
        }}>{playing ? <Pause size={15}/> : <Play size={15}/>} {playing ? uiText("暂停") : uiText("继续游戏")}</button>
        <button type="button" className="btn" disabled={!filename} onClick={() => {
          if (cartridge.current) start(cartridge.current);
          if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
        }}><RotateCcw size={15}/>{uiText("重启")}</button>
        <button type="button" className="btn" disabled={!filename} onClick={() => {
          void sound();
          if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
        }}>{muted ? <VolumeX size={15}/> : <Volume2 size={15}/>} {muted ? uiText("开启声音") : uiText("关闭声音")}</button>
        <button type="button" className="btn arc-nes-full-toggle" disabled={!filename}
          aria-pressed={fullscreen} onClick={toggleFullscreen}>
          {fullscreen ? <Minimize size={15}/> : <Maximize size={15}/>}
          {fullscreen ? uiText("退出全屏") : uiText("全屏")}
        </button>
      </div>
      {error && <p className="arc-error" role="alert">{error}</p>}
      <p className="arc-nes-file-hint">{uiText("卡带只在当前设备读取。将附件保存到这台设备后即可选择。")}</p>
    </section>
    <aside className="arc-side">
      <div className="arc-controls">
        <h2>{uiText("NES 手柄")}</h2>
        <div className="arc-nes-pad">
          <div className="arc-nes-direction">
            {touch(button.BUTTON_UP, uiText("方向键上"), <ChevronUp/>, "arc-nes-up")}
            {touch(button.BUTTON_LEFT, uiText("方向键左"), <ChevronLeft/>, "arc-nes-left")}
            {touch(button.BUTTON_RIGHT, uiText("方向键右"), <ChevronRight/>, "arc-nes-right")}
            {touch(button.BUTTON_DOWN, uiText("方向键下"), <ChevronDown/>, "arc-nes-down")}
          </div>
          <div className="arc-nes-action">
            {touch(button.BUTTON_B, uiText("动作 B"), "B")}
            {touch(button.BUTTON_A, uiText("动作 A"), "A")}
          </div>
          <div className="arc-nes-utility">
            {touch(button.BUTTON_SELECT, uiText("选择键"), "SELECT")}
            {touch(button.BUTTON_START, uiText("开始游戏"), "START")}
          </div>
        </div>
        <label className="arc-nes-swap">
          <input type="checkbox" checked={swapped} onChange={event => {
            release();
            swap.current = event.target.checked;
            setSwapped(swap.current);
            event.target.blur();
          }}/>{uiText("交换 A/B")}
        </label>
        <p>{swapped ? uiText("键盘：方向键移动，Z 为 A，X 为 B，Enter 开始。")
          : uiText("键盘：方向键移动，Z 为 B，X 为 A，Enter 开始。")}</p>
      </div>
    </aside>
  </main>;
}
