// The cartridge and machine state stay on this device. The canvas reads
// the interpreter's framebuffer and sends pointer and keyboard input.
import { createRoot } from "react-dom/client";
import { useEffect, useRef, useState } from "react";
import { Gamepad2, Pause, Play, RotateCcw, Upload, Volume2, VolumeX } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { Chip8, height, maxROM, width } from "./chip8";
import { demoROM } from "./demo";
import { osState } from "../_lib/os";
import { subscribeAppRoute } from "../_lib/page-route";
import { NESPanel } from "./nes-panel";
import { MamePanel } from "./mame-panel";
import "../_lib/sheet.css";
import "./arcade.css";

const layout = [1, 2, 3, 12, 4, 5, 6, 13, 7, 8, 9, 14, 10, 0, 11, 15];
const keyMap = new Map("1234qwerasdfzxcv".split("").map((key, index) => [key, layout[index]]));
for (const [key, value] of [["ArrowUp", 2], ["ArrowLeft", 4], ["ArrowRight", 6], ["ArrowDown", 8]] as const) keyMap.set(key, value);

function paint(ctx: CanvasRenderingContext2D, core: Chip8) {
  ctx.fillStyle = "#121d19";
  ctx.fillRect(0, 0, width, height);
  ctx.fillStyle = "#bceab4";
  core.screen.forEach((pixel, index) => { if (pixel) ctx.fillRect(index % width, Math.floor(index / width), 1, 1); });
  core.dirty = false;
}

