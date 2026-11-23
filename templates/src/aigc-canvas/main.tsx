import { useEffect, useRef, useState } from "react";
import { Clapperboard, FileText, Image as ImageIcon, Minus, Plus, SquarePen, Play, Link2 } from "lucide-react";
import { Kit, mount, Toast, useKit, uid } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { myco, Store } from "../_lib/myco";
import { network } from "../_lib/net";
import { fromBase64 } from "../_lib/os";
import { chain, mediaAddress, nodeHeight, nodeWidth, type CanvasNode, type Entry, type Kind } from "./model";
import { generate, type Keys } from "./provider";
import "../_lib/sheet.css";
import "./canvas.css";

const nodes = new Store<CanvasNode>("nodes");
const kit = new Kit([nodes], { quiet: true });
const kinds = ["note", "text", "image", "video"] as const;
const icons = { note: SquarePen, text: FileText, image: ImageIcon, video: Clapperboard };
const kindName = (kind: Kind) => kind === "note" ? uiText("便签") : kind === "text" ? uiText("文本") :
  kind === "image" ? uiText("图片") : uiText("视频");

function errorText(error: unknown) {
  const reason = (error as { code?: string; message?: string })?.code || (error as Error)?.message;
  switch (reason) {
    case "cycle": return uiText("连线会形成循环。");
    case "missing_input": return uiText("上游节点或输入已丢失。");
    case "missing_key": return uiText("请先在生成服务中填写 API 密钥。");
    case "provider_key": return uiText("生成服务拒绝了密钥或模型权限。");
    case "missing_model": return uiText("请填写生成服务的模型 ID。");
    case "missing_prompt": return uiText("请填写提示词。");
    case "invalid_options": return uiText("模型参数必须是有效的 JSON 对象。");
    case "empty_output": return uiText("生成服务没有返回可用结果。");
    case "provider_timeout": case "timeout": return uiText("生成服务超时，请重试。");
    case "conflict": return uiText("节点已被修改，本次结果未覆盖。");
    default: return uiText("生成未完成，请检查模型、参数或网络后重试。");
  }
}

