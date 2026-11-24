import { t as uiText } from "../_lib/i18n";
// 航海: a small boat on a 3D sea to explore. The sea, sky, islands and
// boat are drawn here, live (world.ts and what it uses); the page's
// records hold the world's settings and its islands, so people and agents
// grow the map by adding islands. Where your boat is stays on this
// computer.
import "../_lib/sheet.css";
import "./ocean.css";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Keyboard, Map as MapIcon, SlidersHorizontal, Volume2, VolumeX } from "lucide-react";
import { mount, useKit } from "../_lib/appkit";
import { isleList, kit, loadLocal, saveLocal, setWorld, settingsOf, startOf, worldOf } from "./model";
import { canWebGL, World, type Settings, type Status } from "./world";
import { Sound } from "./audio";
import { SEAS } from "./waves";
import type { Course } from "./map";
import { distText } from "./map";
import { BigMap, Compass, Gauges, IslandCard, KeysHelp, Labels, MiniMap, SettingsPanel, TouchPad, clock, weatherName } from "./hud";

const coarse = () => window.matchMedia?.("(pointer: coarse)").matches ?? false;

function App() {
  useKit(kit);
  const canvas = useRef<HTMLCanvasElement>(null);
  const worldRef = useRef<World | null>(null);
  const statusRef = useRef<Status | null>(null);
  const sound = useRef(new Sound());
  const [status, setStatus] = useState<Status | null>(null);
  const [failed, setFailed] = useState(() => (canWebGL() ? "" : uiText("这台电脑上画不了 3D（浏览器没有 WebGL）。")));
  const [open, setOpen] = useState<"" | "map" | "settings" | "keys">("");
  const [draft, setDraft] = useState<Settings | null>(null);
  const [title, setTitle] = useState<string | null>(null);
  const [hidden, setHidden] = useState<string | null>(null); // the island card closed for
  const [course, setCourse] = useState<Course>(null);
  const [found, setFound] = useState<Set<string>>(() => new Set());
  const [soundOn, setSoundOn] = useState(true);
  const [fresh, setFresh] = useState(true); // the keys hint until the first move
  const [touch] = useState(coarse);

  const w0 = worldOf();
  const settings = draft ?? settingsOf(w0);
  const isles = useMemo(() => isleList(), [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps
  const islesKey = JSON.stringify(isles);
  const get = useCallback(() => statusRef.current, []);

  // the world: made once
  useEffect(() => {
    if (!canvas.current) return; // (no WebGL: the message is shown instead)
    let world: World;
    try { world = new World(canvas.current); } catch (e) { setFailed(uiText("3D 画面没能启动：{0}", (e as Error).message)); return; }
    worldRef.current = world;
    const local = loadLocal();
    setFound(new Set(local.found || []));
    if (typeof local.soundOn === "boolean") { setSoundOn(local.soundOn); sound.current.on = local.soundOn; }
    const s = startOf();
    if (typeof local.x === "number" && typeof local.z === "number") world.moveBoat(local.x, local.z, local.heading || 0);
    else world.moveBoat(s.x, s.z, s.heading);
    let at = 0;
    world.onFrame = (st) => {
      statusRef.current = st;
      const now = performance.now();
      if (now - at > 120) { at = now; setStatus({ ...st }); const s = settingsOf(); sound.current.update(st.boat.speed, st.boat.throttle, SEAS[s.sea].height / 1.9, s.weather === "storm" ? 1 : s.weather === "rain" ? 0.6 : 0); }
    };
    // thunder after the flash: later the further off it struck
    world.onStrike = (d) => sound.current.thunder(d);
    const unbind = world.controls.bind(window);
    world.controls.onAny = () => { setFresh(false); sound.current.start(); };
    const wake = () => sound.current.start();
    window.addEventListener("pointerdown", wake, { once: true });
    world.start();
    (window as unknown as { __ocean?: World }).__ocean = world;
    const save = () => { const b = world.boat; saveLocal({ x: Math.round(b.x * 10) / 10, z: Math.round(b.z * 10) / 10, heading: Math.round(b.heading * 1000) / 1000 }); };
    const timer = window.setInterval(save, 2000);
    window.addEventListener("pagehide", save);
    const vis = () => { if (document.hidden) world.stop(); else world.start(); };
    document.addEventListener("visibilitychange", vis);
    return () => { save(); unbind(); window.clearInterval(timer); window.removeEventListener("pagehide", save); document.removeEventListener("visibilitychange", vis); sound.current.stop(); world.dispose(); worldRef.current = null; };
  }, []);

  // what the page holds, into the world
  const settingsKey = JSON.stringify(settings);
  useEffect(() => { worldRef.current?.setSettings(settings); }, [settingsKey]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { worldRef.current?.setIslands(isles); }, [islesKey]); // eslint-disable-line react-hooks/exhaustive-deps

  // the shared settings: shown at once, saved after a pause
  const saveTimer = useRef(0);
  const change = (s: Settings) => {
    setDraft(s);
    window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(() => { void setWorld({ time: Math.round(s.time * 1000) / 1000, time_at: s.at, time_speed: s.speed, weather: s.weather, sea: s.sea, wind: s.wind }).finally(() => setDraft(null)); }, 450);
  };
  const titleTimer = useRef(0);
  const rename = (t: string) => {
    setTitle(t);
    window.clearTimeout(titleTimer.current);
    titleTimer.current = window.setTimeout(() => { void setWorld({ title: t.trim() }).finally(() => setTitle(null)); }, 500);
  };

  // islands found: the first time you come close
  const near = status?.near ?? null;
  useEffect(() => {
    if (!near || found.has(near.isle.id)) return;
    const next = new Set(found); next.add(near.isle.id);
    setFound(next);
    saveLocal({ found: [...next] });
  }, [near?.isle.id]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (!near) setHidden(null); }, [near?.isle.id]); // eslint-disable-line react-hooks/exhaustive-deps
  // arrived where we were heading
  useEffect(() => { if (course && near && near.isle.name === course.name && near.dist < 60) setCourse(null); }, [near?.dist, course]); // eslint-disable-line react-hooks/exhaustive-deps

  // M for the map, Esc to close
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.key === "Escape") { setOpen(""); (document.activeElement as HTMLElement | null)?.blur?.(); return; }
      const t = e.target as HTMLElement;
      if (t.tagName === "INPUT" || t.tagName === "TEXTAREA") return;
      if (e.code === "KeyM" && !e.metaKey && !e.ctrlKey) { e.preventDefault(); setOpen((o) => (o === "map" ? "" : "map")); }
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, []);

  const toggleSound = () => {
    const on = !soundOn;
    setSoundOn(on); sound.current.setOn(on); sound.current.start(); saveLocal({ soundOn: on });
  };
  const goStart = () => { const s = startOf(); worldRef.current?.moveBoat(s.x, s.z, s.heading); setOpen(""); };
  const startHere = () => {
    const b = worldRef.current?.boat; if (!b) return;
    void setWorld({ start_x: Math.round(b.x), start_z: Math.round(b.z), start_heading: Math.round((b.heading * 180) / Math.PI) });
    kit.flash(uiText("出发点改到了这里"));
  };

  const b = status?.boat;
  const courseBearing = course && b ? Math.atan2(course.x - b.x, -(course.z - b.z)) : null;
  const courseDist = course && b ? Math.hypot(course.x - b.x, course.z - b.z) : 0;
  const name = title ?? (w0.title || uiText("航海"));
  const where = near ? uiText("在 {0} 附近", near.isle.name || uiText("无名")) : uiText("开阔海面");
  const aground = b && b.aground < 1.2;

  if (failed) return <div className="oc oc-fail"><div><b>{uiText("打不开 3D 海面")}</b><p>{failed}</p></div></div>;
  return (
    <div className="oc">
      <canvas ref={canvas} className="oc-canvas" tabIndex={0} aria-label={uiText("海面：用 W A S D 或方向键开船")} />
      {status && <Labels labels={status.labels} />}
      <div className="oc-top">
        <div className="oc-chip"><b>{name}</b><span className="oc-num">{clock(status?.hour ?? settings.time)} · {weatherName(settings.weather)}</span><span>{where}</span></div>
        <span className="top-gap" />
        <div className="oc-chip oc-tools">
          <button type="button" className="icon-btn" onClick={() => setOpen(open === "map" ? "" : "map")} aria-label={uiText("地图（M）")} title={uiText("地图（M）")}><MapIcon className="ico" /></button>
          <button type="button" className={`icon-btn${open === "settings" ? " is-on" : ""}`} onClick={() => setOpen(open === "settings" ? "" : "settings")} aria-label={uiText("海况和天气")} title={uiText("海况和天气")}><SlidersHorizontal className="ico" /></button>
          <button type="button" className="icon-btn" onClick={toggleSound} aria-label={soundOn ? uiText("关掉声音") : uiText("打开声音")} title={soundOn ? uiText("关掉声音") : uiText("打开声音")}>{soundOn ? <Volume2 className="ico" /> : <VolumeX className="ico" />}</button>
          <button type="button" className={`icon-btn${open === "keys" ? " is-on" : ""}`} onClick={() => setOpen(open === "keys" ? "" : "keys")} aria-label={uiText("操作方法")} title={uiText("操作方法")}><Keyboard className="ico" /></button>
        </div>
      </div>
      {near && hidden !== near.isle.id && open !== "map" && <IslandCard isle={near.isle} dist={near.dist} onClose={() => setHidden(near.isle.id)} />}
      {course && b && open !== "map" && <button type="button" className="oc-chip oc-goal" onClick={() => setCourse(null)} title={uiText("取消航向")}>{uiText("开往 {0} · {1}", course.name, distText(courseDist))}</button>}
      {aground && <div className="oc-chip oc-alert" role="alert">{uiText("触底了，倒车或转个方向")}</div>}
      {b && (
        <div className="oc-bottom">
          {!touch && <MiniMap get={get} isles={isles} course={course} onOpen={() => setOpen("map")} />}
          <div className="oc-dash">
            <Compass heading={b.heading} course={courseBearing} />
            <Gauges s={status!} />
          </div>
        </div>
      )}
      {fresh && !touch && open === "" && <div className="oc-hint">{uiText("W/S 油门 · A/D 转舵 · 拖动看四周 · M 地图")}</div>}
      {touch && status && worldRef.current && <TouchPad controls={worldRef.current.controls} />}
      {open === "settings" && <SettingsPanel value={settings} hour={status?.hour ?? settings.time} title={name} onChange={change} onTitle={rename} onStartHere={startHere} onGoStart={goStart} onClose={() => setOpen("")} />}
      {open === "keys" && <KeysHelp onClose={() => setOpen("")} />}
      {open === "map" && <BigMap get={get} isles={isles} found={found} course={course} onCourse={setCourse} onClose={() => setOpen("")} />}
      {kit.note && <div className="note" role="status">{kit.note.text}</div>}
    </div>
  );
}

mount(kit, App);
