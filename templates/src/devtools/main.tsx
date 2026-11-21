import { t as uiText, locale } from "../_lib/i18n";
// 开发者工具箱: the small tools a developer reaches for every day, in one
// page, worked out here (nothing leaves the page). What someone keeps
// (收藏) is a record everyone in the page sees, agents too.
import "../_lib/sheet.css";
import "./devtools.css";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Bookmark, Copy, Search, Trash2 } from "lucide-react";
import { Store } from "../_lib/myco";
import { Kit, Toast, mount, uid, useKit } from "../_lib/appkit";
import { b64decode, b64encode, cases, color, diff, jwt, md5, sha, uuid } from "./tools";

type Saved = { tool?: string; title?: string; input?: string; extra?: string };
const saved = new Store<Saved & Record<string, unknown>>("saved");
const kit = new Kit([saved], { quiet: true });

const TOOLS: [string, string, string][] = [
  ["json", uiText("JSON 格式化"), uiText("格式化、压缩、校验，出错时指出位置")],
  ["base64", "Base64", uiText("编码、解码，支持中文和 URL 安全的写法")],
  ["url", uiText("URL 编解码"), uiText("编码、解码，拆开查询参数")],
  ["hash", uiText("哈希"), "MD5、SHA-1、SHA-256、SHA-512"],
  ["time", uiText("时间戳"), uiText("秒、毫秒和日期互转")],
  ["uuid", "UUID", uiText("批量生成 UUID v4")],
  ["jwt", uiText("JWT 解析"), uiText("看 header、payload 和过期时间")],
  ["regex", uiText("正则测试"), uiText("高亮匹配，列出分组")],
  ["diff", uiText("文本对比"), uiText("逐行比较两段文字")],
  ["case", uiText("命名转换"), "camelCase、snake_case、kebab-case…"],
  ["number", uiText("进制转换"), uiText("二、八、十、十六进制")],
  ["color", uiText("颜色转换"), "HEX、RGB、HSL"],
];

function Out({ label, value, mono = true }: { label?: string; value: string; mono?: boolean }) {
  return (
    <div className="dt-out">
      {label && <span className="dt-out-label">{label}</span>}
      <pre className={mono ? "" : "is-text"}>{value}</pre>
      <button type="button" className="icon-btn" aria-label={uiText("复制")} title={uiText("复制")} onClick={() => { void navigator.clipboard?.writeText(value).then(() => kit.flash(uiText("复制好了")), () => kit.flash(uiText("没复制上，选中文字手动复制吧"))); }}><Copy className="ico" /></button>
    </div>
  );
}
const Err = ({ e }: { e: unknown }) => <div className="dt-err">{(e as Error)?.message || String(e)}</div>;
const Area = (p: { value: string; onChange: (v: string) => void; placeholder: string; rows?: number }) => (
  <textarea className="dt-in" rows={p.rows || 8} value={p.value} placeholder={p.placeholder} spellCheck={false} onChange={(e) => p.onChange(e.target.value)} />
);
function safe<T>(f: () => T): [T | null, unknown] { try { return [f(), null]; } catch (e) { return [null, e]; } }