function Media({ kind, output }: { kind: Kind; output: string }) {
  const [url, setURL] = useState("");
  const [error, setError] = useState(false);
  useEffect(() => {
    let disposed = false, objectURL = "";
    const address = mediaAddress(output);
    setURL("");
    setError(false);
    if (address && kind === "image") {
      void (async () => {
        try {
          const result = await network().fetch({ url: address, response: "base64", max_bytes: 8 * 1024 * 1024, timeout_ms: 15000 });
          const type = result.headers["content-type"]?.split(";")[0];
          if (result.status !== 200 || result.truncated || !["image/png", "image/jpeg", "image/webp", "image/gif", "image/avif"].includes(type)) throw new Error();
          if (disposed) return;
          objectURL = URL.createObjectURL(new Blob([fromBase64(result.body)], { type }));
          setURL(objectURL);
        } catch { if (!disposed) setError(true); }
      })();
    } else if (address && kind === "video" && myco?.features?.includes("net-media")) {
      try { setURL(myco.media?.url(address) || ""); } catch { setError(true); }
    }
    return () => { disposed = true; if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [output, kind]);
  const address = mediaAddress(output);
  if (!address) return null;
  return <>
    {url && (kind === "image" ? <img src={url} alt={uiText("生成图片")} onError={() => setError(true)}/> :
      <video src={url} controls playsInline preload="metadata" onError={() => setError(true)}/>)}
    {error && <p>{uiText("预览不可用，链接可能已过期。")}</p>}
    <a href={address} target="_blank" rel="noreferrer">{uiText("打开或保存原文件")}</a>
    <p>{uiText("服务商链接会过期，请及时保存需要的文件。")}</p>
  </>;
}

function Inspector({ entry, entries, busy, save, execute, remove }: {
  entry: Entry; entries: Entry[]; busy: boolean;
  save: (node: CanvasNode, version: number) => Promise<Entry | undefined>;
  execute: (entry: Entry) => void; remove: () => void;
}) {
  const [draft, setDraft] = useState(entry.data);
  const [base, setBase] = useState(entry.version);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!dirty) { setDraft(entry.data); setBase(entry.version); }
  }, [entry.data, entry.version, dirty]);
  function change(patch: Partial<CanvasNode>) { setDraft(current => ({ ...current, ...patch })); setDirty(true); }
  async function submit(run: boolean) {
    if (!dirty) { if (run) execute(entry); return; }
    setSaving(true);
    const saved = await save({ ...entry.data, title: draft.title, prompt: draft.prompt, model: draft.model, options: draft.options, from: draft.from, output: "" }, base);
    setSaving(false);
    if (!saved) return;
    setDirty(false);
    setBase(saved.version);
    if (run) execute(saved);
  }
  return <aside className="ac-inspector" aria-label={uiText("节点详情")}>
    <div className="ac-inspector-title"><h2>{kindName(draft.kind)}</h2><button type="button" className="btn" onClick={remove} disabled={busy || saving}>{uiText("删除")}</button></div>
    <label>{uiText("标题")}<input className="inp" value={draft.title} maxLength={100} onChange={event => change({ title: event.target.value })}/></label>
    <label>{draft.kind === "note" ? uiText("便签内容") : uiText("提示词")}
      <textarea className="inp" rows={5} value={draft.prompt} maxLength={4000} onChange={event => change({ prompt: event.target.value })}/>
    </label>
    {draft.kind !== "note" && <>
      <label>{uiText("上游节点")}<select className="sel" value={draft.from} onChange={event => change({ from: event.target.value })}>
        <option value="">{uiText("无")}</option>
        {entries.filter(node => node.id !== entry.id).map(node => <option value={node.id} key={node.id}>{node.data.title || kindName(node.data.kind)}</option>)}
      </select></label>
      <label>{uiText("模型 ID")}<input className="inp" value={draft.model} placeholder="owner/model" maxLength={160} onChange={event => change({ model: event.target.value })}/></label>
      {draft.kind !== "text" && <label>{uiText("模型参数（JSON）")}<textarea className="inp" rows={2} value={draft.options} placeholder={'{"image":"{{upstream}}"}'} maxLength={3000} onChange={event => change({ options: event.target.value })}/></label>}
    </>}
    <div className="ac-controls">
      <button type="button" className="btn" onClick={() => void submit(false)} disabled={!dirty || busy || saving}>{uiText("保存")}</button>
      {draft.kind !== "note" && <button type="button" className="btn btn-primary" onClick={() => void submit(true)} disabled={busy || saving}><Play size={14}/>{uiText("生成")}</button>}
      {dirty && base !== entry.version && <button type="button" className="btn" onClick={() => setDirty(false)}>{uiText("载入最新节点")}</button>}
    </div>
    {entry.data.output && <section className="ac-output">
      <h3>{uiText("生成结果")}</h3>
      {entry.data.kind === "text" ? <pre>{entry.data.output}</pre> : <Media kind={entry.data.kind} output={entry.data.output}/>}
    </section>}
  </aside>;
}

