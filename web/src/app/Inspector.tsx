import { useEffect, useState } from "react";
import { useCached } from "@/lib/memo";
import { Check, ChevronRight, Copy, Pencil, Plus, Power, Trash2, UserCheck, X } from "lucide-react";
import { IconChat } from "@/components/icons";
import { appApi, isOldHub, type AgentProvider, type ProviderList } from "@/appapi";
import type { AgentRow } from "@/app/types";
import type { AuditEntry, Member, Peer, Project, Skill } from "@/api";
import {
  agentKind, cliName, cliVersion, cloudLabel, isCloud, isInbox, kindLabel, machineLabel,
  canManage, cleanLabel, groupsOf, isMe, machineOfRow, nodeIcon, personLabel, tagsOf,
  sessionBusy, stateLabel, stuckCounts, possiblyStuck, nodeName } from "@/app/nodes";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { CopyButton, useCopy } from "@/app/copy";
import { NodePicker } from "@/app/NodePicker";
import { summarize } from "@/app/summary";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";
import { L, lang, locale } from "@/lib/i18n";
import { RowsSkeleton } from "@/app/Skeleton";
import { Avatar } from "@/app/persona";
import { ResumeRow } from "@/app/ResumeRow";
import { RunRows } from "@/app/RunSettings";

type Action = null | "profile" | "skills" | "visibility" | "projects" | "allow" | "kick" | "delete" | "owner" | "copy";

// A skill being edited: examples as one text, one per line.
interface SkillDraft { name: string; summary: string; examples: string }
const toDraft = (s: Skill): SkillDraft => ({ name: s.name, summary: s.summary || "", examples: (s.examples || []).join("\n") });
const fromDraft = (d: SkillDraft): Skill => ({
  name: d.name.trim().toLowerCase(), summary: d.summary.trim() || undefined,
  examples: d.examples.split("\n").map((x) => x.trim()).filter(Boolean),
});
/** A node's skills; a hub before skills only has bare capability names. */
const skillsOf = (a: { skills?: Skill[]; capabilities: string[] }): Skill[] =>
  a.skills?.length ? a.skills : a.capabilities.map((name) => ({ name }));

const pillClass: Record<string, string> = { working: "green", pending: "amber", idle: "", offline: "" };

const isConsole = (name: string) => name.startsWith("@console:");

// MessageRows lists the threads you (the console) take part in with the node.
function MessageRows({ row, now, onThread }: {
  row: AgentRow;
  now: number;
  onThread: (hubId: string, threadId: string, agent: string) => void;
}) {
  const name = row.st.agent.name;
  const seen = new Set<string>();
  const msgs = (row.hub.overview?.recent || [])
    .filter((m) => m.from === name || m.to === name)
    .filter((m) => isMe(m.from, row.hub) || isMe(m.to, row.hub))
    .filter((m) => (seen.has(m.thread_id) ? false : (seen.add(m.thread_id), true)))
    .slice(0, 6);
  if (!msgs.length) return <div className="app-empty-note">{L("你还没有和它对话", "No conversation with it yet")}</div>;
  const label = (n: string) => (isConsole(n) ? personLabel(n, row.hub) : n === name ? nodeName(row.st.agent, row.hub) : n);
  return (
    <>
      {msgs.map((m) => (
        <button key={m.id} type="button" className="app-msg" onClick={() => onThread(row.hub.id, m.thread_id, name)}>
          <span className="meta">
            <span className="who">{label(m.from)}</span>→<span className="who">{label(m.to)}</span>
            <span className="time">{timeAgo(m.updated_at, now)}</span>
          </span>
          <span className="body">{summarize(m)}</span>
        </button>
      ))}
    </>
  );
}

// PeerRows lists the other nodes this node exchanged messages with, one row
// per node (however many threads); a row opens their whole history.
// What a history line says about a field ("labels: +a −b" → 标签 …).
const FIELD_LABEL: Record<string, () => string> = {
  display_name: () => L("显示名", "Display name"),
  description: () => L("介绍", "Description"),
  labels: () => L("分组和标签", "Group and tags"),
  skills: () => L("技能", "Skills"),
  avatar: () => L("头像", "Face"),
};

/** One audit entry about a node, as lines a person reads. */
function historyLines(e: AuditEntry): string[] {
  if (e.action === "agent.edit") {
    return (e.detail || "").split("\n").filter(Boolean).map((line) => {
      const i = line.indexOf(": ");
      const key = i > 0 ? line.slice(0, i) : "";
      if (!FIELD_LABEL[key]) return line;
      let text = line.slice(i + 2);
      // The avatar word means nothing to a person: say what happened.
      if (key === "avatar") text = text === "—" ? L("用回原来的", "back to the original") : text.startsWith("pic:") ? L("换成一张照片", "a picture") : L("换了一个蘑菇", "another mushroom");
      // "+a −b ~c" (labels, skills) in words.
      if (key === "labels" || key === "skills") {
        const groups: Record<string, string[]> = { "+": [], "−": [], "~": [] };
        for (const tok of text.split(" ").filter(Boolean)) (groups[tok[0]] ?? groups["~"]).push(tok.slice(1));
        text = [
          groups["+"].length ? L(`新增 ${groups["+"].join("、")}`, `added ${groups["+"].join(", ")}`) : "",
          groups["−"].length ? L(`去掉 ${groups["−"].join("、")}`, `removed ${groups["−"].join(", ")}`) : "",
          groups["~"].length ? L(`修改 ${groups["~"].join("、")}`, `edited ${groups["~"].join(", ")}`) : "",
        ].filter(Boolean).join(L("；", "; "));
      }
      return `${FIELD_LABEL[key]()}：${text}`;
    });
  }
  switch (e.action) {
    case "agent.visibility": return [L(`可见范围改为「${e.detail === "private" ? "仅自己" : "团队可见"}」`, `Visibility set to ${e.detail === "private" ? "only its owner" : "everyone on the hub"}`)];
    case "agent.owner": return [L(`转交：${e.detail || ""}`, `Given: ${e.detail || ""}`)];
    case "agent.update": return [e.detail?.startsWith("allow_from: ") ? L(`允许谁派任务：${e.detail.slice(12)}`, `Who can give tasks: ${e.detail.slice(12)}`) : (e.detail || L("修改", "Changed"))];
    case "agent.coordinator": return [e.detail === "on" ? L("设为协调者", "Made a coordinator") : L("不再是协调者", "No longer a coordinator")];
    case "agent.kick": return [L("踢下线", "Disconnected")];
    case "agent.register": return [L("注册", "Registered")];
    case "agent.rotate_token": return [L("更换了令牌", "Token replaced")];
  }
  return [e.detail ? `${e.action} ${e.detail}` : e.action];
}

/** Changes to a node's card and settings, newest first: who and when. */
function HistoryRows({ row, now, stamp }: { row: AgentRow; now: number; stamp: string }) {
  const ag = row.st.agent;
  const [all, setAll] = useState(false);
  const { data, error } = useCached(`history\u0000${row.hub.id}\u0000${ag.name}`,
    () => appApi.agentHistory(row.hub.id, ag.name).then((r) => r.entries), [stamp]);
  const entries = data ?? null;
  if (error && !entries && isOldHub(error)) {
    return <div className="app-empty-note">{L("升级这个工作区后，这里会显示谁在什么时候改了它。", "Once this hub is upgraded, this shows who changed it and when.")}</div>;
  }
  if (error && !entries) return <ErrorText error={error} />;
  if (!entries) return <RowsSkeleton rows={2} className="is-flat" />;
  if (!entries.length) return <div className="app-empty-note">{L("还没有修改记录。", "No changes yet.")}</div>;
  const shown = all ? entries : entries.slice(0, 5);
  const who = (e: AuditEntry) =>
    e.actor_kind === "agent" ? (e.actor === ag.name ? L("它自己", "Itself") : e.actor)
      : e.actor_kind === "admin" ? L("管理员", "Admin")
        : row.hub.me?.member?.name === e.actor ? L("你", "You") : e.actor;
  return (
    <>
      {shown.map((e) => (
        <div className="app-history" key={e.id}>
          <div className="app-history-head">
            <span className="app-history-who">{who(e)}</span>
            <span className="app-history-time" title={new Date(e.at).toLocaleString(locale(), { hour12: false })}>{timeAgo(e.at, now)}</span>
          </div>
          {historyLines(e).map((l, i) => <div className="app-history-line" key={i}>{l}</div>)}
        </div>
      ))}
      {entries.length > 5 && (
        <button type="button" className="app-history-more" onClick={() => setAll((v) => !v)}>
          {all ? L("收起", "Show less") : L(`显示全部 ${entries.length} 条`, `Show all ${entries.length}`)}
        </button>
      )}
    </>
  );
}

