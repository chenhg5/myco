// 代码编辑器: a small project people and agents edit together, in Monaco
// (VS Code's editor): a file tree, tabs, syntax colours for many languages,
// find and replace, multiple cursors, the minimap. Each file is a record
// (its path and its text); HTML and Markdown files preview beside it.
import "../_lib/sheet.css";
import "./code.css";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api";
import "monaco-editor/esm/vs/editor/editor.all";
import "monaco-editor/esm/vs/basic-languages/monaco.contribution";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import { marked } from "marked";
import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight, Download, Eye, EyeOff, File, FilePlus, Folder, FolderPlus, Pencil, Trash2, X } from "lucide-react";
import { Store, save } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";
import { zip } from "../_lib/export";

(self as unknown as { MonacoEnvironment: unknown }).MonacoEnvironment = { getWorker: () => new EditorWorker() };

type FileRec = { path?: string; content?: string };
const project = new Store<{ title?: string }>("project");
const files = new Store<FileRec & Record<string, unknown>>("files");
const kit = new Kit([project, files], { quiet: true });
const title = titleSaver(kit, project);

const LANG: Record<string, string> = {
  js: "javascript", mjs: "javascript", cjs: "javascript", jsx: "javascript", ts: "typescript", tsx: "typescript", json: "json", html: "html", htm: "html",
  css: "css", scss: "scss", less: "less", md: "markdown", py: "python", go: "go", rs: "rust", java: "java", kt: "kotlin", swift: "swift", c: "c", h: "c",
  cpp: "cpp", cc: "cpp", hpp: "cpp", cs: "csharp", rb: "ruby", php: "php", sh: "shell", bash: "shell", zsh: "shell", yml: "yaml", yaml: "yaml",
  toml: "ini", ini: "ini", sql: "sql", xml: "xml", svg: "xml", vue: "html", lua: "lua", r: "r", dart: "dart", dockerfile: "dockerfile", graphql: "graphql",
};
const langOf = (p: string) => { const n = p.split("/").pop()!.toLowerCase(); if (n === "dockerfile") return "dockerfile"; if (n === "makefile") return "shell"; return LANG[n.split(".").pop() || ""] || "plaintext"; };
const byPath = () => new Map(files.rows.map((r) => [String(r.data.path || ""), r]));
const MAX = 60000;

type Tree = { name: string; path: string; dirs: Tree[]; files: { name: string; path: string; id: string }[] };
function treeOf(): Tree {
  const root: Tree = { name: "", path: "", dirs: [], files: [] };
  const dirFor = (parts: string[]) => {
    let t = root;
    parts.forEach((p, i) => { let d = t.dirs.find((x) => x.name === p); if (!d) { d = { name: p, path: parts.slice(0, i + 1).join("/"), dirs: [], files: [] }; t.dirs.push(d); } t = d; });
    return t;
  };
  for (const r of files.rows) {
    const path = String(r.data.path || "").replace(/^\/+/, "");
    if (!path) continue;
    const parts = path.split("/"), name = parts.pop()!;
    if (name === ".keep") { dirFor(parts); continue; }
    dirFor(parts).files.push({ name, path, id: r.id });
  }
  const sort = (t: Tree) => { t.dirs.sort((a, b) => a.name.localeCompare(b.name)); t.files.sort((a, b) => a.name.localeCompare(b.name)); t.dirs.forEach(sort); };
  sort(root);
  return root;
}

