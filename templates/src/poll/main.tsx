import { t as uiText } from "../_lib/i18n";
// 投票: one question, its options, everyone's vote. Each voter's choice is
// one record (its id from the voter's name), so voting again changes it.
// Results update live; the one who set it up can edit, close or reopen it.
import "../_lib/sheet.css";
import "./poll.css";
import { useMemo, useState } from "react";
import { Check, Lock, LockOpen, Pencil, Plus, X } from "lucide-react";
import { Store } from "../_lib/myco";
import { Kit, Toast, mount, useKit } from "../_lib/appkit";
import { personColor } from "../_lib/cells";

type Poll = { question?: string; options?: string[]; multi?: boolean; anonymous?: boolean; closed?: boolean; owner?: string; note?: string };
type Vote = { voter?: string; choices?: number[] };
const poll = new Store<Poll & Record<string, unknown>>("poll");
const votes = new Store<Vote & Record<string, unknown>>("votes");
const kit = new Kit([poll, votes], { quiet: true });

const keyOf = (name: string) => { let h = 5381; for (const c of name) h = ((h * 33) ^ c.charCodeAt(0)) >>> 0; return `v-${h.toString(36)}`; };

function App() {
  useKit(kit);
  const p = poll.get("main")?.data || {};
  const options = p.options || [];
  const [editing, setEditing] = useState(!p.question);
  const mineId = keyOf(kit.me);
  const mine = votes.get(mineId)?.data.choices || [];
  const tally = useMemo(() => {
    const n = options.map(() => [] as string[]);
    for (const r of votes.rows) for (const c of r.data.choices || []) if (n[c]) n[c].push(String(r.data.voter || r.updated_by || ""));
    return n;
  }, [kit.version, options.length]); // eslint-disable-line react-hooks/exhaustive-deps
  const voters = votes.rows.filter((r) => (r.data.choices || []).length).length;
  const max = Math.max(1, ...tally.map((t) => t.length));
  const canEdit = !p.owner || p.owner === kit.me;
  const setPoll = (patch: Poll) => { kit.track(poll.put({ ...(poll.get("main")?.data || {}), ...patch }, "main")); kit.emit(); };
  const vote = (i: number) => {
    if (p.closed || !kit.me) return;
    const next = p.multi ? (mine.includes(i) ? mine.filter((x) => x !== i) : [...mine, i].sort()) : mine[0] === i ? [] : [i];
    kit.track(votes.put({ voter: kit.me, choices: next }, mineId));
    kit.emit();
  };
  if (editing && canEdit) return <Editor p={p} onDone={(np) => { setPoll({ ...np, owner: p.owner || kit.me, closed: false }); setEditing(false); }} onCancel={p.question ? () => setEditing(false) : undefined} />;
  return (
    <div className="app pl">
      <div className="pl-main">
        <div className="pl-card">
          <div className="pl-head">
            <span className={`pl-badge${p.closed ? " is-closed" : ""}`}>{p.closed ? uiText("已结束") : p.multi ? uiText("多选") : uiText("单选")}{p.anonymous ? uiText(" · 匿名") : ""}</span>
            <span className="top-gap" />
            {canEdit && <>
              <button type="button" className="tool" onClick={() => setEditing(true)}><Pencil className="ico" />{uiText("编辑")}</button>
              <button type="button" className="tool" onClick={() => setPoll({ closed: !p.closed })}>{p.closed ? <><LockOpen className="ico" />{uiText("重新开始")}</> : <><Lock className="ico" />{uiText("结束投票")}</>}</button>
            </>}
          </div>
          <h1 className="pl-q">{p.question || uiText("（还没有问题）")}</h1>
          {p.note && <p className="pl-note">{p.note}</p>}
          <ul className="pl-opts">
            {options.map((o, i) => {
              const n = tally[i]?.length || 0, pct = voters ? Math.round((n / voters) * 100) : 0, on = mine.includes(i), lead = n === max && n > 0;
              return (
                <li key={i}>
                  <button type="button" className={`pl-opt${on ? " is-on" : ""}${lead ? " is-lead" : ""}`} disabled={p.closed} onClick={() => vote(i)} aria-pressed={on}>
                    <span className="pl-bar" style={{ width: `${(n / max) * 100}%` }} />
                    <span className={`pl-mark${p.multi ? " is-box" : ""}`}>{on && <Check className="ico" />}</span>
                    <span className="pl-text">{o}</span>
                    <span className="pl-num">{n} {" "}{uiText("票 ·")}{" "}{pct}%</span>
                  </button>
                  {!p.anonymous && n > 0 && (
                    <div className="pl-who">{tally[i].map((v) => <span key={v} className="person-dot" title={v} style={{ background: personColor(v) }}>{v.slice(0, 1).toUpperCase()}</span>)}</div>
                  )}
                </li>
              );
            })}
          </ul>
          <div className="pl-foot">
            <span>{voters} {" "}{uiText("人投了")}{mine.length ? uiText("，包括你") : kit.me && !p.closed ? uiText("，点一项投票") : ""}</span>
            {p.owner && <span className="faint">{p.owner} {" "}{uiText("发起")}</span>}
          </div>
        </div>
      </div>
      <Toast kit={kit} />
    </div>
  );
}