function PeerRows({ row, now, onPeer }: {
  row: AgentRow;
  now: number;
  onPeer: (hubId: string, agent: string, peer: string) => void;
}) {
  const name = row.st.agent.name;
  const hubId = row.hub.id;
  // Reload when the node's activity changes (the overview refreshes).
  const stamp = (row.hub.overview?.recent || []).find((m) => m.from === name || m.to === name)?.id || "";
  const { data, error } = useCached(`peers\u0000${hubId}\u0000${name}`,
    () => appApi.peers(hubId, name).then((r) => r.peers), [stamp]);
  const peers: Peer[] | null = data ?? null;
  if (error && !peers) return <ErrorText error={error} />;
  if (!peers) return <RowsSkeleton rows={2} className="is-flat" />;
  if (!peers.length) return <div className="app-empty-note">{L("还没有和其他 agent 的往来", "No exchanges with other nodes yet")}</div>;
  const nameOf = (n: string) => { const x = (row.hub.overview?.agents || []).find((y) => y.agent.name === n); return x ? nodeName(x.agent, row.hub) : n; };
  return (
    <>
      {peers.map((p) => (
        <button key={p.agent} type="button" className="app-msg" onClick={() => onPeer(hubId, name, p.agent)}>
          <span className="meta">
            <span className="who">{nameOf(p.agent)}</span>
            <span className="count">{L(`${p.messages} 条消息`, `${p.messages} message${p.messages === 1 ? "" : "s"}`)}</span>
            {p.last && <span className="time">{timeAgo(p.last.created_at, now)}</span>}
          </span>
          {p.last && <span className="body">{p.last.from === name ? "→ " : "← "}{summarize(p.last)}</span>}
        </button>
      ))}
    </>
  );
}

// LabelsEditor shows the node's group (a group=… label) and tags (bare
// labels), and edits them in a dialog. Other key=value labels, such as
// machine=…, are left alone.
function LabelsEditor({ row, onRefresh, readOnly }: { row: AgentRow; onRefresh: () => Promise<void>; readOnly?: boolean }) {
  const ag = row.st.agent;
  const groups = groupsOf(ag);
  const tags = tagsOf(ag);
  const [open, setOpen] = useState(false);
  const [group, setGroup] = useState("");
  const [draftTags, setDraftTags] = useState<string[]>([]);
  const [tag, setTag] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const others = (row.hub.overview?.agents || []).map((x) => x.agent);
  const knownGroups = [...new Set(others.flatMap(groupsOf))].sort((a, b) => a.localeCompare(b, locale()));
  const knownTags = [...new Set(others.flatMap(tagsOf))].sort((a, b) => a.localeCompare(b, locale()));

  const start = () => {
    setGroup(groups[0] || "");
    setDraftTags(tags);
    setTag("");
    setError(null);
    setOpen(true);
  };
  const addTag = (t: string) => {
    const v = cleanLabel(t);
    if (v && !draftTags.includes(v)) setDraftTags([...draftTags, v]);
    setTag("");
  };
  const save = async () => {
    const g = cleanLabel(group);
    const finalTags = cleanLabel(tag) ? [...new Set([...draftTags, cleanLabel(tag)])] : draftTags;
    const add = [...(g && !groups.includes(g) ? ["group=" + g] : []), ...finalTags.filter((t) => !tags.includes(t))];
    const remove = [...groups.filter((x) => x !== g).map((x) => "group=" + x), ...tags.filter((t) => !finalTags.includes(t))];
    if (!add.length && !remove.length) { setOpen(false); return; }
    setBusy(true);
    setError(null);
    try {
      await appApi.setLabels(row.hub.id, ag.name, { add, remove });
      await onRefresh();
      setOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <button type="button" className="app-labels-summary" onClick={start} disabled={readOnly}>
        <span className="app-labels-chips">
          {groups.length === 0 && tags.length === 0 && <span className="app-labels-empty">{L("未分组，没有标签", "No group, no tags")}</span>}
          {groups.map((g) => <span key={"g" + g} className="app-label group">{g}</span>)}
          {tags.map((t) => <span key={"t" + t} className="app-label">#{t}</span>)}
        </span>
        <span className="app-labels-edit">{L("编辑", "Edit")}<span className="app-chev">›</span></span>
      </button>

      <Modal open={open} onOpenChange={(o) => !busy && setOpen(o)} title={L("分组和标签", "Group and tags")} description={L("分组和标签只用来在 App 里整理和筛选 agent。", "Groups and tags are only used to organize and filter nodes in the app.")}>
        <label className="app-field">
          <span className="app-field-label">{L("分组", "Group")}</span>
          <input className="app-input" value={group} onChange={(e) => setGroup(e.target.value)} placeholder={L("不分组", "No group")} disabled={busy} />
        </label>
        {knownGroups.length > 0 && (
          <div className="app-label-picks">
            {knownGroups.map((g) => (
              <button key={g} type="button" className={cn("app-label group", cleanLabel(group) === g && "picked")} onClick={() => setGroup(g)}>{g}</button>
            ))}
          </div>
        )}
        <div className="app-field">
          <span className="app-field-label">{L("标签", "Tags")}</span>
          <div className="app-tag-box">
            {draftTags.map((t) => (
              <span key={t} className="app-label editable">
                #{t}
                <button type="button" aria-label={L(`去掉标签 ${t}`, `Remove tag ${t}`)} onClick={() => setDraftTags(draftTags.filter((x) => x !== t))}>×</button>
              </span>
            ))}
            <input
              value={tag}
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return;
                if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addTag(tag); }
                if (e.key === "Backspace" && !tag && draftTags.length) setDraftTags(draftTags.slice(0, -1));
              }}
              placeholder={draftTags.length ? "" : L("输入后按回车", "Type and press Return")}
              aria-label={L("添加标签", "Add tag")}
              disabled={busy}
            />
          </div>
        </div>
        {knownTags.filter((t) => !draftTags.includes(t)).length > 0 && (
          <div className="app-label-picks">
            {knownTags.filter((t) => !draftTags.includes(t)).map((t) => (
              <button key={t} type="button" className="app-label" onClick={() => addTag(t)}>+ #{t}</button>
            ))}
          </div>
        )}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setOpen(false)} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={busy}>
            {busy ? L("正在保存…", "Saving…") : L("保存", "Save")}
          </button>
        </div>
      </Modal>
    </>
  );
}