function useDark() {
  const q = window.matchMedia?.("(prefers-color-scheme: dark)");
  const [d, setD] = useState(!!q?.matches);
  useEffect(() => { if (!q) return; const f = () => setD(q.matches); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return d;
}

/** The page a preview shows: an HTML file with its local CSS and JS put in. */
function previewOf(path: string): string {
  const map = byPath(), r = map.get(path);
  const src = String(r?.data.content || "");
  if (langOf(path) === "markdown") {
    return `<!doctype html><meta charset="utf-8"><style>body{font:15px/1.7 -apple-system,"PingFang SC",sans-serif;max-width:760px;margin:24px auto;padding:0 20px;color:#1f2329}pre{background:#f5f6f7;padding:12px;border-radius:6px;overflow:auto}code{font-family:ui-monospace,Menlo,monospace}</style>${marked.parse(src)}`;
  }
  const dir = path.includes("/") ? path.slice(0, path.lastIndexOf("/") + 1) : "";
  const resolve = (ref: string) => { if (/^(https?:)?\/\//.test(ref)) return null; const parts = (dir + ref).split("/"); const out: string[] = []; for (const p of parts) { if (p === "..") out.pop(); else if (p && p !== ".") out.push(p); } return map.get(out.join("/")); };
  return src
    .replace(/<link[^>]+href=["']([^"']+\.css)["'][^>]*>/gi, (m, ref) => { const f = resolve(ref); return f ? `<style>${f.data.content || ""}</style>` : m; })
    .replace(/<script([^>]*)src=["']([^"']+\.m?js)["']([^>]*)><\/script>/gi, (m, a, ref, b) => { const f = resolve(ref); return f ? `<script${a}${b}>${String(f.data.content || "").replace(/<\/script/gi, "<\\/script")}</script>` : m; });
}

function App() {
  useKit(kit);
  const dark = useDark();
  const [tabs, setTabs] = useState<string[]>([]);
  const [active, setActive] = useState<string>("");
  const [open, setOpen] = useState<Set<string>>(new Set([""]));
  const [preview, setPreview] = useState(false);
  const [pos, setPos] = useState({ line: 1, col: 1 });
  const [dirty, setDirty] = useState<Set<string>>(new Set());
  const [, setTick] = useState(0);
  const host = useRef<HTMLDivElement>(null);
  const ed = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const models = useRef(new Map<string, monaco.editor.ITextModel>());
  const timers = useRef(new Map<string, number>());
  const applying = useRef(false);
  const tree = useMemo(treeOf, [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps

  // the editor
  useEffect(() => {
    ed.current = monaco.editor.create(host.current!, {
      automaticLayout: true, fontSize: 13.5, fontFamily: `ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace`,
      minimap: { enabled: true }, scrollBeyondLastLine: false, tabSize: 2, renderWhitespace: "selection", smoothScrolling: true,
      bracketPairColorization: { enabled: true }, guides: { bracketPairs: true }, padding: { top: 8 }, model: null,
    });
    ed.current.onDidChangeCursorPosition((e) => setPos({ line: e.position.lineNumber, col: e.position.column }));
    ed.current.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => { const p = pathOfModel(ed.current?.getModel()); if (p) flush(p); });
    return () => { ed.current?.dispose(); for (const m of models.current.values()) m.dispose(); };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { monaco.editor.setTheme(dark ? "vs-dark" : "vs"); }, [dark]);

  const pathOfModel = (m: monaco.editor.ITextModel | null | undefined) => { for (const [p, x] of models.current) if (x === m) return p; return ""; };
  const flush = (path: string) => {
    window.clearTimeout(timers.current.get(path));
    const m = models.current.get(path), r = byPath().get(path);
    if (!m || !r) return;
    const text = m.getValue();
    if (text.length > MAX) { kit.flash(`「${path}」超过 ${MAX / 1000}KB，没保存：拆成几个文件吧`); return; }
    if (text !== r.data.content) kit.track(files.put({ ...r.data, content: text }, r.id));
    setDirty((d) => { const n = new Set(d); n.delete(path); return n; });
  };
  const modelFor = (path: string) => {
    let m = models.current.get(path);
    if (!m) {
      m = monaco.editor.createModel(String(byPath().get(path)?.data.content || ""), langOf(path));
      m.onDidChangeContent(() => {
        if (applying.current) return;
        setDirty((d) => new Set(d).add(path));
        window.clearTimeout(timers.current.get(path));
        timers.current.set(path, window.setTimeout(() => flush(path), 800));
        setTick((t) => t + 1);
      });
      models.current.set(path, m);
    }
    return m;
  };
  const show = (path: string) => {
    if (!path) return;
    const m = modelFor(path);
    if (ed.current?.getModel() !== m) ed.current?.setModel(m);
    setActive(path);
    setTabs((t) => (t.includes(path) ? t : [...t, path]));
    const parts = path.split("/");
    setOpen((o) => { const n = new Set(o); for (let i = 1; i < parts.length; i++) n.add(parts.slice(0, i).join("/")); return n; });
    if (!/\.(html?|md)$/i.test(path)) setPreview(false);
  };
  // open the first file once there are files
  useEffect(() => {
    if (active || !files.rows.length) return;
    const first = ["index.html", "README.md", "main.py", "main.go", "src/main.ts", "src/index.js"].find((p) => byPath().has(p)) || String(files.rows[0].data.path || "");
    show(first);
  }, [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps
  // what others write lands in the open models (not in one being typed in)
  useEffect(() => {
    const map = byPath();
    for (const [p, m] of models.current) {
      const r = map.get(p);
      if (!r) continue;
      const remote = String(r.data.content || "");
      if (dirty.has(p) || remote === m.getValue()) continue;
      const viewState = ed.current?.getModel() === m ? ed.current?.saveViewState() : null;
      applying.current = true;
      m.pushEditOperations([], [{ range: m.getFullModelRange(), text: remote }], () => null);
      applying.current = false;
      if (viewState) ed.current?.restoreViewState(viewState);
    }
    for (const t of tabs) if (!map.has(t)) closeTab(t);
  }, [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps

  const closeTab = (p: string) => {
    if (dirty.has(p)) flush(p);
    setTabs((t) => {
      const n = t.filter((x) => x !== p);
      if (active === p) { const next = n[n.length - 1] || ""; if (next) setTimeout(() => show(next)); else { ed.current?.setModel(null); setActive(""); } }
      return n;
    });
  };
  const newFile = (dir = "") => {
    const name = window.prompt("新文件名（可以带路径，比如 src/app.js）", dir ? `${dir}/` : "");
    const path = name?.trim().replace(/^\/+/, "");
    if (!path) return;
    if (byPath().has(path)) { show(path); return; }
    const keep = byPath().get(`${path.split("/").slice(0, -1).join("/")}/.keep`);
    if (keep) kit.track(files.remove(keep.id));
    kit.track(files.put({ path, content: "" }, uid("f")));
    kit.emit();
    setTimeout(() => show(path), 0);
  };
  const newFolder = (dir = "") => {
    const name = window.prompt("新文件夹名", dir ? `${dir}/` : "");
    const path = name?.trim().replace(/^\/+|\/+$/g, "");
    if (!path) return;
    kit.track(files.put({ path: `${path}/.keep`, content: "" }, uid("f")));
    setOpen((o) => new Set(o).add(path));
    kit.emit();
  };
  const rename = (path: string) => {
    const to = window.prompt("改名为", path)?.trim().replace(/^\/+/, "");
    if (!to || to === path) return;
    if (byPath().has(to)) { kit.flash(`已经有「${to}」了`); return; }
    const r = byPath().get(path);
    if (!r) return;
    flush(path);
    kit.track(files.put({ ...r.data, path: to }, r.id));
    models.current.get(path)?.dispose();
    models.current.delete(path);
    setTabs((t) => t.map((x) => (x === path ? to : x)));
    kit.emit();
    setTimeout(() => show(to), 0);
  };
  const remove = (path: string, isDir = false) => {
    const gone = files.rows.filter((r) => { const p = String(r.data.path || ""); return isDir ? p.startsWith(`${path}/`) : p === path; });
    if (!gone.length || !window.confirm(isDir ? `删除文件夹「${path}」和里面的 ${gone.length} 个文件？` : `删除「${path}」？`)) return;
    const keep = gone.map((r) => ({ id: r.id, data: r.data }));
    for (const g of keep) { kit.track(files.remove(g.id)); const p = String(g.data.path); if (tabs.includes(p)) closeTab(p); }
    kit.flash(`删除了 ${keep.length} 个文件`, () => { for (const g of keep) kit.track(files.put(g.data, g.id)); kit.note = null; kit.emit(); });
  };
  const exportZip = async () => {
    for (const p of dirty) flush(p);
    const out: Record<string, string> = {};
    for (const r of files.rows) { const p = String(r.data.path || ""); if (p && !p.endsWith("/.keep")) out[p] = String(r.data.content || ""); }
    try { await save(`${(title.get() || "project").replace(/[\\/:*?"<>|]/g, " ")}.zip`, zip(out), "application/zip"); kit.flash("已存到「下载」"); }
    catch (e) { kit.flash((e as Error).message || "没存下来"); }
  };

  const canPreview = /\.(html?|md)$/i.test(active);
  const branch = (t: Tree, depth: number): JSX.Element[] => [
    ...t.dirs.map((d) => (
      <div key={`d:${d.path}`}>
        <div className="cd-node is-dir" style={{ paddingLeft: 8 + depth * 14 }} onClick={() => setOpen((o) => { const n = new Set(o); if (n.has(d.path)) n.delete(d.path); else n.add(d.path); return n; })}>
          {open.has(d.path) ? <ChevronDown className="ico" /> : <ChevronRight className="ico" />}<Folder className="ico cd-ic" /><span>{d.name}</span>
          <span className="cd-acts" onClick={(e) => e.stopPropagation()}>
            <button type="button" title="新文件" onClick={() => newFile(d.path)}><FilePlus className="ico" /></button>
            <button type="button" title="删除文件夹" onClick={() => remove(d.path, true)}><Trash2 className="ico" /></button>
          </span>
        </div>
        {open.has(d.path) && branch(d, depth + 1)}
      </div>
    )),
    ...t.files.map((f) => (
      <div key={f.id} className={`cd-node${active === f.path ? " is-on" : ""}`} style={{ paddingLeft: 22 + depth * 14 }} onClick={() => show(f.path)} title={f.path}>
        <File className="ico cd-ic" /><span>{f.name}</span>{dirty.has(f.path) && <i className="cd-dot" />}
        <span className="cd-acts" onClick={(e) => e.stopPropagation()}>
          <button type="button" title="改名" onClick={() => rename(f.path)}><Pencil className="ico" /></button>
          <button type="button" title="删除" onClick={() => remove(f.path)}><Trash2 className="ico" /></button>
        </span>
      </div>
    )),
  ];
  return (
    <div className={`app cd${dark ? " is-dark" : ""}`}>
      <Bar kit={kit} title={title.get()} placeholder="未命名项目" onTitle={title.set}>
        {canPreview && <button type="button" className={`btn${preview ? " is-on" : ""}`} onClick={() => { for (const p of dirty) flush(p); setPreview(!preview); }}>{preview ? <EyeOff className="ico" /> : <Eye className="ico" />}预览</button>}
        <button type="button" className="btn" onClick={() => void exportZip()}><Download className="ico" />下载 zip</button>
      </Bar>
      <div className="cd-main">
        <aside className="cd-tree">
          <div className="cd-tree-head"><span>文件</span>
            <button type="button" className="icon-btn" title="新文件" aria-label="新文件" onClick={() => newFile()}><FilePlus className="ico" /></button>
            <button type="button" className="icon-btn" title="新文件夹" aria-label="新文件夹" onClick={() => newFolder()}><FolderPlus className="ico" /></button>
          </div>
          <div className="cd-tree-body">{branch(tree, 0)}{!files.rows.length && <p className="cd-empty">还没有文件。点上面的 ＋ 新建，或者让 agent 写：「在代码编辑器里写一个贪吃蛇的 index.html」。</p>}</div>
        </aside>
        <section className="cd-edit">
          <div className="cd-tabs">
            {tabs.map((t) => (
              <div key={t} className={`cd-tab${t === active ? " is-on" : ""}`} onClick={() => show(t)} title={t}>
                <span>{t.split("/").pop()}</span>
                <button type="button" aria-label={`关掉 ${t}`} onClick={(e) => { e.stopPropagation(); closeTab(t); }}>{dirty.has(t) ? <i className="cd-dot" /> : <X className="ico" />}</button>
              </div>
            ))}
          </div>
          <div className="cd-panes">
            <div className="cd-monaco" ref={host} style={{ visibility: active ? "visible" : "hidden" }} />
            {!active && <div className="cd-blank">从左边打开一个文件</div>}
            {preview && canPreview && <iframe className="cd-preview" title="预览" sandbox="allow-scripts" srcDoc={previewOf(active)} />}
          </div>
          <div className="cd-status">
            <span>{active || "—"}</span><span className="top-gap" />
            {active && <><span>行 {pos.line}，列 {pos.col}</span><span>{langOf(active)}</span><span>{dirty.has(active) ? "正在保存…" : kit.pending ? "正在保存…" : "已保存"}</span></>}
          </div>
        </section>
      </div>
      <Toast kit={kit} />
    </div>
  );
}

void mount(kit, App, async () => {
  if (!project.get("main")) await project.put({ title: "" }, "main")?.catch(() => project.load());
});
