import { useEffect, useMemo, useState } from "react";
import { Terminal } from "lucide-react";
import { appApi, type Hub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind } from "@/app/nodes";
import { ErrorText, Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";

export interface NewConversationTarget {
  hubId?: string;
  agent?: string;
}

interface NodeOption {
  hubId: string;
  hubName: string;
  name: string;
  label: string;
  shell: boolean;
  offline: boolean;
}

function valueOf(o: NodeOption): string {
  return JSON.stringify([o.hubId, o.name]);
}

/** Starts a conversation with an AI or shell node. */
export function NewConversationDialog({ target, hubs, rows, onClose, onSent }: {
  target: NewConversationTarget | null;
  hubs: Hub[];
  rows: AgentRow[];
  onClose: () => void;
  onSent: (hubId: string, threadId: string, agent: string) => void;
}) {
  const [hubId, setHubId] = useState("");
  const [agent, setAgent] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const options = useMemo<NodeOption[]>(() => {
    const byHub = new Map(hubs.map((h) => [h.id, h] as const));
    return rows
      .filter((r) => {
        const k = agentKind(r.st.agent);
        return k === "agent" || k === "shell";
      })
      .map((r) => {
        const a = r.st.agent;
        const shell = agentKind(a) === "shell";
        return {
          hubId: r.hub.id,
          hubName: byHub.get(r.hub.id)?.name || r.hub.name,
          name: a.name,
          label: `${a.display_name || a.name}${shell ? L("（Shell）", " (Shell)") : ""}${r.st.state === "offline" ? L("（离线）", " (offline)") : ""}`,
          shell,
          offline: r.st.state === "offline",
        };
      });
  }, [hubs, rows]);

  const node = options.find((o) => o.hubId === hubId && o.name === agent);

  useEffect(() => {
    if (!target) return;
    setHubId(target.hubId || "");
    setAgent(target.agent || "");
    setBody("");
    setBusy(false);
    setError(null);
  }, [target]);

  // Default to the first hub that has a node, and to a node in that hub.
  useEffect(() => {
    if (!target || hubId) return;
    const first = options[0];
    const fallback = hubs.find((h) => h.ok);
    setHubId(first?.hubId || fallback?.id || "");
  }, [target, hubId, options, hubs]);

  useEffect(() => {
    if (!target || !hubId) return;
    const list = options.filter((o) => o.hubId === hubId);
    if (!list.some((o) => o.name === agent)) setAgent(list[0]?.name || "");
  }, [target, hubId, agent, options]);

  const close = () => { if (!busy) onClose(); };

  const send = async () => {
    if (!hubId || !agent || !body.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      const m = await appApi.sendMessage(hubId, { to: agent, body: body.trim() });
      onSent(hubId, m.thread_id, agent);
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
    e.preventDefault();
    void send();
  };

  return (
    <Modal
      open={!!target}
      onOpenChange={(o) => !o && close()}
      title={L("新对话", "New conversation")}
      description={L("把任务发给一个节点，处理过程和结果都会出现在这里。", "Give a node a task. Its progress and results will show up here.")}
      wide
    >
      {options.length === 0 ? (
        <>
          <div className="app-empty-note" style={{ marginTop: 12 }}>
            {L("还没有可以发消息的节点。先在「节点」页添加一个。", "There are no nodes to message yet. Add one on the Nodes page first.")}
          </div>
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={onClose}>{L("关闭", "Close")}</button>
          </div>
        </>
      ) : (
        <>
          <label className="app-field">
            <span className="app-field-label">{L("节点", "Node")}</span>
            <select
              className="app-select-field"
              value={node ? valueOf(node) : ""}
              onChange={(e) => {
                const [h, n] = JSON.parse(e.target.value) as [string, string];
                setHubId(h);
                setAgent(n);
              }}
              disabled={busy}
            >
              {hubs.map((h) => {
                const list = options.filter((o) => o.hubId === h.id);
                if (list.length === 0) return null;
                return (
                  <optgroup key={h.id} label={h.name}>
                    {list.map((o) => (
                      <option key={o.name} value={valueOf(o)}>{o.label}</option>
                    ))}
                  </optgroup>
                );
              })}
            </select>
          </label>

          {node?.shell && (
            <div className="app-notice">
              <Terminal />
              <span>{L("这是一个 Shell 节点：发送的内容会作为命令执行。", "This is a Shell node: what you send runs as a command.")}</span>
            </div>
          )}

          <label className="app-field">
            <span className="app-field-label">{L("任务", "Task")}</span>
            <textarea
              className="app-input"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder={L("要让它做什么？", "What should it do?")}
              autoFocus
              disabled={busy}
            />
          </label>
          <div className="app-small-note">{L("Enter 发送，Shift+Enter 换行", "Enter to send, Shift+Enter for a new line")}</div>

          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
            <button
              type="button"
              className="app-btn app-btn-primary"
              onClick={() => void send()}
              disabled={busy || !body.trim() || !node}
            >
              {busy ? L("正在发送…", "Sending…") : L("发送", "Send")}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