// ProviderRow: which model provider (endpoint + key) the node's CLI uses.
// A node on this computer can be pinned to a cc-switch provider, given an
// endpoint by hand, or follow the computer's own setting.
function ProviderRow({ row, onRefresh }: { row: AgentRow; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const rt = ag.runtime;
  const prov = useCached(`provider\u0000${row.hub.id}\u0000${ag.name}`, () => appApi.agentProvider(ag.name, row.hub.id));
  const local: AgentProvider | null = prov.data ?? null;
  const setLocal = prov.set;
  const [list, setList] = useState<ProviderList | null>(null);
  const [open, setOpen] = useState(false);
  const [choice, setChoice] = useState("");
  const [manual, setManual] = useState({ base_url: "", api_key: "", model: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState("");

  useEffect(() => { setSaved(""); }, [row.hub.id, ag.name]);

  const codex = (rt?.cli || ag.binding.harness) === "codex";
  const editable = !!local?.local && (local.harness === "claude" || local.harness === "codex");
  const start = async () => {
    setError(null);
    setChoice(local?.provider || "");
    setManual({ base_url: "", api_key: "", model: "" });
    setOpen(true);
    try { setList(await appApi.providers(codex ? "codex" : "claude")); } catch (e) { setError(e); }
  };
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const body = choice === "env_file"
        ? (manual.base_url || manual.api_key ? { manual } : null)
        : { provider: choice };
      if (body) setLocal(await appApi.setAgentProvider(ag.name, row.hub.id, body));
      setSaved(L("已保存，从它收到的下一条消息开始生效。", "Saved. It applies from the node's next message."));
      setOpen(false);
      window.setTimeout(() => void onRefresh(), 1500);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const current = list?.providers.find((p) => p.current);
  const name = rt?.provider || (rt ? L("CLI 自己的设置", "The CLI's own setting") : "—");
  return (
    <div className={cn("app-kv", saved && "has-help")}>
      <span className="app-kv-label">{L("服务商", "Provider")}</span>
      <span className="app-kv-value">
        {editable ? (
          <button type="button" className="app-kv-link" onClick={() => void start()}>
            {name}<span className="app-chev">›</span>
          </button>
        ) : name}
      </span>
      {saved && <span className="app-kv-help app-offline-ok">{saved}</span>}

      <Modal open={open} onOpenChange={(o) => !busy && setOpen(o)} title={L("服务商", "Provider")}
        description={L(`后台唤醒这个 agent 时，它的 ${codex ? "Codex" : "Claude Code"} 用哪个服务商（地址、密钥和模型）。只影响这个 agent，密钥只保存在这台电脑上。`, `Which provider (endpoint, key and model) this node's ${codex ? "Codex" : "Claude Code"} uses when it is woken in the background. Only this node is affected; keys stay on this computer.`)}>
        <div className="app-provider-list" role="radiogroup">
          <label className={cn("app-provider-opt", choice === "" && "picked")}>
            <input type="radio" name="provider" checked={choice === ""} onChange={() => setChoice("")} />
            <span>
              <b>{L("跟随本机设置", "Follow this computer")}</b>
              <small>{current ? L(`现在是「${current.name}」，在 cc-switch 里切换时跟着变`, `Now “${current.name}”; changes when you switch in cc-switch`) : L(`用这台电脑上 ${codex ? "Codex" : "Claude Code"} 自己的设置`, `Uses ${codex ? "Codex" : "Claude Code"}'s own settings on this computer`)}</small>
            </span>
          </label>
          {list?.providers.map((p) => (
            <label key={p.ref} className={cn("app-provider-opt", choice === p.ref && "picked")}>
              <input type="radio" name="provider" checked={choice === p.ref} onChange={() => setChoice(p.ref)} />
              <span>
                <b>{p.name}</b>
                <small>{[p.host || L("官方地址", "Official endpoint"), p.model, "cc-switch"].filter(Boolean).join(" · ")}</small>
              </span>
            </label>
          ))}
          <label className={cn("app-provider-opt", choice === "env_file" && "picked")}>
            <input type="radio" name="provider" checked={choice === "env_file"} onChange={() => setChoice("env_file")} />
            <span>
              <b>{L("手动填写", "Enter by hand")}</b>
              <small>{local?.env_host ? L(`现在是 ${local.env_host}`, `Now ${local.env_host}`) : codex ? L("OpenAI 兼容的地址和密钥", "An OpenAI-compatible endpoint and key") : L("Anthropic 兼容的地址和密钥", "An Anthropic-compatible endpoint and key")}</small>
            </span>
          </label>
        </div>
        {choice === "env_file" && (
          <div className="app-provider-manual">
            <input className="app-input" placeholder={codex ? "https://api.example.com/v1" : "https://api.example.com/anthropic"} value={manual.base_url} onChange={(e) => setManual({ ...manual, base_url: e.target.value })} disabled={busy} />
            <input className="app-input" type="password" placeholder={L("密钥", "Key")} value={manual.api_key} onChange={(e) => setManual({ ...manual, api_key: e.target.value })} disabled={busy} autoComplete="off" />
            <input className="app-input" placeholder={L("模型（可选）", "Model (optional)")} value={manual.model} onChange={(e) => setManual({ ...manual, model: e.target.value })} disabled={busy} />
          </div>
        )}
        {list && !list.cc_switch && (
          <p className="app-modal-note">{L("装了 cc-switch 的话，这里会列出它的服务商。", "With cc-switch installed, its providers are listed here.")}</p>
        )}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setOpen(false)} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void save()}
            disabled={busy || (choice === "env_file" && !local?.env_host && (!manual.base_url || !manual.api_key))}>
            {busy ? L("正在保存…", "Saving…") : L("保存", "Save")}
          </button>
        </div>
      </Modal>
    </div>
  );
}

const hostKey = (h?: string) => (h || "").toLowerCase().replace(/\.local$/, "");

function openStatusLabel(status: string, stuck = false): string {
  if (stuck) return L("可能卡住", "May be stuck");
  // delivered: the node pulled it but has not acknowledged it yet.
  if (status === "queued" || status === "delivered") return L("待处理", "Queued");
  if (status === "held") return L("已挂起", "Held");
  return L("处理中", "In progress");
}

// The command a session keeps running to receive in the window.
const WAIT_CMD = "myco inbox wait";

function receivingLabel(r?: string) {
  if (r === "session") return L("会话里在线接收", "In the session");
  if (r === "runner") return L("后台唤醒", "Woken in the background");
  return "—";
}

function visibilityLabel(v?: string) {
  return v === "private" ? L("仅自己", "Only its owner") : L("团队可见", "Everyone on the hub");
}

// InUseCard: the runner found the session open elsewhere (a terminal or the
// IDE) and handed messages back instead of interrupting. The fix is to let
// that open session receive by itself.
export function InUseCard() {
  const ask = L(
    `请在后台一直运行 \`${WAIT_CMD}\`，结束后马上再启动一次，收到消息时用 myco inbox pull 读取并处理。`,
    `Keep \`${WAIT_CMD}\` running as a background command, start it again whenever it exits, and when it reports messages read them with myco inbox pull and handle them.`,
  );
  return (
    <section className="app-offline app-inuse">
      <div className="app-offline-head">
        <span className="dot pending" />
        <b>{L("会话正在被使用", "Session in use")}</b>
      </div>
      <p>{L("这个会话正开在终端或编辑器里，myco 后台 不会打断你，消息先排着、过一会儿再试。想让它在你开着的窗口里直接收消息，把下面这句话发给那个会话：", "This session is open in a terminal or editor, so the runner won't interrupt you: messages wait and are retried later. To have the open window receive them directly, send this sentence to that session:")}</p>
      <div className="app-command">
        <code>{ask}</code>
        <CopyButton value={ask} />
      </div>
    </section>
  );
}

// OfflineCard says why a node is offline and how to bring it back: on this
// computer, restart the runner; on another one, the command to run there;
// a cloud session comes back when its session continues.
export function OfflineCard({ row, now, onRefresh }: { row: AgentRow; now: number; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const machine = machineOfRow(row);
  const [hostname, setHostname] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let off = false;
    setNote("");
    setError(null);
    appApi.local().then((l) => { if (!off) setHostname(l.hostname); }).catch(() => {});
    return () => { off = true; };
  }, [row.hub.id, ag.name]);
  const here = !!machine?.hostname && !!hostname && hostKey(machine.hostname) === hostKey(hostname);

  const restart = async () => {
    setBusy(true);
    setError(null);
    setNote("");
    try {
      const r = await appApi.restartRunner();
      if (r.runner !== "running") throw new Error(r.runner);
      setNote(L("myco 后台 已重启，通常半分钟内会重新上线。", "The runner restarted. The node is usually back online within half a minute."));
      window.setTimeout(() => void onRefresh(), 8000);
      window.setTimeout(() => void onRefresh(), 30000);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const seen = ag.last_seen_at ? timeAgo(ag.last_seen_at, now) : "";
  // An inbox node has no runner behind it: it comes back when its session
  // runs `myco inbox wait` again, not by restarting a runner.
  if (isInbox(ag)) {
    return (
      <section className="app-offline">
        <div className="app-offline-head">
          <span className="dot offline" />
          <b>{L("离线", "Offline")}</b>
          {seen && <span>{L(`· 最后一次连接工作区是 ${seen}`, `· Last connected to the Hub ${seen}`)}</span>}
        </div>
        <p>
          {L("这个 agent 只在会话里接收消息：它的会话运行时挂着 myco inbox wait。会话关掉后它就离线，发给它的任务会先排队，等它再挂上 inbox wait 才会收到。", "This node receives only inside its session: while that session runs, myco inbox wait is attached to it. When the session closes it goes offline and tasks wait in a queue until inbox wait is attached again.")}
        </p>
        <p>{L("回到那个会话，让它在后台运行下面这条命令，它就会重新上线。", "Go back to that session and run this command in the background to bring it back online.")}</p>
        <div className="app-command">
          <code>{WAIT_CMD}</code>
          <CopyButton value={WAIT_CMD} />
        </div>
      </section>
    );
  }
  return (
    <section className="app-offline">
      <div className="app-offline-head">
        <span className="dot offline" />
        <b>{L("离线", "Offline")}</b>
        {seen && <span>{L(`· 最后一次连接工作区是 ${seen}`, `· Last connected to the Hub ${seen}`)}</span>}
      </div>
      <p>
        {L("在线是指它最近 90 秒内连过工作区： agent 所在电脑上的 myco 后台 会每隔几秒替它连一次。 离线时发给它的任务会先排队，上线后自动送达。", "Online means it reached the Hub in the last 90 seconds: the runner on the node's machine checks in for it every few seconds. Tasks sent while it is offline wait in a queue and are delivered when it comes back.")}
      </p>
      {isCloud(ag) ? (
        <p>{L("这是云端会话，会话结束就会离线。回到那个会话继续对话，它就会重新上线。", "This is a cloud session. It goes offline when the session ends. Continue the conversation in that session to bring it back online.")}</p>
      ) : here ? (
        <>
          <p>{L("它在这台电脑上。重启这里的 myco 后台，它就会重新连上工作区。", "It runs on this computer. Restart the runner here and it reconnects to the Hub.")}</p>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void restart()} disabled={busy}>
            {busy ? <><span className="app-spinner" />{L("正在重启…", "Restarting…")}</> : L("重启本机的 myco 后台", "Restart the runner on this computer")}
          </button>
        </>
      ) : (
        <>
          <p>
            {machine ? (lang() === "zh" ? <>它在「{machine.name}」上。</> : <>It runs on “{machine.name}”. </>) : null}
            {L("在那台电脑的终端里运行下面这条命令（启动 myco 后台；已经在运行就重启它）。电脑关机或睡眠时，开机后会自动上线。", "Run this command in a terminal on that machine (it starts the runner, or restarts it if it's already running). If the machine is off or asleep, the node comes back online once it wakes.")}
          </p>
          <div className="app-command">
            <code>myco runner service restart || myco runner service install</code>
            <CopyButton value="myco runner service restart || myco runner service install" />
          </div>
        </>
      )}
      {note && <p className="app-offline-ok">{note}</p>}
      <ErrorText error={error} />
    </section>
  );
}

// AliasChip shows one alias. Its name copies the alias (the form a sender
// types); the × beside it removes the alias for an owner or admin.
function AliasChip({ alias, busy, onRemove }: {
  alias: string;
  busy: boolean;
  onRemove?: () => void;
}) {
  const { copied, copy } = useCopy(alias);
  const copyLabel = L(`复制别名 ${alias}`, `Copy alias ${alias}`);
  return (
    <span className="app-label editable alias">
      <button type="button" className="copy" onClick={() => void copy()} disabled={busy} title={copyLabel} aria-label={copyLabel}>
        {copied ? L("已复制", "Copied") : "@" + alias}
      </button>
      {onRemove && (
        <button type="button" aria-label={L(`删除别名 ${alias}`, `Remove alias ${alias}`)} onClick={onRemove} disabled={busy}>×</button>
      )}
    </span>
  );
}

// AliasRow lists the addresses others can use to message this node. An owner
// or hub admin adds and removes them inline, without a dialog.
function AliasRow({ row, mayManage, onRefresh }: {
  row: AgentRow;
  mayManage: boolean;
  onRefresh: () => Promise<void>;
}) {
  const ag = row.st.agent;
  const aliases = (ag.aliases || []).filter(Boolean);
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const start = () => {
    setDraft("");
    setError(null);
    setAdding(true);
  };
  const add = async () => {
    const alias = draft.trim().toLowerCase();
    if (!alias) return;
    if (!/^[a-z0-9][a-z0-9._-]{0,62}$/.test(alias)) {
      setError(new Error(L(
        "别名只能用 1-63 位小写字母、数字、点、下划线或连字符，并以字母或数字开头。",
        "An alias uses 1-63 lowercase letters, digits, '.', '_' or '-', starting with a letter or digit.",
      )));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await appApi.setAlias(row.hub.id, alias, ag.name);
      await onRefresh();
      setAdding(false);
      setDraft("");
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const remove = async (alias: string) => {
    setBusy(true);
    setError(null);
    try {
      await appApi.removeAlias(row.hub.id, alias);
      await onRefresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="app-kv has-help">
      <span className="app-kv-label">{L("别名", "Aliases")}</span>
      <span className="app-kv-value app-aliases">
        {aliases.length === 0 && <span className="app-kv-note">{L("还没有别名", "None yet")}</span>}
        {aliases.map((alias) => (
          <AliasChip key={alias} alias={alias} busy={busy}
            onRemove={mayManage ? () => void remove(alias) : undefined} />
        ))}
        {mayManage && !adding && (
          <button type="button" className="app-kv-link" onClick={start} disabled={busy}>
            <Plus size={12} aria-hidden />{L("添加", "Add")}
          </button>
        )}
      </span>
      <span className="app-kv-help">
        {L("别人可以用别名给它发消息；点别名复制。", "Others can message it by an alias; click one to copy it.")}
      </span>
      {adding && (
        <form className="app-alias-form" onSubmit={(e) => { e.preventDefault(); void add(); }}>
          <input className="app-input" value={draft} onChange={(e) => setDraft(e.target.value)}
            placeholder={L("例如 reviewer", "e.g. reviewer")} aria-label={L("新别名", "New alias")}
            disabled={busy} autoFocus />
          <button type="submit" className="app-btn app-btn-small app-btn-primary" disabled={busy || !draft.trim()}>
            {busy ? L("正在添加…", "Adding…") : L("确认", "Add")}
          </button>
          <button type="button" className="app-btn app-btn-small" onClick={() => setAdding(false)} disabled={busy}>
            {L("取消", "Cancel")}
          </button>
        </form>
      )}
      {error != null && <div className="app-kv-help"><ErrorText error={error} /></div>}
    </div>
  );
}

export function Inspector({ row, now, projects, onClose, onRefresh, onThread, onPeer, onMessage, onOpenMachine, onOpenProject, onProjectsChanged, embedded = false }: {
  row: AgentRow | null;
  /** inside a member's card: no panel of its own; what the card already
   *  shows (name, description, projects, chat) is left out, and the
   *  technical details fold away */
  embedded?: boolean;
  now: number;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onThread: (hubId: string, threadId: string, agent: string) => void;
  // Opens the whole history between agent and another node.
  onPeer: (hubId: string, agent: string, peer: string) => void;
  onMessage: (row: AgentRow) => void;
  onOpenMachine: (hubId: string, machineId: string) => void;
  /** Projects of every hub, as the sidebar lists them. */
  projects?: { hubId: string; p: Project }[] | null;
  onOpenProject?: (ref: { hubId: string; name: string }) => void;
  onProjectsChanged?: () => void;
}) {
  const ag = row?.st.agent;
  const [action, setAction] = useState<Action>(null);
  const [people, setPeople] = useState<Member[]>([]);
  const [newOwner, setNewOwner] = useState("");
  const [copyName, setCopyName] = useState("");
  const [copied, setCopied] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [allowFrom, setAllowFrom] = useState<string[]>([]);
  const [skillDrafts, setSkillDrafts] = useState<SkillDraft[]>([]);
  const [visChoice, setVisChoice] = useState<"team" | "private">("team");
  const [joinChoice, setJoinChoice] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    setAction(null);
    setError(null);
    setBusy(false);
    setDisplayName(ag?.display_name || "");
    setDescription(ag?.description || "");
    setAllowFrom(ag?.allow_from || []);
    // Only reset when a different agent is opened, not on every 5s poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [row?.hub.id, ag?.id]);

  useEffect(() => {
    if (!row) return;
    if (embedded) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !action) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [row, action, onClose, embedded]);
  const [more, setMore] = useState(false);

  if (!row || !ag) return null;

  const hubId = row.hub.id;
  const state = stateMeta[row.st.state];
  const kind = agentKind(ag);
  const inbox = isInbox(ag);
  const Icon = nodeIcon(ag);
  const rt = ag.runtime;
  const machine = machineLabel(row);
  const machineObj = machineOfRow(row);
  const mayManage = canManage(row.hub, ag.owner_id);
  // Who a confirmation is about: the face, the name people know, the id.
  const who = (
    <>
      <Avatar agent={ag} size={32} badge={false} />
      <span className="app-confirm-who">
        <span className="app-confirm-name">{nodeName(ag, row.hub)}</span>
        {nodeName(ag, row.hub) !== ag.name && <span className="app-confirm-id">{ag.name}</span>}
      </span>
    </>
  );
  const me = row.hub.me;
  // Copy: fork the session into a new node on the same machine and folder.
  // An inbox node has no CLI session to copy and no runner to start it: it
  // cannot be forked or spawned.
  const forkable = kind === "agent" && !inbox && (ag.binding.harness === "claude" || ag.binding.harness === "codex") &&
    !!ag.binding.session_id && !!ag.binding.workdir && !!machineObj?.can_spawn && canManage(row.hub, machineObj?.owner_id);
  const openCopy = () => {
    const names = new Set((row.hub.overview?.agents || []).map((x) => x.agent.name));
    let n = 2;
    while (names.has(`${ag.name}-${n}`)) n++;
    setCopyName(`${ag.name}-${n}`);
    setCopied("");
    openAction("copy");
  };
  const copyNode = () => void run(async () => {
    if (!machineObj) return;
    const r = await appApi.spawn(hubId, machineObj.id, {
      cli: ag.binding.harness as "claude" | "codex", dir: ag.binding.workdir || "", fork_from: ag.binding.session_id,
      name: copyName.trim(), description: ag.description || undefined,
      display_name: ag.display_name ? `${ag.display_name} ${L("副本", "copy")}` : undefined,
      ...(rt?.access ? { access: rt.access } : {}),
    });
    setCopied(r.agent || copyName);
  }, "stay");
  const openOwner = () => {
    setNewOwner(ag.owner || "");
    appApi.members(hubId).then((r) => setPeople(r.members)).catch(() => {});
    openAction("owner");
  };

  const hubProjects = (projects || []).filter((x) => x.hubId === hubId).map((x) => x.p);
  const inProjects = hubProjects.filter((p) => p.members.some((m) => m.name === ag.name));
  const joinable = hubProjects.filter((p) => !p.archived && !p.members.some((m) => m.name === ag.name));
  const roleIn = (p: Project) => p.members.find((m) => m.name === ag.name)?.role;

  const openAction = (a: Exclude<Action, null>) => { setError(null); setAction(a); };
  const closeAction = () => { if (!busy) { setAction(null); setError(null); } };

  const run = async (fn: () => Promise<unknown>, after: "close-action" | "close-panel" | "stay") => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await onRefresh();
      if (after === "close-action") { setAction(null); }
      if (after === "close-panel") { setAction(null); onClose(); }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const saveProfile = () => void run(
    () => appApi.updateProfile(hubId, ag.name, { display_name: displayName.trim(), description: description.trim() }),
    "close-action",
  );
  const openSkills = () => {
    const have = skillsOf(ag).map(toDraft);
    setSkillDrafts(have.length ? have : [{ name: "", summary: "", examples: "" }]);
    openAction("skills");
  };
  const editDraft = (i: number, p: Partial<SkillDraft>) => setSkillDrafts((ds) => ds.map((d, j) => (j === i ? { ...d, ...p } : d)));
  const saveSkills = () => void run(
    () => appApi.setSkills(hubId, ag.name, skillDrafts.filter((d) => d.name.trim()).map(fromDraft)),
    "close-action",
  );
  // "*" (any node) is set by join links; picking nodes replaces it. A shell
  // node always names who may run commands, so clearing keeps "*" there.
  const anyNode = allowFrom.includes("*");
  const saveAllowFrom = () => {
    // Still open to any node: nothing to change ("*" is only set with the
    // CLI's explicit --allow-anyone, never by name).
    if (anyNode || (allowFrom.length === 0 && ag.allow_from.includes("*"))) {
      closeAction();
      return;
    }
    void run(() => appApi.updateAllowFrom(hubId, ag.name, allowFrom), "close-action");
  };

  const current = row.st.current || [];
  // Tasks picked up and quiet since: their results may never come back, so
  // they are counted apart from the ones still moving.
  const stuck = stuckCounts(row.st, now);
  const isStuck = (m: { status: string; active_at?: string; updated_at: string }) =>
    possiblyStuck(m, { agent: row.st.agent, receiving: row.st.receiving, offline: row.st.state === "offline" }, now);

  const sec_state = (
        <>
          {row.st.state === "offline" && <OfflineCard row={row} now={now} onRefresh={onRefresh} />}
          {row.st.state !== "offline" && row.st.session_in_use && row.st.receiving !== "session" && <InUseCard />}

        </>
      );
  const sec_about = (
        <>
          {kind !== "human" && (
            <section className="app-group">
              <div className="app-group-title">
                <span className="name">{embedded ? L("技能", "Skills") : L("介绍与技能", "About and skills")}</span>
                {mayManage && (
                  <button type="button" className="app-group-action" onClick={openSkills}>
                    <Pencil />{L("编辑技能", "Edit skills")}
                  </button>
                )}
              </div>
              <div className="app-list">
                {!embedded && <div className={cn("app-about", !ag.description && "is-empty")}>
                  {ag.description || L("还没有介绍。", "No description yet.")}
                  {mayManage && (
                    <button type="button" className="app-about-edit" onClick={() => openAction("profile")} aria-label={L("修改介绍", "Edit description")} title={L("修改介绍", "Edit description")}>
                      <Pencil />
                    </button>
                  )}
                </div>}
                {skillsOf(ag).length === 0 ? (
                  <div className="app-skill-empty">
                    {L("还没有技能。技能告诉别的 agent 可以把什么活派给它；它自己也能用 myco agent skills add 添加。", "No skills yet. Skills tell other agents what work to hand it; it can add them itself with myco agent skills add.")}
                  </div>
                ) : skillsOf(ag).map((s) => (
                  <div className="app-skill" key={s.name}>
                    <span className="app-skill-name">{s.name}</span>
                    {s.summary && <span className="app-skill-summary">{s.summary}</span>}
                    {s.examples?.map((x) => <span className="app-skill-example" key={x}>{L("例：", "e.g. ")}{x}</span>)}
                  </div>
                ))}
              </div>
            </section>
          )}

        </>
      );
  const sec_runtime = (
        <>
          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("运行", "Runtime")}</span></div>
            <div className="app-list">
              <div className="app-kv">
                <span className="app-kv-label">{L("类型", "Type")}</span>
                <span className="app-kv-value">{kindLabel(kind)}</span>
              </div>
              {ag.owner && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("主人", "Owner")}</span>
                  <span className="app-kv-value">
                    {me?.can_manage ? (
                      <button type="button" className="app-kv-link" onClick={openOwner}>
                        {me.member?.id === ag.owner_id ? <span title={ag.owner}>{L("你", "You")}</span> : ag.owner}<span className="app-chev">›</span>
                      </button>
                    ) : <>{me?.member?.id === ag.owner_id ? <span title={ag.owner}>{L("你", "You")}</span> : ag.owner}</>}
                  </span>
                </div>
              )}
              {kind !== "human" && row.hub.me?.member && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("可见范围", "Who can see it")}</span>
                  <span className="app-kv-value">
                    {mayManage && ag.owner ? (
                      <button type="button" className="app-kv-link" onClick={() => { setVisChoice(ag.visibility === "private" ? "private" : "team"); openAction("visibility"); }}>
                        {visibilityLabel(ag.visibility)}<span className="app-chev">›</span>
                      </button>
                    ) : visibilityLabel(ag.visibility)}
                  </span>
                </div>
              )}
              {!embedded && kind !== "human" && ag.binding.harness !== "machine" && (inProjects.length > 0 || (mayManage && joinable.length > 0)) && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("项目", "Projects")}</span>
                  <span className="app-kv-value app-kv-projects">
                    {inProjects.map((p) => (
                      <button key={p.name} type="button" className="app-kv-link" onClick={() => onOpenProject?.({ hubId, name: p.name })}>
                        {p.title || p.name}{roleIn(p) === "coordinator" && <span className="app-kv-note">{L("（协调者）", " (coordinator)")}</span>}
                      </button>
                    ))}
                    {mayManage && joinable.length > 0 && (
                      <button type="button" className="app-kv-link" onClick={() => { setJoinChoice(joinable[0].name); openAction("projects"); }}>
                        {L("加入项目", "Join")}
                      </button>
                    )}
                  </span>
                </div>
              )}
              {kind !== "human" && (row.st.receiving || inbox) && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("接收方式", "Receives")}</span>
                  <span className="app-kv-value">
                    {inbox
                      ? L("只能在会话里接收，不能后台唤醒", "In its session only; no background wake")
                      : receivingLabel(row.st.receiving)}
                  </span>
                </div>
              )}
              <div className="app-kv">
                <span className="app-kv-label">CLI</span>
                <span className="app-kv-value">
                  {kind === "human" ? "—" : <>{cliName(ag)}{cliVersion(ag) ? ` ${cliVersion(ag)}` : ""}</>}
                </span>
              </div>
              <RunRows row={row} now={now} mayManage={mayManage} onRefresh={onRefresh} />
              {kind === "agent" && !isCloud(ag) && ["claude", "codex"].includes(rt?.cli || ag.binding.harness || "") && <ProviderRow row={row} onRefresh={onRefresh} />}
              <div className="app-kv">
                <span className="app-kv-label">{L("电脑", "Machine")}</span>
                <span className="app-kv-value">
                  {machineObj ? (
                    <button
                      type="button"
                      className="app-kv-link"
                      onClick={() => onOpenMachine(hubId, machineObj.id)}
                    >
                      {machineObj.name}<span className="app-chev">›</span>
                    </button>
                  ) : isCloud(ag) ? (
                    cloudLabel(ag)
                  ) : (
                    machine || "—"
                  )}
                </span>
              </div>
            </div>
          </section>

        </>
      );
  const sec_labels = (
        <>
          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("分组和标签", "Group and tags")}</span></div>
            <LabelsEditor row={row} onRefresh={onRefresh} readOnly={!mayManage} />
          </section>

        </>
      );
  const sec_current = (
        <>
          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("当前工作", "Current work")}</span></div>
            <div className="app-list">
              <div className="app-counts">
                <span>{L("处理中", "In progress")}<b>{Math.max(0, row.st.in_progress - stuck.in_progress)}</b></span>
                <span>{L("待处理", "Queued")}<b>{row.st.queued}</b></span>
                <span>{L("已挂起", "Held")}<b>{Math.max(0, row.st.held - stuck.held)}</b></span>
                {stuck.total > 0 && <span className="is-stuck">{L("可能卡住", "May be stuck")}<b>{stuck.total}</b></span>}
              </div>
              {current.length === 0 && <div className="app-empty-note">{row.st.queued + row.st.in_progress + row.st.held > 0
                ? L("这些消息不在你能看到的范围里。", "These messages are not visible to you.")
                : L("暂无未完成的任务", "Nothing open")}</div>}
              {current.map((m) => (
                <div key={m.id} className="app-msg-wrap">
                  <button type="button" className="app-msg" onClick={() => onThread(hubId, m.thread_id, ag.name)}>
                    <span className="meta">
                      <span className={cn("app-msg-status", m.status, isStuck(m) && "is-stuck")}>{openStatusLabel(m.status, isStuck(m))}</span>
                      <span className="who">{personLabel(m.from, row.hub)}</span>→<span className="who">{personLabel(m.to, row.hub)}</span>
                      <span className="time">{timeAgo(m.active_at || m.updated_at, now)}</span>
                    </span>
                    <span className="body">{m.body}</span>
                  </button>
                  {mayManage && (
                    <button type="button" className="app-msg-dismiss" disabled={busy}
                      title={L("不回复，直接标记为已处理", "Mark it handled without a reply")}
                      onClick={() => void run(() => appApi.dismissMessage(hubId, m.id), "stay")}>
                      {L("忽略", "Dismiss")}
                    </button>
                  )}
                </div>
              ))}
            </div>
            {!action && error != null && <ErrorText error={error} />}
          </section>

        </>
      );
  const sec_convo = (
        <>
          <section className="app-group">
            <div className="app-group-title">
              <span className="name">{L("和你的对话", "Conversations with you")}</span>
              {(kind === "agent" || kind === "shell") && (
                <button type="button" className="app-group-action" onClick={() => onMessage(row)}>
                  <IconChat />{L("发消息", "Message")}
                </button>
              )}
            </div>
            <div className="app-list"><MessageRows row={row} now={now} onThread={onThread} /></div>
          </section>

        </>
      );
  const sec_peers = (
        <>
          <section className="app-group">
            <div className="app-group-title">
              <span className="name">{L("与其他 agent 的往来", "Exchanges with other nodes")}</span>
              <span className="app-group-note">{L("只读", "Read-only")}</span>
            </div>
            <div className="app-list"><PeerRows row={row} now={now} onPeer={onPeer} /></div>
          </section>

        </>
      );
  const sec_history = (
        <>
          {mayManage && row.hub.me?.member && (
            <section className="app-group">
              <div className="app-group-title"><span className="name">{L("修改记录", "Changes")}</span></div>
              <div className="app-list">
                <HistoryRows row={row} now={now}
                  stamp={[ag.display_name, ag.description, ag.labels.join(","), (ag.skills || []).map((x) => `${x.name}:${x.summary}:${(x.examples || []).join("|")}`).join(","), ag.visibility, ag.owner, ag.allow_from.join(",")].join("\u0000")} />
              </div>
            </section>
          )}

        </>
      );
  const sec_identity = (
        <>
          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("身份与绑定", "Identity and binding")}</span></div>
            <div className="app-list">
              <div className="app-kv"><span className="app-kv-label">{L("agent 名", "Node name")}</span><span className="app-kv-value mono">{ag.name}</span></div>
              <AliasRow row={row} mayManage={mayManage} onRefresh={onRefresh} />
              <ResumeRow row={row} now={now} mayManage={mayManage} onRefresh={onRefresh} />
              <div className="app-kv"><span className="app-kv-label">Hub</span><span className="app-kv-value">{row.hub.name}</span></div>
              <div className="app-kv"><span className="app-kv-label">Agent ID</span><span className="app-kv-value mono">{ag.id}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("会话 ID", "Session ID")}</span><span className="app-kv-value mono">{ag.binding.session_id || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("工作目录", "Working directory")}</span><span className="app-kv-value mono">{ag.binding.workdir || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("投递方式", "Delivery")}</span><span className="app-kv-value">{ag.delivery.mode}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("入网令牌", "Join token")}</span><span className="app-kv-value mono">{ag.enrolled_by || L("管理员注册", "Registered by admin")}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("允许来源", "Allowed from")}</span><span className="app-kv-value">{ag.allow_from.length && !ag.allow_from.includes("*") ? ag.allow_from.join(", ") : L("任何 agent", "Any node")}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("最近在线", "Last online")}</span><span className="app-kv-value">{timeAgo(ag.last_seen_at, now)}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("注册时间", "Registered")}</span><span className="app-kv-value">{new Date(ag.created_at).toLocaleString(locale(), { hour12: false })}</span></div>
            </div>
          </section>

        </>
      );
  const sec_actions = (
        <>
          <section className="app-group">
            <div className="app-group-title"><span className="name">{embedded ? L("管理", "Manage") : L("操作", "Actions")}</span></div>
            <div className="app-list">
              {!mayManage && (
                <div className="app-empty-note">{L(`这是 ${ag.owner || L("别人", "someone else")} 的 agent，只有 ${ag.owner || L("他", "they")} 或管理员能修改。`, `This node belongs to ${ag.owner || "someone else"}; only they or an admin can change it.`)}</div>
              )}
              {!embedded && (
                <button type="button" className="app-action-row" onClick={() => openAction("profile")} disabled={!mayManage}>
                  <Pencil />{L("修改显示名和描述", "Edit display name and description")}<span className="app-chev">›</span>
                </button>
              )}
              <button type="button" className="app-action-row" onClick={() => openAction("allow")} disabled={!mayManage}>
                <UserCheck />{L("允许谁派任务", "Who can give tasks")}
                <span className="app-action-value">{ag.allow_from.length && !ag.allow_from.includes("*") ? L(`${ag.allow_from.length} 个`, `${ag.allow_from.length}`) : L("任何 agent", "Anyone")}</span>
                <span className="app-chev">›</span>
              </button>
              {forkable && (
                <button type="button" className="app-action-row" onClick={openCopy}>
                  <Copy />{L("复制一个", "Make a copy")}<span className="app-chev">›</span>
                </button>
              )}
              {/* Quiet until pointed at: they ask first anyway. */}
              <button type="button" className="app-action-row danger" onClick={() => openAction("kick")} disabled={!mayManage}>
                <Power />{L("让它下线", "Disconnect")}
              </button>
              <button type="button" className="app-action-row danger" onClick={() => openAction("delete")} disabled={!mayManage}>
                <Trash2 />{L("删除这个 agent", "Delete this agent")}
              </button>
            </div>
          </section>
        </>
      );
  const sections = embedded ? (
    <div className="ms-details">
      <button type="button" className={cn("ms-fold", more && "open")} onClick={() => setMore((v) => !v)} aria-expanded={more}>
        <ChevronRight />{L("详细信息", "Details")}
      </button>
      {more && (
        <div className="ms-fold-body">
          {row.st.state !== "offline" && row.st.session_in_use && row.st.receiving !== "session" && <div className="ms-alert-in"><InUseCard /></div>}
          {sec_runtime}
          {sec_about}
          {sec_labels}
          {sec_peers}
          {sec_history}
          {sec_identity}
        </div>
      )}
      {sec_actions}
    </div>
  ) : null;

  return (
    <>
      {embedded ? sections : (<>
      <div className="app-inspector-backdrop" onClick={onClose} />
      <aside className="app-inspector" role="dialog" aria-label={L(`${nodeName(ag, row.hub)} 详情`, `${nodeName(ag, row.hub)} details`)}>
        <div className="app-inspector-head">
          <span className="app-row-icon"><Icon /></span>
          <div className="titles">
            <h2 title={ag.name}>{nodeName(ag, row.hub)}</h2>
          </div>
          <span className={cn("app-pill", sessionBusy(row.st) ? pillClass.pending : pillClass[row.st.state])}>
            <span className={cn("dot", stateLabel(row.st, state.label).dot)} />{stateLabel(row.st, state.label).label}
          </span>
          <button type="button" className="app-btn app-btn-plain" onClick={onClose} aria-label={L("关闭", "Close")}>
            <X />
          </button>
        </div>

        <div className="app-inspector-body">

          {sec_state}
          {sec_about}
          {sec_runtime}
          {sec_labels}
          {sec_current}
          {sec_convo}
          {sec_peers}
          {sec_history}
          {sec_identity}
          {sec_actions}
        </div>
      </aside>
      </>)}

      <Modal
        open={action === "copy"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("复制 agent", "Copy this node")}
        description={L("在同一台电脑、同一个目录里，从它的会话复制出一个新 agent：带着全部上下文，之后两边各干各的。复制要跑一轮对话，可能需要一两分钟。", "Copies its session into a new node on the same machine and folder: it keeps the whole context, then the two work separately. Copying runs one turn, which can take a minute or two.")}
      >
        {copied ? (
          <>
            <div className="app-success"><Check />{L(`已复制为 ${copied}`, `Copied as ${copied}`)}</div>
            <div className="app-modal-actions"><button type="button" className="app-btn app-btn-primary" onClick={closeAction}>{L("完成", "Done")}</button></div>
          </>
        ) : (
          <>
            <label className="app-field">
              <span className="app-field-label">{L("新 agent 的名字", "Name of the copy")}</span>
              <input className="app-input mono" value={copyName} onChange={(e) => setCopyName(e.target.value)} disabled={busy} />
            </label>
            <ErrorText error={error} />
            {busy && <div className="app-progress"><span className="app-spinner" />{L("正在复制会话…", "Copying the session…")}</div>}
            <div className="app-modal-actions">
              <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
              <button type="button" className="app-btn app-btn-primary" onClick={copyNode} disabled={busy || !copyName.trim()}>{busy ? L("正在复制…", "Copying…") : L("复制", "Copy")}</button>
            </div>
          </>
        )}
      </Modal>

      <Modal
        open={action === "owner"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("转交 agent", "Give this node to someone")}
        description={L("新主人可以修改、踢下线和删除它；它收到的消息会说明是谁的 agent。", "The new owner can change, disconnect and delete it; messages it sends say whose agent it is.")}
      >
        <label className="app-field">
          <span className="app-field-label">{L("主人", "Owner")}</span>
          <select className="app-select-field" value={newOwner} onChange={(e) => setNewOwner(e.target.value)} disabled={busy}>
            {people.map((p) => <option key={p.id} value={p.name}>{p.display_name ? `${p.display_name} (@${p.name})` : p.name}</option>)}
          </select>
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" disabled={busy || !newOwner || newOwner === ag.owner}
            onClick={() => void run(() => appApi.setAgentOwner(hubId, ag.name, newOwner), "close-action")}>{L("转交", "Give")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "profile"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("修改显示名和描述", "Edit display name and description")}
        description={L("显示名和描述会在所有工作区上生效。显示名留空则只显示名字。", "The display name and description apply on every Hub. Leave the display name empty to show just the name.")}
      >
        <label className="app-field">
          <span className="app-field-label">{L("显示名", "Display name")}</span>
          <input className="app-input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={ag.name} />
        </label>
        <label className="app-field">
          <span className="app-field-label">{L("描述", "Description")}</span>
          <textarea className="app-input" value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveProfile} disabled={busy}>{busy ? L("保存中…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "visibility"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("可见范围", "Who can see it")}
        description={L("谁能在列表里找到它、给它派活。只有它的主人和管理员能改， agent 自己改不了。", "Who can find it and give it tasks. Only its owner and admins can change this; the agent itself cannot.")}
      >
        <div className="app-provider-list" role="radiogroup">
          <label className={cn("app-provider-opt", visChoice === "team" && "picked")}>
            <input type="radio" name="visibility" checked={visChoice === "team"} onChange={() => setVisChoice("team")} disabled={busy} />
            <span>
              <b>{L("团队可见", "Everyone on the hub")}</b>
              <small>{L("工作区上的人和 agent 都能找到它；谁能派活看「允许谁派任务」。", "Everyone on the hub can find it; who can give it tasks is set under Who can give tasks.")}</small>
            </span>
          </label>
          <label className={cn("app-provider-opt", visChoice === "private" && "picked")}>
            <input type="radio" name="visibility" checked={visChoice === "private"} onChange={() => setVisChoice("private")} disabled={busy} />
            <span>
              <b>{L("仅自己", "Only its owner")}</b>
              <small>{L(`只有 ${ag.owner || "主人"} 和 ${ag.owner || "主人"} 自己的 agent 能找到它、给它派活；别人的回复照常能收到。`, `Only ${ag.owner || "its owner"} and their own agents can find it and give it tasks; replies from others still arrive.`)}</small>
            </span>
          </label>
        </div>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" disabled={busy || visChoice === (ag.visibility === "private" ? "private" : "team")}
            onClick={() => void run(() => appApi.setVisibility(hubId, ag.name, visChoice), "close-action")}>{L("保存", "Save")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "projects"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("加入项目", "Join a project")}
        description={L("加入后它能用 myco project show / pull 读到项目的目标、说明和文档。", "Once in, it reads the project's goal, brief and documents with myco project show / pull.")}
      >
        <div className="app-provider-list" role="radiogroup">
          {joinable.map((p) => (
            <label key={p.name} className={cn("app-provider-opt", joinChoice === p.name && "picked")}>
              <input type="radio" name="join-project" checked={joinChoice === p.name} onChange={() => setJoinChoice(p.name)} disabled={busy} />
              <span>
                <b>{p.title || p.name}</b>
                {p.goal && <small>{p.goal}</small>}
              </span>
            </label>
          ))}
        </div>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" disabled={busy || !joinChoice}
            onClick={() => void run(async () => { await appApi.putProjectMember(hubId, joinChoice, ag.name); onProjectsChanged?.(); }, "close-action")}>{L("加入", "Join")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "skills"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("技能", "Skills")}
        description={L("别的 agent 按技能挑人派活。每个技能一个短名字、一句话说明，可以加几个例子。 agent 自己也能改，以后写的为准。", "Other agents pick who to hand work to by skill: a short name, one line on what it covers, and a few examples. The agent can change them too; the last change wins.")}
      >
        <div className="app-skill-edit-list">
          {skillDrafts.map((d, i) => (
            <div className="app-skill-edit" key={i}>
              <div className="app-skill-edit-row">
                <input className="app-input mono" value={d.name} placeholder={L("名字，如 ios-release", "name, e.g. ios-release")}
                  onChange={(e) => editDraft(i, { name: e.target.value })} disabled={busy} aria-label={L("技能名", "Skill name")} />
                <button type="button" className="app-btn app-btn-plain" onClick={() => setSkillDrafts((ds) => ds.filter((_, j) => j !== i))}
                  disabled={busy} aria-label={L("删除这个技能", "Remove this skill")} title={L("删除", "Remove")}><Trash2 /></button>
              </div>
              <input className="app-input" value={d.summary} placeholder={L("一句话说明它能做什么", "One line: what it covers")}
                onChange={(e) => editDraft(i, { summary: e.target.value })} disabled={busy} aria-label={L("说明", "Summary")} />
              <textarea className="app-input app-skill-examples" value={d.examples} rows={2}
                placeholder={L("例子（可选，每行一个，最多 3 个）", "Examples (optional, one per line, up to 3)")}
                onChange={(e) => editDraft(i, { examples: e.target.value })} disabled={busy} aria-label={L("例子", "Examples")} />
            </div>
          ))}
          <button type="button" className="app-btn app-skill-add" onClick={() => setSkillDrafts((ds) => [...ds, { name: "", summary: "", examples: "" }])}
            disabled={busy || skillDrafts.length >= 20}><Plus />{L("添加技能", "Add a skill")}</button>
        </div>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveSkills} disabled={busy}>{busy ? L("正在保存…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </Modal>

      <Modal
        open={action === "allow"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("允许谁派任务", "Who can give tasks")}
        description={L("只有选中的 agent 可以给它派任务。一个都不选表示任何 agent 都可以。你在 App 里发的消息不受这个限制。", "Only the selected nodes can give it tasks. Select none to allow any node. Messages you send from the app are not limited by this.")}
      >
        <NodePicker
          nodes={(row.hub.overview?.agents || []).map((x) => x.agent)
            .filter((x) => x.name !== ag.name && agentKind(x) !== "machine")}
          value={allowFrom.filter((n) => n !== "*")}
          onChange={setAllowFrom}
          emptyPlaceholder={anyNode ? L("现在任何 agent 都可以；选中 agent 后只允许它们", "Any node can now; pick nodes to allow only them") : undefined}
          disabled={busy}
        />
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveAllowFrom} disabled={busy}>{busy ? L("保存中…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </Modal>

      <ConfirmDialog
        open={action === "kick"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("让它下线？", "Disconnect it?")}
        subject={who}
        description={L("它会马上断开，收不到新消息，正在做的活会中断。", "It disconnects right away, stops getting messages, and what it is doing stops.")}
        consequences={L("资料和聊天记录都还在，重新接入就回来了。", "Its profile and chats stay; joining again brings it back.")}
        confirmLabel={L("下线", "Disconnect")}
        busy={busy}
        error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void run(() => appApi.kick(hubId, ag.name), "close-action")}
      />
      <ConfirmDialog
        open={action === "delete"}
        onOpenChange={(o) => !o && closeAction()}
        title={L("删除这个 agent？", "Delete this agent?")}
        subject={who}
        description={L(`它会从「${row.hub.name}」里删掉，令牌随之作废，要回来得重新接入。`, `It is removed from ${row.hub.name} and its token stops working; it has to join again to come back.`)}
        consequences={L("和它的聊天记录会留着。", "Your chats with it are kept.")}
        confirmLabel={L("删除", "Delete")}
        busy={busy}
        error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void run(() => appApi.removeAgent(hubId, ag.name), "close-panel")}
      />
    </>
  );
}
