import { useMemo, useState } from "react";
import { Check } from "lucide-react";
import type { Agent } from "@/api";
import { agentKind, nodeIcon, nodeName } from "@/app/nodes";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

const NAME = /^[a-z0-9][a-z0-9._-]*$/;

/**
 * NodePicker picks several node names: the picked ones as chips, and a
 * searchable list of the hub's nodes to tick. A name not in the list
 * (e.g. a node that has not joined yet) can be typed and added with Enter.
 */
export function NodePicker({ nodes, value, onChange, disabled, emptyPlaceholder, listedOnly }: {
  nodes: Agent[];
  value: string[];
  onChange: (names: string[]) => void;
  disabled?: boolean;
  /** Shown in the search box while nothing is picked. */
  emptyPlaceholder?: string;
  /** Only nodes in the list can be picked (no typed names). */
  listedOnly?: boolean;
}) {
  const [q, setQ] = useState("");
  const byName = useMemo(() => new Map(nodes.map((n) => [n.name, n])), [nodes]);
  const query = q.trim().toLowerCase();
  const list = nodes.filter((n) => !query || n.name.includes(query) || (n.display_name || "").toLowerCase().includes(query));
  const toggle = (name: string) => onChange(value.includes(name) ? value.filter((x) => x !== name) : [...value, name]);
  const add = () => {
    const first = list.find((n) => !value.includes(n.name));
    if (first) toggle(first.name);
    else if (!listedOnly && NAME.test(query) && !value.includes(query)) onChange([...value, query]);
    setQ("");
  };
  const label = (name: string) => { const n = byName.get(name); return n ? nodeName(n) : name; };

  return (
    <div className={cn("app-picker", disabled && "is-disabled")}>
      <div className="app-tag-box">
        {value.map((name) => (
          <span key={name} className="app-label editable" title={name}>
            {label(name)}
            <button type="button" aria-label={L(`去掉 ${name}`, `Remove ${name}`)} onClick={() => toggle(name)} disabled={disabled}>×</button>
          </span>
        ))}
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return;
            if (e.key === "Enter") { e.preventDefault(); add(); }
            if (e.key === "Backspace" && !q && value.length) onChange(value.slice(0, -1));
          }}
          placeholder={value.length ? (listedOnly ? L("搜索", "Search") : L("搜索或输入名字", "Search or type a name")) : emptyPlaceholder ?? L("任何人（不限制）", "Anyone (no limit)")}
          aria-label={L("搜索节点", "Search nodes")}
          disabled={disabled}
        />
      </div>
      <div className="app-picker-list" role="listbox" aria-multiselectable="true">
        {list.length === 0 && (
          <div className="app-picker-empty">
            {!listedOnly && NAME.test(query) ? L(`按回车添加「${query}」`, `Press Return to add “${query}”`) : L("没有匹配的节点", "No matching nodes")}
          </div>
        )}
        {list.map((n) => {
          const Icon = nodeIcon(n);
          const on = value.includes(n.name);
          return (
            <button
              key={n.name}
              type="button"
              role="option"
              aria-selected={on}
              className={cn("app-picker-row", on && "on")}
              onClick={() => toggle(n.name)}
              disabled={disabled}
            >
              <span className="app-picker-check">{on && <Check />}</span>
              <Icon />
              <span className="app-picker-name">{nodeName(n)}</span>
              {n.display_name && <span className="app-picker-sub mono">{n.name}</span>}
              <span className="app-picker-kind">{agentKind(n) === "human" ? L("人", "Human") : ""}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
