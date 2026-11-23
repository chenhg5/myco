import { useEffect, useRef, useState } from "react";
import { Play, Settings2 } from "lucide-react";
import { Board, icons, kindName, kinds } from "./board";
import type { XYPosition } from "@xyflow/react";
import { Kit, mount, Toast, useKit, uid } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { myco, Store } from "../_lib/myco";
import { network } from "../_lib/net";
import { fromBase64 } from "../_lib/os";
import { chain, mediaAddress, type CanvasNode, type Entry, type Kind } from "./model";
import { checkInputs, generate, type Keys } from "./provider";
import "../_lib/sheet.css";
import "./canvas.css";

const nodes = new Store<CanvasNode>("nodes");
const kit = new Kit([nodes], { quiet: true });
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

function Services({ keys, update, close }: { keys: Keys; update: (keys: Keys) => void; close: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { dialog.current?.showModal(); }, []);
  return <dialog ref={dialog} className="ac-settings" aria-labelledby="ac-service-title" onClose={close}>
    <h2 id="ac-service-title">{uiText("生成服务")}</h2>
    <p>{uiText("密钥仅在当前会话使用。生成会向所选服务发送提示词和上游素材，并可能收费。")}</p>
    {(["text", "media"] as const).map(key => {
      const name = key === "text" ? "OpenRouter" : "Replicate";
      return <div key={key}>
        <div className="ac-service-field">
          <label>{name}<input className="inp" type="password" autoComplete="off" value={keys[key]}
            onChange={event => update({ ...keys, [key]: event.target.value })}/></label>
          <button type="button" className="btn" disabled={!keys[key]} aria-label={uiText("清除 {0} 密钥", name)}
            onClick={() => update({ ...keys, [key]: "" })}>{uiText("清除")}</button>
        </div>
        <small>{key === "text" ? uiText("用于文本节点") : uiText("用于图片和视频节点")} · {keys[key].trim() ? uiText("已填写") : uiText("未填写")}</small>
      </div>;
    })}
    <form method="dialog" className="ac-controls"><button className="btn btn-primary">{uiText("完成")}</button></form>
  </dialog>;
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
  const [busy, setBusy] = useState("");
  const [services, setServices] = useState(false);
  const [progress, setProgress] = useState("");
  const [keys, setKeys] = useState<Keys>({ text: "", media: "" });
  const engine = useRef<AbortController>();
  const movements = useRef(new Map<string, Promise<XYPosition>>());
  const entries = nodes.rows.filter(node => kinds.includes(node.data.kind));
  const selectedNode = entries.find(node => node.id === selected);
  useEffect(() => () => engine.current?.abort(), []);

  async function save(id: string, data: CanvasNode, version: number) {
    try {
      chain(entries.map(node => node.id === id ? { ...node, data } : node), id);
      return await kit.track(put(id, data, version));
    } catch (error) { kit.flash(errorText(error)); return undefined; }
  }

  async function add(kind: Kind, point?: XYPosition, from = "") {
    const id = uid("node_");
    const index = entries.length;
    const result = await kit.track(put(id, {
      kind, title: kindName(kind), prompt: "", model: "", options: "", from,
      x: point?.x ?? (index % 3) * 262, y: point?.y ?? Math.floor(index / 3) * 162, output: "",
    }, 0));
    if (result) setSelected(id);
  }

  async function execute(target: Entry) {
    if (engine.current) return;
    let path: Entry[];
    try { path = chain(entries.map(node => node.id === target.id ? target : node), target.id); }
    catch (error) { kit.flash(errorText(error)); return; }
    for (const [index, entry] of path.entries()) {
      const upstream = path[index - 1];
      try {
        checkInputs(entry.data, upstream?.data.kind === "note" ? upstream.data.prompt : upstream ? "upstream output" : "", keys);
      } catch (error) {
        setSelected(entry.id);
        kit.flash((entry.data.title || kindName(entry.data.kind)) + " · " + errorText(error));
        if ((error as Error).message === "missing_key") setServices(true);
        return;
      }
    }
    const controller = new AbortController();
    engine.current = controller;
    let current = target;
    try {
      let input = "";
      const total = path.filter(entry => entry.data.kind !== "note").length;
      let step = 0;
      for (const entry of path) {
        controller.signal.throwIfAborted();
        if (entry.data.kind === "note") { input = entry.data.prompt; continue; }
        current = entry;
        setBusy(entry.id);
        setProgress(uiText("正在生成 {0} · {1}/{2}", entry.data.title || kindName(entry.data.kind), ++step, total));
        input = await generate(entry.data, input, keys, controller.signal);
        controller.signal.throwIfAborted();
        await put(entry.id, { ...entry.data, output: input }, entry.version);
      }
      setProgress(uiText("工作流已完成。"));
    } catch (error) {
      const message = controller.signal.aborted ? uiText("已停止等待生成。") :
        (current.data.title || kindName(current.data.kind)) + " · " + errorText(error);
      setProgress(message);
      setSelected(current.id);
    } finally { engine.current = undefined; setBusy(""); }
  }

  async function put(id: string, data: CanvasNode, version: number) {
    if (new TextEncoder().encode(JSON.stringify(data)).byteLength > 64 * 1024) throw new Error("too_large");
    const result = await myco!.data.put("nodes", data, { id, ifVersion: version });
    if (nodes.apply({ seq: 0, collection: "nodes", record: id, op: "put", data: result.data, version: result.version })) kit.emit();
    return { id: result.id, version: result.version, data };
  }

  function move(id: string, point: XYPosition) {
    async function commit() {
      const node = nodes.get(id);
      if (!node || (Math.abs(node.data.x - point.x) < 0.5 && Math.abs(node.data.y - point.y) < 0.5)) return point;
      const saved = await kit.track(put(id, { ...node.data, x: Math.round(point.x), y: Math.round(point.y) }, node.version));
      return saved?.data || nodes.get(id)?.data || point;
    }
    const task = (movements.current.get(id) || Promise.resolve()).then(commit, commit);
    movements.current.set(id, task);
    return task.finally(() => { if (movements.current.get(id) === task) movements.current.delete(id); });
  }

  function connect(from: string, target: string) {
    const node = nodes.get(target);
    if (!node || node.data.kind === "note" || node.data.from === from) return;
    void save(target, { ...node.data, from, output: "" }, node.version).then(saved => { if (saved) setSelected(target); });
  }

  return <div className="ac-app">
    <header className="ac-header">
      <div><h1>{uiText("AIGC 画布")}</h1>
        <small role="status">{progress || (kit.pending ? uiText("正在保存…") : kit.failed ? uiText("没保存上") : uiText("已保存"))}</small></div>
      <div className="ac-controls">
        {kinds.map(kind => { const Icon = icons[kind]; return <button type="button" className="btn" key={kind} onClick={() => void add(kind)}><Icon size={15}/>{kindName(kind)}</button>; })}
        {busy && <button type="button" className="btn" onClick={() => engine.current?.abort()}>{uiText("停止")}</button>}
        <button type="button" className="btn" onClick={() => setServices(true)}><Settings2 size={15}/>{uiText("生成服务")}</button>
      </div>
    </header>
    <div className="ac-body">
      <Board entries={entries} selected={selected} busy={busy} select={setSelected}
        add={(kind, point, from) => void add(kind, point, from)} move={move} connect={connect}/>
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
    {services && <Services keys={keys} update={setKeys} close={() => setServices(false)}/>}
    <Toast kit={kit}/>
  </div>;
}

void mount(kit, App);
