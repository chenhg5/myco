import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { t as uiText } from "../_lib/i18n";

export type ImageNavigation = { index: number; count: number; move: (direction: number) => void };

export function ImageViewer({ src, alt, original, navigation, close }: {
  src: string; alt: string; original: boolean; navigation: ImageNavigation; close: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const stage = useRef<HTMLDivElement>(null);
  const [actual, setActual] = useState(false);
  useEffect(() => { dialog.current?.showModal(); }, []);
  useEffect(() => { stage.current?.scrollTo(0, 0); }, [src, actual]);
  return <dialog ref={dialog} className="mk-viewer" aria-label={uiText("大图查看")} onClose={close}
    onKeyDown={event => {
      if (navigation.count < 2 || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) return;
      event.preventDefault();
      navigation.move(event.key === "ArrowLeft" ? -1 : 1);
    }}>
    <header className="mk-viewer-toolbar">
      <button className="btn" type="button" aria-label={uiText("上一个素材")} disabled={navigation.count < 2} onClick={() => navigation.move(-1)}><ChevronLeft size={16}/></button>
      <span>{navigation.index + 1} / {navigation.count}</span>
      <button className="btn" type="button" aria-label={uiText("下一个素材")} disabled={navigation.count < 2} onClick={() => navigation.move(1)}><ChevronRight size={16}/></button>
      <span className="mk-spacer"/>
      <button className="btn" type="button" aria-disabled={!original} aria-pressed={actual && original} onClick={() => { if (original) setActual(!actual); }}>
        {actual && original ? uiText("适应窗口") : "100%"}
      </button>
      <button className="btn" type="button" aria-label={uiText("关闭")} autoFocus onClick={() => dialog.current?.close()}><X size={18}/></button>
    </header>
    <div ref={stage} className="mk-viewer-stage" data-actual={actual && original} tabIndex={0}>
      {src && <img src={src} alt={alt}/>}
    </div>
  </dialog>;
}
