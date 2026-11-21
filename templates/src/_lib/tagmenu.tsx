import { t as uiText } from "./i18n";
// Picking options (单选, 多选): a search box that also makes new ones, the
// options as coloured tags, the chosen ones ticked.
import { useMemo, useState } from "react";
import { tagOf } from "./fields";

export function Tag({ value, options, dark, onRemove }: { value: string; options: string[]; dark: boolean; onRemove?: () => void }) {
  const t = tagOf(options, value);
  return (
    <span className="tag" style={{ background: dark ? t.dbg : t.bg, color: dark ? t.dfg : t.fg }}>
      {value}
      {onRemove && <button type="button" className="tag-x" aria-label={uiText("去掉{0}", value)} onClick={(e) => { e.stopPropagation(); onRemove(); }}>×</button>}
    </span>
  );
}

export function TagMenu({ options, value, multi, dark, onChange, onDone }: {
  options: string[]; value: string[]; multi: boolean; dark: boolean;
  onChange: (values: string[]) => void; onDone: () => void;
}) {
  const [q, setQ] = useState("");
  const [at, setAt] = useState(0);
  const all = useMemo(() => [...options, ...value.filter((v) => !options.includes(v))], [options, value]);
  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    const found = all.filter((o) => !s || o.toLowerCase().includes(s));
    return q.trim() && !all.includes(q.trim()) ? [...found, q.trim()] : found;
  }, [all, q]);
  const toggle = (o: string) => {
    setQ("");
    if (!multi) { onChange(value[0] === o ? [] : [o]); return; }
    onChange(value.includes(o) ? value.filter((x) => x !== o) : [...value, o]);
  };
  return (
    <div className="tag-menu" onMouseDown={(e) => e.stopPropagation()}>
      <div className="tag-menu-head">
        {multi && value.map((v) => <Tag key={v} value={v} options={all} dark={dark} onRemove={() => toggle(v)} />)}
        <input className="tag-menu-q" autoFocus value={q} placeholder={all.length ? uiText("搜索或新建选项") : uiText("输入一个选项")}
          onChange={(e) => { setQ(e.target.value); setAt(0); }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") { e.preventDefault(); setAt((i) => Math.min(list.length - 1, i + 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setAt((i) => Math.max(0, i - 1)); }
            else if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); if (list[at] !== undefined) toggle(list[at]); else onDone(); }
            else if (e.key === "Backspace" && !q && multi && value.length) onChange(value.slice(0, -1));
            else if (e.key === "Escape") onDone();
          }} />
      </div>
      <div className="tag-menu-list" role="listbox">
        {list.map((o, i) => (
          <button key={o} type="button" role="option" aria-selected={value.includes(o)} className={`tag-menu-item${i === at ? " is-at" : ""}`}
            onMouseEnter={() => setAt(i)} onClick={() => toggle(o)}>
            {all.includes(o) ? <Tag value={o} options={all} dark={dark} /> : <span className="tag-menu-new">{uiText("新建")}{" "}<Tag value={o} options={[...all, o]} dark={dark} /></span>}
            {value.includes(o) && <span className="tag-menu-tick">✓</span>}
          </button>
        ))}
        {!list.length && <div className="tag-menu-empty">{uiText("没有选项，输入一个新建")}</div>}
      </div>
    </div>
  );
}
