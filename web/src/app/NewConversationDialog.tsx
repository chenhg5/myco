import { useEffect, useMemo, useState } from "react";
import { Search } from "lucide-react";
import type { Hub } from "@/appapi";
import type { Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, aliasSummary, cliName, machineLabel, nodeIcon, nodeName, stateLabel } from "@/app/nodes";
import { stateMeta } from "@/components/status";
import { Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export interface NewConversationTarget {
  hubId?: string;
  agent?: string;
  /** Puts the conversation in this project (of hubId). */
  project?: string;
}

/**
 * Starts a conversation the way a chat app does: search, pick a node, and
 * its chat opens; the first message is written there.
 */
export function NewConversationDialog({ target, hubs, rows, projects, onClose, onPick }: {
  target: NewConversationTarget | null;
  hubs: Hub[];
  rows: AgentRow[];
  /** Projects of every hub: in a project, its members come first. */
  projects?: { hubId: string; p: Project }[] | null;
  onClose: () => void;
  onPick: (hubId: string, agent: string, project?: string) => void;
}) {
  const [query, setQuery] = useState("");
  useEffect(() => { if (target) setQuery(""); }, [target]);

  const project = target?.project ? (projects || []).find((x) => x.hubId === target.hubId && x.p.name === target.project)?.p : undefined;
  const members = new Set(project?.members.map((m) => m.name) || []);

  const groups = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = rows
      .filter((r) => {
        const k = agentKind(r.st.agent);
        if (k !== "agent" && k !== "shell") return false;
        if (target?.hubId && target.project && r.hub.id !== target.hubId) return false;
        if (!q) return true;
        const a = r.st.agent;
        return [nodeName(a, r.hub), a.name, a.description, cliName(a), machineLabel(r) || "", ...(a.aliases || [])].some((s) => (s || "").toLowerCase().includes(q));
      })
      .sort((a, b) =>
        Number(members.has(b.st.agent.name)) - Number(members.has(a.st.agent.name))
        || Number(a.st.state === "offline") - Number(b.st.state === "offline")
        || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub)));
    return hubs
      .map((h) => ({ hub: h, rows: list.filter((r) => r.hub.id === h.id) }))
      .filter((g) => g.rows.length > 0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, hubs, query, target?.hubId, target?.project, project]);

  const pick = (r: AgentRow) => onPick(r.hub.id, r.st.agent.name, target?.project);
  const first = groups[0]?.rows[0];

  return (
    <Modal
      open={!!target}
      onOpenChange={(o) => !o && onClose()}
      title={L("新对话", "New conversation")}
      description={project
        ? L(`选一个节点，第一条消息会放进项目「${project.title || project.name}」。`, `Pick a node; the first message goes into ${project.title || project.name}.`)
        : L("选一个节点，打开和它的对话。", "Pick a node to open your conversation with it.")}
      wide
    >
      <div className="app-convpick-search">
        <Search aria-hidden />
        <input
          className="app-convpick-input"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && first && !e.nativeEvent.isComposing) { e.preventDefault(); pick(first); } }}
          placeholder={L("搜索节点：名字、CLI、机器", "Search nodes: name, CLI, machine")}
          aria-label={L("搜索节点", "Search nodes")}
          autoFocus
        />
      </div>
      <div className="app-convpick-list">
        {groups.length === 0 && (
          <div className="app-empty-note">
            {rows.length === 0
              ? L("还没有可以发消息的节点。先在「节点」页添加一个。", "There are no nodes to message yet. Add one on the Nodes page first.")
              : L("没有匹配的节点。", "No matching nodes.")}
          </div>
        )}
        {groups.map((g) => (
          <div key={g.hub.id}>
            {groups.length > 1 && <div className="app-convpick-group">{g.hub.name}</div>}
            <div className="app-list">
              {g.rows.map((r) => {
                const a = r.st.agent;
                const Icon = nodeIcon(a);
                const st = stateLabel(r.st, stateMeta[r.st.state].label);
                const machine = machineLabel(r);
                const aliases = aliasSummary(a);
                return (
                  <button key={a.name} type="button" className={cn("app-row", r.st.state === "offline" && "offline")} onClick={() => pick(r)} title={a.name}>
                    <span className="app-row-icon"><Icon /></span>
                    <span className="app-row-main">
                      <span className="app-row-titleline">
                        <span className="app-row-title">{nodeName(a, r.hub)}</span>
                        {members.has(a.name) && <span className="app-label group">{L("项目成员", "Member")}</span>}
                      </span>
                      <span className="app-row-sub">
                        <span className={cn("dot", st.dot)} />
                        <span>{st.label}</span>
                        {aliases && (
                          <>
                            <span className="sep">·</span>
                            <span className="txt mono alias" title={(a.aliases || []).map((x) => "@" + x).join(", ")}>{aliases}</span>
                          </>
                        )}
                        <span className="sep">·</span>
                        <span className="txt">{cliName(a)}</span>
                        {machine && <><span className="sep">·</span><span className="txt">{machine}</span></>}
                      </span>
                    </span>
                    <span className="app-row-trail"><span className="app-chev">›</span></span>
                  </button>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </Modal>
  );
}