function Editor({ p, onDone, onCancel }: { p: Poll; onDone: (p: Poll) => void; onCancel?: () => void }) {
  const [q, setQ] = useState(p.question || "");
  const [note, setNote] = useState(p.note || "");
  const [opts, setOpts] = useState<string[]>(p.options?.length ? p.options : ["", ""]);
  const [multi, setMulti] = useState(!!p.multi);
  const [anon, setAnon] = useState(!!p.anonymous);
  const ok = q.trim() && opts.filter((o) => o.trim()).length >= 2;
  return (
    <div className="app pl">
      <div className="pl-main">
        <form className="pl-card pl-edit" onSubmit={(e) => { e.preventDefault(); if (ok) onDone({ question: q.trim(), note: note.trim(), options: opts.map((o) => o.trim()).filter(Boolean), multi, anonymous: anon }); }}>
          <h2>{p.question ? uiText("编辑投票") : uiText("发起一个投票")}</h2>
          <input className="fi pl-q-in" value={q} onChange={(e) => setQ(e.target.value)} placeholder={uiText("问题，比如：团建去哪？")} autoFocus aria-label={uiText("问题")} />
          <input className="fi" value={note} onChange={(e) => setNote(e.target.value)} placeholder={uiText("补充说明（可不填）")} aria-label={uiText("补充说明")} />
          <div className="pl-opt-edit">
            {opts.map((o, i) => (
              <div key={i} className="pl-opt-row">
                <span className="faint">{i + 1}</span>
                <input className="fi" value={o} onChange={(e) => setOpts(opts.map((x, j) => (j === i ? e.target.value : x)))} placeholder={uiText("选项 {0}", i + 1)} aria-label={uiText("选项 {0}", i + 1)}
                  onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing && i === opts.length - 1 && o.trim()) { e.preventDefault(); setOpts([...opts, ""]); } }} />
                {opts.length > 2 && <button type="button" className="icon-btn" aria-label={uiText("去掉这一项")} onClick={() => setOpts(opts.filter((_, j) => j !== i))}><X className="ico" /></button>}
              </div>
            ))}
            <button type="button" className="link-btn" onClick={() => setOpts([...opts, ""])}><Plus className="ico" />{uiText("加一个选项")}</button>
          </div>
          <label className="pl-switch"><input type="checkbox" checked={multi} onChange={(e) => setMulti(e.target.checked)} />{uiText("可以选多项")}</label>
          <label className="pl-switch"><input type="checkbox" checked={anon} onChange={(e) => setAnon(e.target.checked)} />{uiText("不显示谁投了什么")}</label>
          <div className="pl-edit-foot">
            {onCancel && <button type="button" className="btn" onClick={onCancel}>{uiText("取消")}</button>}
            <button type="submit" className="btn btn-primary" disabled={!ok}>{p.question ? uiText("保存") : uiText("发起投票")}</button>
          </div>
          {p.question && <p className="faint pl-warn">{uiText("改了选项的顺序或删了选项，已经投的票会对不上，最好只改文字或在后面加。")}</p>}
        </form>
      </div>
    </div>
  );
}

void mount(kit, App);
