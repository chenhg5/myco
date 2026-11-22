import { useRef, useState } from "react";
import { ArrowLeft, ArrowRight, FileText, FolderOpen, MessageCircle, Upload, X } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { importMaterial, FILE_SIZE_MB, PDF_PAGES } from "./import-material";
import { materials, materialText } from "./model";

export type MaterialLocation = { id: string; page: number };
export function locationLabel(format: string, page: number) {
  return format === "pdf" ? uiText("第 {0} 页", page) : uiText("第 {0} 段", page);
}

export function Library({ onOpen }: { onOpen: (location: MaterialLocation) => void }) {
  const picker = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  async function choose(file: File) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNote(uiText("正在读取教材…"));
    try {
      const id = await importMaterial(file, (page, total) => {
        setNote(uiText("正在读取第 {0} 页，共 {1} 页", page, total));
      });
      onOpen({ id, page: 1 });
    } catch (err) {
      setError(err instanceof Error ? err.message : uiText("教材没有保存成功。请检查连接后重试。"));
    } finally {
      setNote("");
      setBusy(false);
    }
  }
  return <section className="tu-library">
    <header className="tu-study-head"><div><h1>{uiText("教材库")}</h1><p>{uiText("选一页原文，带着它问导师。")}</p></div></header>
    <div className="tu-import">
      <span className="tu-mark"><Upload size={23}/></span>
      <div><h2>{uiText("导入教材")}</h2>
        <p>{uiText("支持 PDF、TXT 和 Markdown，最大 {0} MB，PDF 最多 {1} 页。", FILE_SIZE_MB, PDF_PAGES)}</p>
        <p>{uiText("只共享提取的文字，PDF 保留页码。扫描件需要先做文字识别。")}</p>
      </div>
      <input type="file" hidden ref={picker} aria-label={uiText("选择教材文件")} accept=".pdf,.txt,.md,.markdown" onChange={(e) => {
        const file = e.target.files?.[0];
        e.target.value = "";
        if (file) void choose(file);
      }}/>
      <button className="btn btn-primary" type="button" disabled={busy} onClick={() => picker.current?.click()}>{uiText("选择教材文件")}</button>
    </div>
    {note && <p role="status">{note}</p>}
    {error && <p role="alert" className="tu-import-error">{error}</p>}
    <div className="tu-library-list">
      {materials.rows.map(({ id, data }) => <button key={id} type="button" onClick={() => onOpen({ id, page: 1 })}>
        <FileText size={19}/><span><strong>{data.title}</strong><small>{data.format === "pdf" ? uiText("{0} 页", data.pages) : uiText("{0} 段", data.pages)}</small></span><ArrowRight size={16}/>
      </button>)}
      {materials.rows.length === 0 && <p className="faint"><FolderOpen size={18}/>{uiText("教材库是空的。导入文件后，教材会显示在这里。")}</p>}
    </div>
  </section>;
}

export function Reader({ location, onPage, onClose, onAsk }: {
  location: MaterialLocation;
  onPage: (location: MaterialLocation) => void;
  onClose: () => void;
  onAsk: (location: MaterialLocation) => void;
}) {
  const entry = materials.get(location.id)?.data;
  if (!entry) return null;
  const page = Math.max(1, Math.min(location.page, entry.pages));
  const text = materialText(location.id, page);
  return <aside className="tu-reader" aria-label={uiText("教材原文")}>
    <header><FileText size={18}/><h2>{entry.title}</h2>
      <button type="button" className="icon-btn" aria-label={uiText("关闭教材")} onClick={onClose}><X size={16}/></button>
    </header>
    <div className="tu-reader-controls">
      <button type="button" className="icon-btn" aria-label={uiText("上一页")} disabled={page <= 1} onClick={() => onPage({ id: location.id, page: page - 1 })}><ArrowLeft size={16}/></button>
      <span>{locationLabel(entry.format, page)} / {entry.pages}</span>
      <button type="button" className="icon-btn" aria-label={uiText("下一页")} disabled={page >= entry.pages} onClick={() => onPage({ id: location.id, page: page + 1 })}><ArrowRight size={16}/></button>
    </div>
    <div className="tu-reader-text" key={`${location.id}:${page}`} tabIndex={0}>
      {text || uiText("这一页没有可显示的文字。")}
    </div>
    <footer><button className="btn" type="button" disabled={!text} onClick={() => onAsk({ id: location.id, page })}><MessageCircle size={15}/>{uiText("结合这页问导师")}</button></footer>
  </aside>;
}
