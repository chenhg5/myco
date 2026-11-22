// Logo studio connects a brand brief to shared, inspectable vector concepts.
import { useRef, useState, type FormEvent } from "react";
import { ArrowUpRight, Download, Frame, Plus, Shapes } from "lucide-react";
import { mount, Toast, useKit } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { Canvas, Mark } from "./canvas";
import { agentMessage, canSend, createBlank, downloadLogo, saveBrief, sendBrief } from "./edit";
import { Inspector } from "./inspector";
import { briefs, concepts, kit, readLayers } from "./model";
import "../_lib/sheet.css";
import "./style.css";

function App() {
  useKit(kit);
  const last = briefs.rows.at(-1);
  const [brand, setBrand] = useState(last?.data.brand ?? "");
  const [brief, setBrief] = useState(last?.data.brief ?? "");
  const [selected, setSelected] = useState<string>();
  const [layer, setLayer] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [manual, setManual] = useState("");
  const sending = useRef(false);
  const hasResults = last && concepts.rows.some((row) => row.data.request === last.id);
  const current = (selected && concepts.get(selected)) || concepts.rows.find((row) => row.data.request === last?.id) || concepts.rows[0];
  const layers = readLayers(current?.data.layers);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (sending.current || !brand.trim() || !brief.trim()) return;
    sending.current = true;
    setBusy(true);
    setNotice("");
    setManual("");
    try {
      const saved = await saveBrief({ brand: brand.trim(), brief: brief.trim(), order: Date.now(), ...(current ? { base: current.id } : {}) });
      if (!saved) return;
      setSelected(undefined);
      setLayer(undefined);
      if (canSend) {
        try { await sendBrief(saved.id); return; }
        catch { setNotice(uiText("未能确认发送")); }
      }
      setManual(agentMessage(saved.id));
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  async function addBlank() {
    if (sending.current) return;
    sending.current = true;
    setBusy(true);
    try {
      const saved = await createBlank(brand.trim() || uiText("未命名方案"), "");
      if (saved) { setSelected(saved.id); setLayer(undefined); }
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  return <div className="logo-app">
    <header className="logo-header">
      <div className="logo-brand"><span><Frame size={18}/></span><h1>{uiText("标志工坊")}</h1></div>
      <button className="btn" type="button" disabled={!layers.some((item) => !item.guide)}
        onClick={() => void downloadLogo(current?.data.title ?? "", layers).catch(() => kit.flash(uiText("导出失败")))}>
        <Download size={16}/>{uiText("导出 SVG")}
      </button>
    </header>
    <main className="logo-workspace">
      <aside className="logo-brief" aria-label={uiText("设计需求")}>
        <h2>{uiText("设计需求")}</h2>
        <form onSubmit={(event) => void submit(event)}>
          <label>{uiText("品牌名称")}<input className="inp" required value={brand} maxLength={80} placeholder={uiText("例如：山径咖啡")}
            onChange={(event) => setBrand(event.target.value)}/></label>
          <label>{uiText("希望标志是什么样的？")}<textarea className="inp" required value={brief} rows={5} maxLength={1200}
            placeholder={uiText("说说品牌、想传达的感觉，以及标志会用在哪里。")} onChange={(event) => setBrief(event.target.value)}/></label>
          <button className="btn btn-primary logo-send" disabled={busy || !brand.trim() || !brief.trim()}>
            {canSend ? uiText("请 agent 出方案") : uiText("准备设计需求")}<ArrowUpRight size={16}/>
          </button>
        </form>
        {(busy || (last && !hasResults)) && <div className="logo-wait">
          <p role="status">{busy ? uiText("正在准备…") : notice || (canSend ? uiText("等待设计方案…") : uiText("将需求发给你的 agent。"))}</p>
          {!busy && last && <button type="button" className="btn" onClick={() => setManual(manual ? "" : agentMessage(last.id))}>{uiText("查看可复制的需求")}</button>}
        </div>}
        {manual && !hasResults && <textarea className="inp logo-manual" aria-label={uiText("给 agent 的需求")} rows={4} readOnly value={manual}
          onFocus={(event) => event.target.select()}/>}
        {current?.data.rationale && <section className="logo-rationale"><h2>{uiText("构图说明")}</h2><p>{current.data.rationale}</p></section>}
      </aside>
      <section className="logo-center" aria-label={uiText("标志画布")}>
        <div className="logo-canvas-title"><h2>{current?.data.title || uiText("标志画布")}</h2></div>
        {current ? <Canvas key={current.id} layers={layers} selected={layer} onLayer={setLayer}/> :
          <div className="logo-board logo-empty"><Shapes size={34}/><p>{uiText("提交品牌需求，或从空白方案开始。")}</p></div>}
        <div className="logo-variants-head"><h2>{uiText("设计方案")}</h2>
          <button type="button" className="btn" disabled={busy} onClick={() => void addBlank()}><Plus size={15}/>{uiText("空白方案")}</button>
        </div>
        <div className="logo-variants">
          {concepts.rows.map((row) => <button type="button" key={row.id} aria-pressed={current?.id === row.id}
            onClick={() => { setSelected(row.id); setLayer(undefined); }}>
            <span className="logo-preview"><Mark layers={readLayers(row.data.layers)}/></span>
            <span>{row.data.title}</span>
          </button>)}
        </div>
      </section>
      {current && <Inspector key={current.id} id={current.id} version={current.version} layers={layers} selected={layer} onSelect={setLayer}/>}
    </main>
    <Toast kit={kit}/>
  </div>;
}

void mount(kit, App);