function Tool({ id, input, setInput, extra, setExtra }: { id: string; input: string; setInput: (v: string) => void; extra: string; setExtra: (v: string) => void }) {
  const [mode, setMode] = useState("a");
  const [hashes, setHashes] = useState<Record<string, string>>({});
  const [ids, setIds] = useState<string[]>(() => Array.from({ length: 5 }, uuid));
  useEffect(() => { setMode("a"); }, [id]);
  useEffect(() => {
    if (id !== "hash") return;
    let live = true;
    void Promise.all([sha("SHA-1", input), sha("SHA-256", input), sha("SHA-512", input)]).then(([a, b, c]) => { if (live) setHashes({ md5: md5(input), sha1: a, sha256: b, sha512: c }); });
    return () => { live = false; };
  }, [id, input]);
  const seg = (opts: [string, string][]) => <div className="seg">{opts.map(([k, n]) => <button key={k} type="button" className={mode === k ? "is-on" : ""} onClick={() => setMode(k)}>{uiText(String(n))}</button>)}</div>;
  let body: ReactNode = null;
  switch (id) {
    case "json": {
      const [v, e] = input.trim() ? safe(() => JSON.parse(input)) : [undefined, null];
      body = <>{seg([["a", "格式化"], ["b", "压缩"]])}<Area value={input} onChange={setInput} placeholder={uiText("粘贴 JSON，比如 {\"name\": \"myco\", \"ok\": true}")} />
        {e ? <Err e={e} /> : input.trim() ? <Out value={mode === "a" ? JSON.stringify(v, null, 2) : JSON.stringify(v)} /> : null}</>;
      break;
    }
    case "base64": {
      const [v, e] = input ? safe(() => (mode === "a" ? b64encode(input) : mode === "c" ? b64encode(input, true) : b64decode(input))) : ["", null];
      body = <>{seg([["a", "编码"], ["c", "编码（URL 安全）"], ["b", "解码"]])}<Area value={input} onChange={setInput} placeholder={mode === "b" ? uiText("粘贴 Base64") : uiText("要编码的文字")} />{e ? <Err e={new Error(uiText("不是有效的 Base64"))} /> : v ? <Out value={v} /> : null}</>;
      break;
    }
    case "url": {
      const [v, e] = input ? safe(() => (mode === "a" ? encodeURIComponent(input) : decodeURIComponent(input.replace(/\+/g, " ")))) : ["", null];
      const q = mode === "c" ? safe(() => [...new URL(input.includes("://") ? input : `http://x/?${input.replace(/^\?/, "")}`).searchParams]) : [null, null];
      body = <>{seg([["a", "编码"], ["b", "解码"], ["c", "拆参数"]])}<Area value={input} onChange={setInput} placeholder={mode === "c" ? uiText("粘贴一个网址或 a=1&b=2") : uiText("要处理的文字或网址")} rows={5} />
        {mode === "c" ? (q[0] ? <table className="dt-table"><tbody>{(q[0] as [string, string][]).map(([k, val], i) => <tr key={i}><td>{k}</td><td>{val}</td></tr>)}</tbody></table> : q[1] ? <Err e={q[1]} /> : null)
          : e ? <Err e={new Error(uiText("有不完整的 % 编码"))} /> : v ? <Out value={v} /> : null}</>;
      break;
    }
    case "hash":
      body = <><Area value={input} onChange={setInput} placeholder={uiText("要算哈希的文字（按 UTF-8）")} rows={5} />{Object.entries(hashes).map(([k, val]) => <Out key={k} label={k.toUpperCase().replace("SHA", "SHA-")} value={val} />)}</>;
      break;
    case "time": {
      const t = input.trim();
      const [d, e] = t ? safe(() => {
        if (/^\d{9,13}$/.test(t)) return new Date(t.length > 10 ? Number(t) : Number(t) * 1000);
        const x = new Date(t.replace(/\//g, "-"));
        if (isNaN(x.getTime())) throw new Error(uiText("认得秒、毫秒，或 2026-11-18 12:30 这样的日期"));
        return x;
      }) : [new Date(), null];
      const p2 = (n: number) => String(n).padStart(2, "0");
      const local = d ? `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}` : "";
      body = <><div className="dt-row"><input className="inp dt-wide" value={input} onChange={(e2) => setInput(e2.target.value)} placeholder={uiText("时间戳（秒或毫秒），或一个日期；空着就是现在")} />
        <button type="button" className="btn" onClick={() => setInput(String(Math.floor(Date.now() / 1000)))}>{uiText("现在")}</button></div>
        {e ? <Err e={e} /> : d && <><Out label={uiText("本地时间")} value={local} /><Out label={uiText("秒")} value={String(Math.floor(d.getTime() / 1000))} /><Out label={uiText("毫秒")} value={String(d.getTime())} /><Out label="ISO（UTC）" value={d.toISOString()} /></>}</>;
      break;
    }
    case "uuid":
      body = <><div className="dt-row"><button type="button" className="btn btn-primary" onClick={() => setIds(Array.from({ length: Number(extra) || 5 }, uuid))}>{uiText("再生成")}</button>
        <label className="dt-label">{uiText("数量")}{" "}<input className="inp" style={{ width: 70 }} value={extra || "5"} onChange={(e) => setExtra(e.target.value.replace(/\D/g, "").slice(0, 3))} /></label></div>
        <Out value={ids.join("\n")} /></>;
      break;
    case "jwt": {
      const [v, e] = input.trim() ? safe(() => jwt(input)) : [null, null];
      const exp = v && typeof v.payload?.exp === "number" ? new Date(v.payload.exp * 1000) : null;
      body = <><Area value={input} onChange={setInput} placeholder={uiText("粘贴 JWT（eyJ…）")} rows={4} />{e ? <Err e={e} /> : v && <>
        <Out label="Header" value={JSON.stringify(v.header, null, 2)} /><Out label="Payload" value={JSON.stringify(v.payload, null, 2)} />
        {exp && <div className={`dt-note${exp < new Date() ? " is-bad" : ""}`}>{exp < new Date() ? uiText("已过期") : uiText("还没过期")}：{exp.toLocaleString(locale())}</div>}
        <div className="dt-note faint">{uiText("签名没有校验（需要密钥）。")}</div></>}</>;
      break;
    }
    case "regex": {
      const [re, e] = extra ? safe(() => new RegExp(extra, mode === "a" ? "g" : "gi")) : [null, null];
      const ms = re ? [...input.matchAll(re)].slice(0, 500) : [];
      const parts: ReactNode[] = [];
      let at = 0;
      ms.forEach((m, i) => { const s = m.index || 0; if (!m[0]) return; parts.push(input.slice(at, s), <mark key={i}>{m[0]}</mark>); at = s + m[0].length; });
      parts.push(input.slice(at));
      body = <><div className="dt-row"><span className="dt-slash">/</span><input className="inp dt-wide dt-mono" value={extra} onChange={(e2) => setExtra(e2.target.value)} placeholder={uiText("正则，比如 (\\d{3})-(\\d{4})")} /><span className="dt-slash">/</span>{seg([["a", "区分大小写"], ["b", "不区分"]])}</div>
        <Area value={input} onChange={setInput} placeholder={uiText("要匹配的文字")} rows={6} />
        {e ? <Err e={e} /> : re && <><div className="dt-note">{ms.length} {" "}{uiText("处匹配")}</div><pre className="dt-hl">{parts}</pre>
          {ms.some((m) => m.length > 1) && <table className="dt-table"><tbody>{ms.slice(0, 50).map((m, i) => <tr key={i}><td>{i + 1}</td><td>{m[0]}</td>{m.slice(1).map((g, j) => <td key={j}>${j + 1}: {g ?? "—"}</td>)}</tr>)}</tbody></table>}</>}</>;
      break;
    }
    case "diff": {
      const [d, e] = input || extra ? safe(() => diff(input, extra)) : [null, null];
      body = <><div className="dt-two"><Area value={input} onChange={setInput} placeholder={uiText("原来的")} rows={10} /><Area value={extra} onChange={setExtra} placeholder={uiText("改过的")} rows={10} /></div>
        {e ? <Err e={e} /> : d && <><div className="dt-note">{uiText("删了")}{" "}{d.filter((x) => x.op === "-").length} {" "}{uiText("行，加了")}{" "}{d.filter((x) => x.op === "+").length} {" "}{uiText("行")}</div>
          <pre className="dt-diff">{d.map((x, i) => <div key={i} className={x.op === "-" ? "is-del" : x.op === "+" ? "is-add" : ""}>{x.op} {x.text}</div>)}</pre></>}</>;
      break;
    }
    case "case":
      body = <><input className="inp dt-wide" value={input} onChange={(e) => setInput(e.target.value)} placeholder={uiText("比如 user profile id、userProfileId、user_profile_id")} />
        {input.trim() && Object.entries(cases(input)).map(([k, v]) => <Out key={k} label={k} value={v} />)}</>;
      break;
    case "number": {
      const base = Number(mode === "a" ? 10 : mode === "b" ? 16 : mode === "c" ? 2 : 8), t = input.trim().replace(/^0[xbo]/i, "").replace(/[_\s]/g, "");
      const [n, e] = t ? safe(() => { if (!new RegExp(`^-?[${"0123456789abcdef".slice(0, base)}]+$`, "i").test(t)) throw new Error(uiText("不是 {0} 进制的数", base)); return BigInt(base === 10 ? t : (t.startsWith("-") ? "-" : "") + `0${base === 16 ? "x" : base === 2 ? "b" : "o"}${t.replace("-", "")}`); }) : [null, null];
      body = <><div className="dt-row">{seg([["a", "十进制"], ["b", "十六进制"], ["c", "二进制"], ["d", "八进制"]])}<input className="inp dt-wide dt-mono" value={input} onChange={(e2) => setInput(e2.target.value)} placeholder={uiText("输入一个数")} /></div>
        {e ? <Err e={e} /> : n !== null && <><Out label={uiText("十进制")} value={n.toString(10)} /><Out label={uiText("十六进制")} value={n.toString(16).toUpperCase()} /><Out label={uiText("二进制")} value={n.toString(2)} /><Out label={uiText("八进制")} value={n.toString(8)} /></>}</>;
      break;
    }
    case "color": {
      const [c, e] = input.trim() ? safe(() => color(input)) : [null, null];
      body = <><div className="dt-row"><input className="inp dt-wide dt-mono" value={input} onChange={(e2) => setInput(e2.target.value)} placeholder={uiText("#3370ff、rgb(51 112 255) 或 hsl(222 100% 60%)")} />
        <input type="color" className="dt-picker" value={c?.hex.slice(0, 7) || "#3370ff"} onChange={(e2) => setInput(e2.target.value)} aria-label={uiText("取色")} /></div>
        {e ? <Err e={e} /> : c && <><div className="dt-swatch" style={{ background: c.css }} /><Out label="HEX" value={c.hex} /><Out label="RGB" value={c.rgb} /><Out label="HSL" value={c.hsl} /></>}</>;
      break;
    }
  }
  return <div className="dt-tool">{body}</div>;
}

function App() {
  useKit(kit);
  const [id, setId] = useState("json");
  const [q, setQ] = useState("");
  const [inputs, setInputs] = useState<Record<string, { input: string; extra: string }>>({});
  const cur = inputs[id] || { input: "", extra: "" };
  const set = (k: "input" | "extra") => (v: string) => setInputs((x) => ({ ...x, [id]: { ...(x[id] || { input: "", extra: "" }), [k]: v } }));
  const list = TOOLS.filter(([k, n, d]) => !q || `${k}${n}${d}`.toLowerCase().includes(q.toLowerCase()));
  const mine = useMemo(() => saved.rows.filter((r) => r.data.tool === id), [kit.version, id]); // eslint-disable-line react-hooks/exhaustive-deps
  const tool = TOOLS.find((t) => t[0] === id)!;
  return (
    <div className="app dt">
      <aside className="dt-nav">
        <label className="dt-search"><Search className="ico" /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder={uiText("找工具")} aria-label={uiText("找工具")} /></label>
        {list.map(([k, n]) => <button key={k} type="button" className={`dt-item${id === k ? " is-on" : ""}`} onClick={() => setId(k)}>{n}{saved.rows.some((r) => r.data.tool === k) && <i className="dt-has" />}</button>)}
      </aside>
      <main className="dt-main">
        <header className="dt-head">
          <div><h1>{tool[1]}</h1><p className="faint">{tool[2]}</p></div>
          <button type="button" className="btn" disabled={!cur.input && !cur.extra} onClick={() => {
            const t = window.prompt(uiText("收藏起来，大家都能看到。起个名字："), cur.input.slice(0, 20));
            if (t === null) return;
            kit.track(saved.put({ tool: id, title: t.trim() || tool[1], input: cur.input, extra: cur.extra }, uid("s")));
            kit.emit();
          }}><Bookmark className="ico" />{uiText("收藏")}</button>
        </header>
        <Tool id={id} input={cur.input} setInput={set("input")} extra={cur.extra} setExtra={set("extra")} />
        {mine.length > 0 && (
          <section className="dt-saved">
            <h3>{uiText("收藏的")}</h3>
            {mine.map((r) => (
              <div key={r.id} className="dt-saved-row">
                <button type="button" className="dt-saved-open" onClick={() => setInputs((x) => ({ ...x, [id]: { input: String(r.data.input || ""), extra: String(r.data.extra || "") } }))}>
                  <b>{r.data.title}</b><span className="faint">{String(r.data.input || r.data.extra || "").slice(0, 60)}</span>
                </button>
                {r.updated_by && <span className="faint dt-by">{r.updated_by}</span>}
                <button type="button" className="icon-btn" aria-label={uiText("删除")} onClick={() => { kit.track(saved.remove(r.id)); kit.emit(); }}><Trash2 className="ico" /></button>
              </div>
            ))}
          </section>
        )}
      </main>
      <Toast kit={kit} />
    </div>
  );
}

void mount(kit, App);