function ChipView() {
  const [cart, setCart] = useState(() => ({ rom: demoROM(), name: "" }));
  const [classic, setClassic] = useState(false);
  const [speed, setSpeed] = useState(720);
  const [running, setRunning] = useState(false);
  const [muted, setMuted] = useState(true);
  const [error, setError] = useState("");
  const [pressed, setPressed] = useState<number[]>([]);
  const machine = useRef<Chip8>();
  const audio = useRef<{ context: AudioContext; gain: GainNode; tone: OscillatorNode }>();
  const canvas = useRef<HTMLCanvasElement>(null);
  const chooser = useRef<HTMLInputElement>(null);
  const inputs = useRef(new Map<string, number>());
  const load = useRef(0);

  function input(source: string, key: number, down: boolean) {
    if (!machine.current) return;
    if (down) {
      inputs.current.set(source, key);
      machine.current.key(key, true);
    } else {
      if (!inputs.current.delete(source)) return;
      if (![...inputs.current.values()].includes(key)) machine.current.key(key, false);
    }
    setPressed([...inputs.current.values()]);
  }

  useEffect(() => {
    const down = (event: KeyboardEvent) => {
      if (event.ctrlKey || event.metaKey || event.altKey || (event.target instanceof Element && event.target.closest("input,select,textarea"))) return;
      const key = keyMap.get(event.key.length === 1 ? event.key.toLowerCase() : event.key);
      if (key === undefined) return;
      event.preventDefault();
      if (!event.repeat) input("key-" + event.code, key, true);
    };
    const up = (event: KeyboardEvent) => {
      const source = "key-" + event.code;
      const key = inputs.current.get(source);
      if (key !== undefined) input(source, key, false);
    };
    const clear = () => {
      for (const key of inputs.current.values()) machine.current?.key(key, false);
      inputs.current.clear();
      setPressed([]);
    };
    const hidden = () => { if (document.hidden) { clear(); setRunning(false); } };
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    window.addEventListener("blur", clear);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
      window.removeEventListener("blur", clear);
      document.removeEventListener("visibilitychange", hidden);
      if (audio.current) { void audio.current.context.close(); audio.current = undefined; }
    };
  }, []);

  useEffect(() => {
    const next = new Chip8(cart.rom, classic);
    machine.current = next;
    inputs.current.clear();
    setPressed([]);
    setError("");
    // Preview one short slice of the built-in cartridge before pressing play.
    if (!cart.name) for (let i = 0; i < 45; i++) { if (!next.step()) next.tick(); }
    const ctx = canvas.current?.getContext("2d");
    if (ctx) paint(ctx, next);
  }, [cart, classic]);

  useEffect(() => {
    const ctx = canvas.current?.getContext("2d");
    const core = machine.current;
    if (!running || !ctx || !core) return;
    let frame = 0, last = performance.now(), timers = 0, cycles = 0;
    const run = (now: number) => {
      const elapsed = Math.min(50, now - last);
      last = now;
      timers += elapsed * 0.06;
      cycles += elapsed * speed / 1000;
      try {
        while (timers >= 1) { core.tick(); timers--; }
        while (cycles >= 1) {
          cycles--;
          if (!core.step()) { cycles %= 1; break; }
        }
        if (core.dirty) paint(ctx, core);
        if (audio.current) audio.current.gain.gain.value = !muted && core.sound > 0 ? 0.055 : 0;
        frame = requestAnimationFrame(run);
      } catch (failure) {
        setError(uiText("程序已停止：{0}", failure instanceof Error ? failure.message : ""));
        setRunning(false);
      }
    };
    frame = requestAnimationFrame(run);
    return () => {
      cancelAnimationFrame(frame);
      if (audio.current) audio.current.gain.gain.value = 0;
    };
  }, [running, speed, cart, classic, muted]);

  async function readFile(file?: File) {
    if (!file) return;
    const attempt = ++load.current;
    try {
      if (!/\.(ch8|c8|rom|bin)$/i.test(file.name) || file.size < 2 || file.size > maxROM) {
        throw new Error("format");
      }
      const rom = new Uint8Array(await file.arrayBuffer());
      if (attempt !== load.current) return;
      if (rom.length !== file.size) throw new Error("read");
      setCart({ rom, name: file.name });
      setRunning(true);
    } catch {
      if (attempt === load.current) setError(uiText("请选择 2 到 3584 字节的 CHIP-8 ROM。"));
    }
  }

  async function toggleSound() {
    if (!muted) { setMuted(true); return; }
    try {
      if (!audio.current) {
        const context = new AudioContext();
        const gain = context.createGain();
        const tone = context.createOscillator();
        gain.gain.value = 0;
        tone.type = "square";
        tone.frequency.value = 440;
        tone.connect(gain).connect(context.destination);
        tone.start();
        audio.current = { context, gain, tone };
      }
      await audio.current.context.resume();
      setMuted(false);
    } catch { setError(uiText("音频无法启动。")); }
  }

  const name = cart.name || uiText("轨道信使");
  return <>
      <input ref={chooser} hidden type="file" accept=".ch8,.c8,.rom,.bin" onChange={event => {
        void readFile(event.target.files?.[0]); event.target.value = "";
      }}/>
    <main className="arc-layout">
      <section className="arc-cabinet">
        <div className="arc-title"><h2>{name}</h2><span role="status">{running ? uiText("运行中") : uiText("已暂停")}</span></div>
        <div className="arc-screen"><canvas ref={canvas} width={width} height={height} role="img" aria-label={uiText("游戏画面")}/></div>
        <div className="arc-tools">
          <button type="button" className="btn btn-primary" onClick={() => chooser.current?.click()}><Upload size={15}/>{uiText("打开 ROM")}</button>
          <button type="button" className="btn" onClick={() => setRunning(!running)}>
            {running ? <Pause size={16}/> : <Play size={16}/>} {running ? uiText("暂停") : uiText("开始")}
          </button>
          <button type="button" className="btn" onClick={() => { ++load.current; setCart({ ...cart }); }}><RotateCcw size={15}/>{uiText("重启")}</button>
          <button type="button" className="btn" onClick={() => void toggleSound()}>
            {muted ? <VolumeX size={16}/> : <Volume2 size={16}/>} {muted ? uiText("开启声音") : uiText("关闭声音")}
          </button>
        </div>
        {error && <p role="alert" className="arc-error">{error}</p>}
        {!cart.name && <p>{uiText("用方向键驾驶飞船，收集信标。触屏可按 2、4、6、8。")}</p>}
      </section>
      <aside className="arc-side">
        <div className="arc-controls">
          <h2>{uiText("键盘与触屏")}</h2>
          <div className="arc-keys">
            {layout.map((key, index) => {
              const label = key.toString(16).toUpperCase();
              return <button key={key} type="button" aria-label={uiText("按键 {0}", label)} aria-pressed={pressed.includes(key)}
                onPointerDown={event => { event.currentTarget.setPointerCapture(event.pointerId); input("touch-" + event.pointerId, key, true); }}
                onLostPointerCapture={event => input("touch-" + event.pointerId, key, false)}
                onPointerUp={event => input("touch-" + event.pointerId, key, false)}
                onPointerCancel={event => input("touch-" + event.pointerId, key, false)}
                onKeyDown={event => { if (event.key === " " || event.key === "Enter") { event.preventDefault(); input("button-" + key, key, true); } }}
                onKeyUp={event => { if (event.key === " " || event.key === "Enter") input("button-" + key, key, false); }}
                onBlur={() => input("button-" + key, key, false)}>
                <b>{label}</b><small>{"1234QWERASDFZXCV"[index]}</small>
              </button>;
            })}
          </div>
        </div>
        <div className="arc-settings">
          <label>{uiText("兼容模式")}<select className="sel" value={classic ? "vip" : "modern"} onChange={event => setClassic(event.target.value === "vip")}>
            <option value="modern">{uiText("现代 CHIP-8")}</option><option value="vip">COSMAC VIP</option>
          </select></label>
          <label>{uiText("运行速度")}<select className="sel" value={speed} onChange={event => setSpeed(Number(event.target.value))}>
            <option value={360}>0.5×</option><option value={720}>1×</option><option value={1440}>2×</option>
          </select></label>
          <button type="button" className="btn" onClick={() => { ++load.current; setCart({ rom: demoROM(), name: "" }); setRunning(false); setClassic(false); }}>{uiText("载入原创演示")}</button>
          <p>{uiText("支持 CHIP-8 ROM。文件只在本机运行。")}</p>
        </div>
      </aside>
    </main>
  </>;
}

function App() {
  const [mode, setMode] = useState<"mame" | "chip" | "nes">(
    () => location.hash === "#nes" ? "nes" : osState === "ready" ? "mame" : "chip",
  );
  useEffect(() => subscribeAppRoute(() => {
    if (location.hash === "#nes") setMode("nes");
  }), []);
  return <div className="arc-app">
    <header className="arc-header">
      <div><Gamepad2 size={21}/><h1>{uiText("街机模拟器")}</h1></div>
      <div className="arc-modes">
        <button type="button" className="btn" aria-pressed={mode === "mame"} onClick={() => setMode("mame")}>MAME</button>
        <button type="button" className="btn" aria-pressed={mode === "nes"} onClick={() => setMode("nes")}>NES</button>
        <button type="button" className="btn" aria-pressed={mode === "chip"} onClick={() => setMode("chip")}>CHIP‑8</button>
      </div>
    </header>
    <div hidden={mode !== "mame"}><MamePanel/></div>
    {mode === "chip" && <ChipView/>}
    {mode === "nes" && <NESPanel/>}
  </div>;
}

createRoot(document.getElementById("root")!).render(<App/>);
