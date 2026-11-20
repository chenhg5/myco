import type { MCPServer, ToolPlugin, ToolSkill } from "@/api";
import { L } from "@/lib/i18n";

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

/** Only names and public metadata appear here. Paths, URL queries and secrets never reach the App. */
export function ToolInventory({ tools, mcpBlocked = false, showSummary = true }: { tools: ToolLists; mcpBlocked?: boolean; showSummary?: boolean }) {
  if (!hasTools(tools)) return null;
  return (
    <div className="app-tool-inventory" data-testid="tool-inventory">
      {showSummary && <div className="app-small-note">{L(`MCP ${tools.mcp?.length || 0} · 技能 ${tools.skills?.length || 0} · 插件 ${tools.plugins?.length || 0}`, `MCP ${tools.mcp?.length || 0} · Skills ${tools.skills?.length || 0} · Plugins ${tools.plugins?.length || 0}`)}</div>}
      {tools.mcp?.map((m, i) => <div key={`m-${i}`} className="app-small-note">
        MCP · {m.name} · {m.transport === "stdio" ? L("本地", "Local") : L(`远程${m.host ? `（${m.host}）` : ""}`, `Remote${m.host ? ` (${m.host})` : ""}`)} · {scope(m.scope)}
        {m.enabled === false && ` · ${L("未启用", "Disabled")}`}
      </div>)}
      {mcpBlocked && !!tools.mcp?.length && <div className="app-small-note">{L("这些 MCP 在工作区权限下用不了：在上面「允许 MCP 工具」打开。", "These MCP tools don't run with workspace access: turn on Allow MCP tools above.")}</div>}
      {tools.skills?.map((s, i) => <div key={`s-${i}`} className="app-small-note">
        {L("技能", "Skill")} · {s.name}{s.description && ` · ${s.description}`} · {scope(s.scope)}{s.managed_by === "myco" && ` · ${L("myco 装的", "Installed by myco")}`}
      </div>)}
      {tools.plugins?.map((p, i) => <div key={`p-${i}`} className="app-small-note">
        {L("插件", "Plugin")} · {p.name}{p.marketplace && ` · ${p.marketplace}`} · {p.enabled ? L("已启用", "Enabled") : L("未启用", "Disabled")}
      </div>)}
      {!!tools.tool_warnings?.length && <div className="app-small-note" title={tools.tool_warnings.join("\n")}>{L(`有 ${tools.tool_warnings.length} 个配置文件没读懂`, `${tools.tool_warnings.length} configuration files could not be read`)}</div>}
    </div>
  );
}
