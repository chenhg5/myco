// A touch keypad for the existing authorized desktop input channel.
// Each pointer owns one physical key until release, cancel, or focus loss.
import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Monitor, Unplug } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import type { DesktopSession } from "../_lib/remote";

export function Gamepad({ session, close, disconnect }: {
  session: DesktopSession; close: () => void; disconnect: () => void;
}) {
  const pointers = useRef(new Map<number, string>());
  const [held, setHeld] = useState<string[]>([]);
  const [failed, setFailed] = useState(false);

  function releaseAll() {
    for (const code of new Set(pointers.current.values())) session.input({ kind: "key", code, down: false });
    pointers.current.clear();
  }

  useEffect(() => {
    function clear() { releaseAll(); setHeld([]); }
    function hidden() { if (document.hidden) clear(); }
    window.addEventListener("blur", clear);
    window.addEventListener("pagehide", clear);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      releaseAll();
      window.removeEventListener("blur", clear);
      window.removeEventListener("pagehide", clear);
      document.removeEventListener("visibilitychange", hidden);
    };
  }, [session]);

  function release(pointer: number) {
    const code = pointers.current.get(pointer);
    if (!code) return;
    pointers.current.delete(pointer);
    if (![...pointers.current.values()].includes(code)) session.input({ kind: "key", code, down: false });
    setHeld([...pointers.current.values()]);
  }

  function button(code: string, label: string, content: React.ReactNode, style?: string) {
    return <button type="button" className={style} aria-label={label} aria-pressed={held.includes(code)}
      onContextMenu={event => event.preventDefault()}
      onPointerDown={event => {
        if (event.button !== 0 || pointers.current.has(event.pointerId)) return;
        event.preventDefault();
        const alreadyHeld = [...pointers.current.values()].includes(code);
        if (!alreadyHeld && !session.input({ kind: "key", code, down: true })) { setFailed(true); return; }
        event.currentTarget.setPointerCapture(event.pointerId);
        pointers.current.set(event.pointerId, code);
        setHeld([...pointers.current.values()]);
        setFailed(false);
      }}
      onPointerUp={event => release(event.pointerId)}
      onPointerCancel={event => release(event.pointerId)}
      onLostPointerCapture={event => release(event.pointerId)}>
      {content}
    </button>;
  }

  return <div className="rd-gamepad" aria-label={uiText("街机手柄")}>
    <header>
      <button type="button" className="rd-btn" onClick={() => { releaseAll(); close(); }}><Monitor size={16}/>{uiText("返回画面")}</button>
      <h2>{uiText("街机手柄")}</h2>
      <button type="button" className="rd-btn" onClick={() => { releaseAll(); disconnect(); }}><Unplug size={16}/>{uiText("断开")}</button>
    </header>
    <div className="rd-gamepad-controls">
      <div className="rd-gamepad-directions">
        {button("ArrowUp", uiText("方向键上"), <ChevronUp/>, "rd-pad-up")}
        {button("ArrowLeft", uiText("方向键左"), <ChevronLeft/>, "rd-pad-left")}
        {button("ArrowRight", uiText("方向键右"), <ChevronRight/>, "rd-pad-right")}
        {button("ArrowDown", uiText("方向键下"), <ChevronDown/>, "rd-pad-down")}
      </div>
      <div className="rd-gamepad-actions">
        {button("ControlLeft", uiText("动作 A"), <><b>A</b><small>Ctrl</small></>)}
        {button("AltLeft", uiText("动作 B"), <><b>B</b><small>Alt</small></>)}
      </div>
    </div>
    <div className="rd-gamepad-utility">
      {button("Digit5", uiText("投币"), uiText("投币"))}
      {button("Digit1", uiText("开始游戏"), uiText("开始游戏"))}
      {button("KeyP", uiText("暂停游戏"), uiText("暂停游戏"))}
    </div>
    {failed ? <p role="status">{uiText("手柄尚未连上，请检查远程连接。")}</p> :
      <p>{uiText("请让电脑上的 MAME 游戏窗口保持在前台。")}</p>}
  </div>;
}