function App() {
  useKit(kit);
  const [selected, setSelected] = useState("");
  const [transform, setTransform] = useState({ x: 24, y: 24, zoom: 1 });
  const [moving, setMoving] = useState<{ id: string; x: number; y: number }>();
  const [busy, setBusy] = useState("");
  const [keys, setKeys] = useState<Keys>({ text: "", media: "" });
  const engine = useRef<AbortController>();
  const drag = useRef<{ pointer: number; id: string; x: number; y: number; sx: number; sy: number }>();
  const entries = nodes.rows.filter(node => kinds.includes(node.data.kind));
  const selectedNode = entries.find(node => node.id === selected);
  useEffect(() => () => engine.current?.abort(), []);

  async function save(id: string, data: CanvasNode, version: number) {
    try {
      chain(entries.map(node => node.id === id ? { ...node, data } : node), id);
      return await kit.track(put(id, data, version));
    } catch (error) { kit.flash(errorText(error)); return undefined; }
  }

  async function add(kind: Kind) {
    const id = uid("node_");
    const index = entries.length;
    const result = await kit.track(put(id, {
      kind, title: kindName(kind), prompt: "", model: "", options: "", from: "",
      x: (index % 3) * 262 + (24 - transform.x) / transform.zoom,
      y: Math.floor(index / 3) * 162 + (24 - transform.y) / transform.zoom, output: "",
    }, 0));
    if (result) setSelected(id);
  }

  async function execute(target: Entry) {
    if (engine.current) return;
    const controller = new AbortController();
    engine.current = controller;
    try {
      const path = chain(entries.map(node => node.id === target.id ? target : node), target.id);
      let input = "";
      for (const entry of path) {
        controller.signal.throwIfAborted();
        if (entry.data.kind === "note") { input = entry.data.prompt; continue; }
        setBusy(entry.id);
        input = await generate(entry.data, input, keys, controller.signal);
        controller.signal.throwIfAborted();
        await put(entry.id, { ...entry.data, output: input }, entry.version);
      }
    } catch (error) {
      kit.flash(controller.signal.aborted ? uiText("已停止等待生成。") : errorText(error));
    } finally { engine.current = undefined; setBusy(""); }
  }

  async function put(id: string, data: CanvasNode, version: number) {
    if (new TextEncoder().encode(JSON.stringify(data)).byteLength > 64 * 1024) throw new Error("too_large");
    const result = await myco!.data.put("nodes", data, { id, ifVersion: version });
    if (nodes.apply({ seq: 0, collection: "nodes", record: id, op: "put", data: result.data, version: result.version })) kit.emit();
    return { id: result.id, version: result.version, data };
  }

  function location(node: Entry) { return moving?.id === node.id ? moving : node.data; }
  function begin(event: React.PointerEvent<HTMLElement>, id: string, x: number, y: number) {
    if (event.button !== 0 || drag.current) return;
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { pointer: event.pointerId, id, x, y, sx: event.clientX, sy: event.clientY };
    if (id) setSelected(id);
  }
  function move(event: React.PointerEvent) {
    if (drag.current?.pointer !== event.pointerId) return;
    const { id, x, y, sx, sy } = drag.current;
    const dx = event.clientX - sx, dy = event.clientY - sy;
    if (id) setMoving({ id, x: Math.round(x + dx / transform.zoom), y: Math.round(y + dy / transform.zoom) });
    else setTransform(current => ({ ...current, x: x + dx, y: y + dy }));
  }
  function end(event: React.PointerEvent, cancel = false) {
    const previous = drag.current;
    if (previous?.pointer !== event.pointerId) return;
    drag.current = undefined;
    setMoving(undefined);
    if (!previous?.id || cancel) return;
    const node = entries.find(entry => entry.id === previous.id);
    if (!node) return;
    const x = Math.round(previous.x + (event.clientX - previous.sx) / transform.zoom);
    const y = Math.round(previous.y + (event.clientY - previous.sy) / transform.zoom);
    if (x !== node.data.x || y !== node.data.y) void kit.track(put(node.id, { ...node.data, x, y }, node.version));
  }
  function zoom(step: number) {
    setTransform(current => ({ ...current, zoom: Math.max(0.35, Math.min(2, current.zoom + step)) }));
  }

  return <div className="ac-app">
    <header className="ac-header">
      <div><h1>{uiText("AIGC 画布")}</h1>
        <small>{kit.pending ? uiText("正在保存…") : kit.failed ? uiText("没保存上") : uiText("已保存")}</small></div>
      <div className="ac-controls">
        {kinds.map(kind => { const Icon = icons[kind]; return <button type="button" className="btn" key={kind} onClick={() => void add(kind)}><Icon size={15}/>{kindName(kind)}</button>; })}
        <details className="ac-settings"><summary className="btn">{uiText("生成服务")}</summary><div>
          <label>OpenRouter<input className="inp" type="password" autoComplete="off" value={keys.text} onChange={event => setKeys({ ...keys, text: event.target.value })}/></label>
          <label>Replicate<input className="inp" type="password" autoComplete="off" value={keys.media} onChange={event => setKeys({ ...keys, media: event.target.value })}/></label>
          <p>{uiText("密钥仅在当前会话使用。生成会向所选服务发送提示词和上游素材，并可能收费。")}</p>
        </div></details>
      </div>
    </header>
    <div className="ac-body">
      <section className="ac-stage" aria-label={uiText("节点画布")}
        onPointerDown={event => begin(event, "", transform.x, transform.y)} onPointerMove={move} onPointerUp={event => end(event)}
        onPointerCancel={event => end(event, true)}>
        <div className="ac-world" style={{ transform: 'translate(' + transform.x + 'px,' + transform.y + 'px) scale(' + transform.zoom + ')' }}>
          <svg className="ac-links" aria-hidden="true">
            {entries.map(node => {
              const upstream = entries.find(item => item.id === node.data.from);
              if (!upstream) return null;
              const a = location(upstream), b = location(node);
              const ax = a.x + nodeWidth, ay = a.y + nodeHeight / 2, bx = b.x, by = b.y + nodeHeight / 2;
              const bend = Math.max(32, Math.abs(bx - ax) / 2);
              return <path key={node.id} d={['M', ax, ay, 'C', ax+bend, ay, bx-bend, by, bx, by].join(' ')}/>;
            })}
          </svg>
          {entries.map(node => {
            const Icon = icons[node.data.kind], at = location(node);
            return <button type="button" className="ac-node" key={node.id} aria-pressed={selected === node.id}
              onPointerDown={event => begin(event, node.id, at.x, at.y)} onClick={() => setSelected(node.id)}
              style={{ left: at.x, top: at.y, width: nodeWidth, height: nodeHeight }}>
              <span className="ac-node-kind"><Icon size={15}/>{kindName(node.data.kind)}{node.data.from && <Link2 size={13}/>}</span>
              <strong>{node.data.title || kindName(node.data.kind)}</strong>
              <span className="ac-node-preview">{busy === node.id ? uiText("生成中…") :
                node.data.output && (node.data.kind === "image" || node.data.kind === "video") ? uiText("生成结果") :
                node.data.output || node.data.prompt || uiText("点击编辑节点")}</span>
            </button>;
          })}
        </div>
        {!entries.length && <div className="ac-empty"><h2>{uiText("添加第一个节点")}</h2><p>{uiText("从便签开始，连接文本、图片和视频。")}</p></div>}
        <div className="ac-viewport" onPointerDown={event => event.stopPropagation()}>
          <button type="button" className="btn" aria-label={uiText("缩小")} onClick={() => zoom(-0.15)}><Minus size={15}/></button>
          <button type="button" className="btn" onClick={() => setTransform({ x: 24, y: 24, zoom: 1 })}>{Math.round(transform.zoom * 100)}%</button>
          <button type="button" className="btn" aria-label={uiText("放大")} onClick={() => zoom(0.15)}><Plus size={15}/></button>
          {busy && <button type="button" className="btn" onClick={() => engine.current?.abort()}>{uiText("停止")}</button>}
        </div>
      </section>
      {selectedNode && <Inspector key={selectedNode.id} entry={selectedNode} entries={entries} busy={Boolean(busy)}
        save={(data, version) => save(selectedNode.id, data, version)} execute={entry => void execute(entry)}
        remove={() => {
          if (entries.some(node => node.data.from === selectedNode.id)) { kit.flash(uiText("请先断开下游连线。")); return; }
          void kit.track(myco!.data.remove("nodes", selectedNode.id, { ifVersion: selectedNode.version })).then(done => {
            if (done) {
              nodes.apply({ seq: 0, collection: "nodes", record: selectedNode.id, op: "delete", version: selectedNode.version });
              setSelected("");
              kit.emit();
            }
          });
        }}/>}
    </div>
    <Toast kit={kit}/>
  </div>;
}

void mount(kit, App);
