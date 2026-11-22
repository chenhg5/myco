import { useState } from "react";
import { ChevronDown } from "lucide-react";
import type { MCPServer, ToolPlugin, ToolSkill } from "@/api";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export type ToolLists = {
  mcp?: MCPServer[];
  skills?: ToolSkill[];
  plugins?: ToolPlugin[];
  tool_warnings?: string[];
};

export function hasTools(x: ToolLists) {
  return Boolean(x.mcp?.length || x.skills?.length || x.plugins?.length || x.tool_warnings?.length);
}

const scope = (s: string) => s === "project" ? L("项目级", "Project") : s === "plugin" ? L("插件", "Plugin") : L("用户级", "User");

/** How many rows a group shows before "show N more". */
const FIRST = 5;

/*
 * The tools an agent's CLI has (MCP servers, skills, plugins), as a list
 * people can scan: a group per kind with its count, one row per tool (its
 * name, then where it comes from, a skill's description cut at two lines),
 * a long group folded after the first few. Only names and public metadata
 * appear here: paths, URL queries and secrets never reach the App.
 */
export function ToolInventory({ tools, mcpBlocked = false }: { tools: ToolLists; mcpBlocked?: boolean; showSummary?: boolean }) {
  if (!hasTools(tools)) return null;
  // An agent's list joins its own with its computer's: each tool once
  // (the one that says more about itself).
  const mcp = once(tools.mcp || [], (m) => m.name, () => 0);
  const skills = once(tools.skills || [], (x) => x.name, (x) => (x.description || "").length);
  const plugins = once(tools.plugins || [], (p) => p.name, () => 0);
  return (
    <div className="ti" data-testid="tool-inventory">
      {mcp.length > 0 && (
        <Group title="MCP" count={mcp.length} kind="mcp"
          note={mcpBlocked ? L("在工作区权限下用不了：到「设置」里打开「允许 MCP 工具」。", "Not available with workspace access: turn on Allow MCP tools in Settings.") : undefined}
          rows={mcp.map((m, i) => (
            <Row key={`m-${i}`} name={m.name}
              meta={[m.transport === "stdio" ? L("本地", "Local") : L(`远程${m.host ? ` · ${m.host}` : ""}`, `Remote${m.host ? ` · ${m.host}` : ""}`), scope(m.scope)].join(" · ")}
              tag={m.enabled === false ? L("未启用", "Disabled") : undefined} off={m.enabled === false} />
          ))} />
      )}
      {skills.length > 0 && (
        <Group title={L("技能", "Skills")} count={skills.length} kind="skills"
          rows={skills.map((s, i) => (
            <Row key={`s-${i}`} name={s.name} desc={s.description}
              meta={[scope(s.scope), s.managed_by === "myco" ? L("myco 装的", "Installed by myco") : ""].filter(Boolean).join(" · ")} />
          ))} />
      )}
      {plugins.length > 0 && (
        <Group title={L("插件", "Plugins")} count={plugins.length} kind="plugins"
          rows={plugins.map((p, i) => (
            <Row key={`p-${i}`} name={p.name} meta={p.marketplace || ""}
              tag={p.enabled ? undefined : L("未启用", "Disabled")} off={!p.enabled} />
          ))} />
      )}
      {!!tools.tool_warnings?.length && (
        <div className="ti-warn" title={tools.tool_warnings.join("\n")}>
          {L(`有 ${tools.tool_warnings.length} 个配置文件没读懂，里面的工具没列出来。`, `${tools.tool_warnings.length} configuration files could not be read; their tools are not listed.`)}
        </div>
      )}
    </div>
  );
}

function once<T>(list: T[], key: (x: T) => string, worth: (x: T) => number): T[] {
  const best = new Map<string, T>();
  for (const x of list) {
    const k = key(x);
    const had = best.get(k);
    if (!had || worth(x) > worth(had)) best.set(k, x);
  }
  return list.filter((x) => best.get(key(x)) === x);
}

function Group({ title, count, kind, rows, note }: { title: string; count: number; kind: string; rows: React.ReactNode[]; note?: string }) {
  const [all, setAll] = useState(false);
  const more = rows.length - FIRST;
  return (
    <div className="ti-group" data-kind={kind}>
      <div className="ti-head"><span>{title}</span><small>{count}</small></div>
      {note && <div className="ti-note">{note}</div>}
      {all || more <= 1 ? rows : rows.slice(0, FIRST)}
      {more > 1 && (
        <button type="button" className={cn("ti-more", all && "is-open")} onClick={() => setAll((v) => !v)} aria-expanded={all}>
          {all ? L("收起", "Show fewer") : L(`再显示 ${more} 个`, `Show ${more} more`)}<ChevronDown />
        </button>
      )}
    </div>
  );
}

function Row({ name, meta, desc, tag, off }: { name: string; meta?: string; desc?: string; tag?: string; off?: boolean }) {
  return (
    <div className={cn("ti-row", off && "is-off")}>
      <div className="ti-line">
        <span className="ti-name" title={name}>{name}</span>
        {tag ? <span className="ti-tag">{tag}</span> : meta && !desc ? <span className="ti-meta">{meta}</span> : null}
      </div>
      {desc && <div className="ti-desc" title={desc}>{desc}</div>}
      {desc && meta && <div className="ti-meta ti-meta-below">{meta}</div>}
    </div>
  );
}
